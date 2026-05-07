package protocol_openai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
	"polaris-gateway/internal/webapi"
)

// Project Atlas: Polaris Gateway (OpenAI Protocol Module)
// Author: mrlaoliai

type AccountStatus int

const (
	StatusIdle AccountStatus = iota
	StatusBusy
	StatusCooldown
	StatusProbation
	StatusExhausted
)

type AccountState struct {
	config.AccountDetail
	Status            AccountStatus
	FailureTimestamps []time.Time
	CurrentCooldown   time.Duration
	CooldownUntil     time.Time
	TotalConsumed     float64
}

var (
	accountStates []*AccountState
	stateInitOnce sync.Once
	httpClient    = &http.Client{Timeout: 180 * time.Second}
	availableSem  chan struct{}
	poolMutex     sync.Mutex
)

// ==========================================
// 状态机核心方法
// ==========================================

func (s *AccountState) checkFailureWindow() bool {
	threshold := config.AppConfig.Breaker.FailureThreshold
	window := time.Duration(config.AppConfig.Breaker.FailureWindowSeconds) * time.Second
	now := time.Now()
	var valid []time.Time
	for _, t := range s.FailureTimestamps {
		if now.Sub(t) <= window {
			valid = append(valid, t)
		}
	}
	s.FailureTimestamps = valid
	return len(s.FailureTimestamps) >= threshold
}

func (s *AccountState) recordFailureAndCheck() bool {
	s.FailureTimestamps = append(s.FailureTimestamps, time.Now())
	return s.checkFailureWindow()
}

func initPool(accounts []config.AccountDetail) {
	stateInitOnce.Do(func() {
		accountStates = make([]*AccountState, 0, len(accounts))
		availableSem = make(chan struct{}, len(accounts))

		for _, acc := range accounts {
			cycleConsumed := db.GetConsumedSince(acc.Name, acc.BillingStartDate)
			state := &AccountState{
				AccountDetail:   acc,
				CurrentCooldown: time.Duration(config.AppConfig.Breaker.InitialCooldownSeconds) * time.Second,
				TotalConsumed:   cycleConsumed,
			}

			threshold := state.CutoffPercent / 100.0
			if state.Budget > 0 && state.TotalConsumed >= state.Budget*threshold {
				state.Status = StatusExhausted
				slog.Warn("🚫 [OAI 启动期熔断] 账号已达消耗上限，执行物理隔离", "account", state.Name, "limit", state.CutoffPercent, "used", state.TotalConsumed, "budget", state.Budget)
			} else {
				state.Status = StatusIdle
				availableSem <- struct{}{}
			}
			accountStates = append(accountStates, state)
		}
		go cooldownManager()
	})
}

func cooldownManager() {
	for {
		time.Sleep(1 * time.Second)
		now := time.Now()
		poolMutex.Lock()
		for _, state := range accountStates {
			if state.Status == StatusCooldown && now.After(state.CooldownUntil) {
				state.Status = StatusProbation
				slog.Info("⏳ [冷却守护] OAI 账号冷却期结束，进入 🟠 Probation", "account", state.Name)
				availableSem <- struct{}{}
			}
		}
		poolMutex.Unlock()
	}
}

// ==========================================
// HTTP Handler 与核心调度
// ==========================================

type Handler struct{}

func NewHandler(accounts []config.AccountDetail) *Handler {
	initPool(accounts)
	return &Handler{}
}

func (h *Handler) ProxyHandler(w http.ResponseWriter, r *http.Request) {
	traceID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	clientType := identifyClient(r)
	originalURI := r.URL.RequestURI()
	methodName := extractMethodName(r.URL.Path)

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()

	// 📊 载荷体积与粗略 Token 估算
	payloadSize := len(bodyBytes)
	payloadKB := float64(payloadSize) / 1024.0
	// 粗略估算：综合中英文字符，约 3.5 个字节对应 1 个 Token
	estTokens := int(float64(payloadSize) / 3.5)

	slog.Info("📥 [请求接入]", "trace_id", traceID, "method", r.Method, "client", clientType, "size_kb", payloadKB, "est_tokens", estTokens, "target", originalURI)
	// ⚠️ 超大载荷预警 (当预估 Token 超过 100,000 时在终端标红高亮提醒)
	if estTokens > 100000 {
		slog.Warn("⚠️ [上下文过载警告] 客户端发送了超大上下文", "trace_id", traceID)
	}

	if config.AppConfig.DebugMode {
		var displayBody string
		const threshold = 1024

		if len(bodyBytes) > threshold {
			displayBody = string(bodyBytes[:512]) +
				fmt.Sprintf("\n\n... [数据过长已触发安全截断，原始总大小: %d Bytes] ...\n\n", len(bodyBytes)) +
				string(bodyBytes[len(bodyBytes)-256:])
		} else {
			displayBody = string(bodyBytes)
		}

		var headersStr strings.Builder
		for k, vv := range r.Header {
			headersStr.WriteString(fmt.Sprintf("\n  - %s: %s", k, strings.Join(vv, ", ")))
		}

		slog.Debug("🔍 [深度排障]", "trace_id", traceID, "headers", headersStr.String(), "payload", displayBody)
	}

	if bytes.Contains(bodyBytes, []byte(`"stream": true`)) || bytes.Contains(bodyBytes, []byte(`"stream":true`)) {
		if !bytes.Contains(bodyBytes, []byte(`"include_usage"`)) {
			bodyBytes = bytes.Replace(bodyBytes, []byte(`"stream": true`), []byte(`"stream": true, "stream_options": {"include_usage": true}`), 1)
			bodyBytes = bytes.Replace(bodyBytes, []byte(`"stream":true`), []byte(`"stream":true,"stream_options":{"include_usage":true}`), 1)
		}
	}

	startTime := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()

	// 🆕 废除重试循环，实施“单次调度，透明透传”
	atomic.AddInt32(&webapi.WaitingCount, 1)
	chosenState, isProbationRun, err := acquireAccount(ctx)
	atomic.AddInt32(&webapi.WaitingCount, -1)

	if err != nil || chosenState == nil {
		http.Error(w, "OAI Gateway: MaxConcurrency Limit or Queue Timeout", http.StatusGatewayTimeout)
		return
	}

	atomic.AddInt32(&webapi.ActiveCount, 1)
	defer atomic.AddInt32(&webapi.ActiveCount, -1)

	targetURL := buildTargetURL(chosenState.AccountDetail, r.URL.Path)
	currentBody := bodyBytes
	modelName := extractModelName(bodyBytes)

	if chosenState.ProjectID != "" {
		if !bytes.Contains(currentBody, []byte(`"model":"google/`)) && !bytes.Contains(currentBody, []byte(`"model": "google/`)) {
			currentBody = bytes.ReplaceAll(currentBody, []byte(`"model":"`), []byte(`"model":"google/`))
			currentBody = bytes.ReplaceAll(currentBody, []byte(`"model": "`), []byte(`"model": "google/`))
		}
	}

	if isProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation OAI 账号探路", "trace_id", traceID, "account", chosenState.Name)
	}

	// 🆕 调用纯净版代理器
	finalResp, err := executeProxyRequest(ctx, r, targetURL, currentBody, chosenState.AccountDetail)

	if err != nil {
		// 物理网络断联 (如 DNS 故障、连接超时) -> 绝对的节点故障
		errMsg := err.Error()
		db.SaveUsage("openai", chosenState.Name, clientType, methodName, 0, 0, 0, http.StatusBadGateway)
		updateAccountStateOnFailure(chosenState, isProbationRun, traceID)
		slog.Error("OAI 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Polaris Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	// 🆕 高精度熔断判定逻辑
	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		// 上游宕机、限流、或 Token 鉴权失败 -> 触发熔断扣分
		db.SaveUsage("openai", chosenState.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("OAI 节点异常/限流，记入熔断惩罚队列", "trace_id", traceID, "status", statusCode)
	} else {
		if statusCode >= 400 {
			// 业务参数错误 -> 正常扣分不熔断
			db.SaveUsage("openai", chosenState.Name, clientType, methodName, 0, 0, 0, statusCode)
			slog.Warn("OAI 客户端业务请求参数错误", "trace_id", traceID, "status", statusCode)
		}
	}

	// 无论成功还是 400 失败，都将响应体原样流式泵回给客户端
	// ⚠️ 在完全流式读取结束前，账号必须保持 Busy 状态，以严格防止并发！
	streamAndSettleUsage(w, finalResp, chosenState, modelName, clientType, methodName, traceID, startTime)

	// 🔴 所有数据流式传输完成（或断开）后，再最终释放账号和更新状态
	if isNodeFailure {
		updateAccountStateOnFailure(chosenState, isProbationRun, traceID)
	} else {
		updateAccountStateOnSuccess(chosenState)
	}
}

// ==========================================
// 私有领域服务模块 (Domain Services)
// ==========================================

func acquireAccount(ctx context.Context) (*AccountState, bool, error) {
	select {
	case <-availableSem:
		poolMutex.Lock()
		defer poolMutex.Unlock()
		var chosen *AccountState
		for _, state := range accountStates {
			if state.Status == StatusIdle || state.Status == StatusProbation {
				if chosen == nil || state.Priority < chosen.Priority {
					chosen = state
				}
			}
		}
		if chosen != nil {
			isProbRun := (chosen.Status == StatusProbation)
			chosen.Status = StatusBusy
			return chosen, isProbRun, nil
		}
		return nil, false, nil
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-time.After(120 * time.Second):
		return nil, false, fmt.Errorf("queue timeout")
	}
}

// 🆕 纯净版网络透传函数：只做发包，不做拦截干预
func executeProxyRequest(ctx context.Context, r *http.Request, targetURL string, bodyBytes []byte, acc config.AccountDetail) (*http.Response, error) {
	if acc.ProjectID != "" {
		parsedURL, err := url.Parse(targetURL)
		if err == nil {
			q := parsedURL.Query()
			q.Set("key", acc.Key)
			parsedURL.RawQuery = q.Encode()
			targetURL = parsedURL.String()
		}
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))

	for k, vv := range r.Header {
		if !strings.EqualFold(k, "Host") && !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Accept-Encoding") && !strings.EqualFold(k, "Authorization") {
			for _, v := range vv {
				proxyReq.Header.Add(k, v)
			}
		}
	}

	if acc.ProjectID == "" {
		proxyReq.Header.Set("Authorization", "Bearer "+acc.Key)
	}

	// 只要建立连接即返回，不管 HTTP 状态码是多少
	localResp, err := httpClient.Do(proxyReq)
	if err != nil {
		return nil, err
	}
	return localResp, nil
}

func updateAccountStateOnSuccess(state *AccountState) {
	poolMutex.Lock()
	defer poolMutex.Unlock()
	if state.Status == StatusExhausted {
		return
	}
	state.FailureTimestamps = nil
	state.CurrentCooldown = time.Duration(config.AppConfig.Breaker.InitialCooldownSeconds) * time.Second
	state.Status = StatusIdle
	availableSem <- struct{}{}
}

func updateAccountStateOnFailure(state *AccountState, isProbationRun bool, traceID string) {
	poolMutex.Lock()
	defer poolMutex.Unlock()
	if state.Status == StatusExhausted {
		return
	}
	if isProbationRun || state.recordFailureAndCheck() {
		state.Status = StatusCooldown
		state.CooldownUntil = time.Now().Add(state.CurrentCooldown)
		slog.Warn("🧊 [OAI 熔断]", "trace_id", traceID, "account", state.Name, "until", state.CooldownUntil.Format("15:04:05"))

		state.CurrentCooldown *= 2
		if max := time.Duration(config.AppConfig.Breaker.MaxCooldownSeconds) * time.Second; state.CurrentCooldown > max {
			state.CurrentCooldown = max
		}
	} else {
		state.Status = StatusIdle
		availableSem <- struct{}{}
	}
}

func streamAndSettleUsage(w http.ResponseWriter, finalResp *http.Response, state *AccountState, modelName, clientType, methodName, traceID string, startTime time.Time) {
	defer finalResp.Body.Close()
	for k, vv := range finalResp.Header {
		if !strings.EqualFold(k, "Content-Length") && !strings.EqualFold(k, "Content-Encoding") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(finalResp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 8192)
	var tailBuf []byte
	const tailWindowSize = 8192

	for {
		n, err := finalResp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				break
			}
			if flusher != nil {
				flusher.Flush()
			}

			tailBuf = append(tailBuf, buf[:n]...)
			if len(tailBuf) > tailWindowSize {
				tailBuf = tailBuf[len(tailBuf)-tailWindowSize:]
			}
		}
		if err != nil {
			break
		}
	}

	if bytes.Contains(tailBuf, []byte("usage")) || bytes.Contains(tailBuf, []byte("prompt_tokens")) {
		pMatch := openAIPromptRegex.FindSubmatch(tailBuf)
		cMatch := openAICompletionRegex.FindSubmatch(tailBuf)
		cacheMatch := openAICachedRegex.FindSubmatch(tailBuf)

		var p, c, cached int64
		if len(pMatch) > 1 {
			p = parseToInt(pMatch[1])
		}
		if len(cMatch) > 1 {
			c = parseToInt(cMatch[1])
		}
		if len(cacheMatch) > 1 {
			cached = parseToInt(cacheMatch[1])
		}

		if p > 0 || c > 0 {
			cost := calculateCost(modelName, p, c, cached)

			db.SaveUsage("openai", state.Name, clientType, methodName, p, c, cost, finalResp.StatusCode)

			poolMutex.Lock()
			state.TotalConsumed += cost
			if state.Budget > 0 && state.CutoffPercent > 0 {
				if (state.TotalConsumed / state.Budget * 100) >= state.CutoffPercent {
					if state.Status != StatusExhausted {
						state.Status = StatusExhausted
						slog.Warn("🚫 [OAI 运行期熔断] 账号触达上限，物理隔离", "account", state.Name)
					}
				}
			}
			poolMutex.Unlock()

			slog.Info("💰 结算成功", "trace_id", traceID, "account", state.Name, "model", modelName, "cost", fmt.Sprintf("%.6f", cost))
		}
	}
}
