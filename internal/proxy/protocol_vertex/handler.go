package protocol_vertex

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
	"polaris-gateway/internal/webapi"
)

// Project Atlas: Polaris Gateway (Vertex Native Protocol Module)
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
	CutoffPercent     float64
	Budget            float64
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
				CutoffPercent:   acc.CutoffPercent,
				Budget:          acc.Budget,
			}

			if state.Budget > 0 && state.CutoffPercent > 0 {
				usagePercent := (state.TotalConsumed / state.Budget) * 100
				if usagePercent >= state.CutoffPercent {
					state.Status = StatusExhausted
					slog.Warn("🚫 [Vertex 启动期熔断] 账号物理隔离", "name", state.Name, "percent", state.CutoffPercent, "consumed", state.TotalConsumed, "budget", state.Budget)
				} else {
					state.Status = StatusIdle
					availableSem <- struct{}{}
				}
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
				slog.Info("⏳ [冷却守护] Vertex 账号冷却结束", "name", state.Name, "status", "Probation")
				availableSem <- struct{}{}
			}
		}
		poolMutex.Unlock()
	}
}

// ==========================================
// HTTP Handler 与 核心逻辑编排
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
		slog.Warn("⚠️ [上下文过载警告] 客户端发送了超大上下文，建议及时开启新会话！", "trace_id", traceID)
	}

	if config.AppConfig.DebugMode {
		var displayBody string
		const threshold = 1024

		if len(bodyBytes) > threshold {
			displayBody = string(bodyBytes[:512]) +
				fmt.Sprintf("\n\n... [数据过长触发安全截断，原始大小: %d Bytes] ...\n\n", len(bodyBytes)) +
				string(bodyBytes[len(bodyBytes)-256:])
		} else {
			displayBody = string(bodyBytes)
		}

		var headersStr strings.Builder
		for k, vv := range r.Header {
			headersStr.WriteString(fmt.Sprintf("\n  - %s: %s", k, strings.Join(vv, ", ")))
		}

		slog.Debug("🔍 [深度排障]", "trace_id", traceID, "headers", headersStr.String(), "body", displayBody)
	}

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method == http.MethodGet && (r.URL.Path == "/v1" || r.URL.Path == "/v1/" || r.URL.Path == "/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "Polaris Vertex Gateway is fully operational"}`))
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": [{"id": "gemini-3.1-pro-preview-customtools", "object": "model"}], "object": "list"}`))
		return
	}

	if strings.Contains(r.URL.Path, "chat/completions") {
		slog.Error("🚨 协议阻断: 客户端尝试将 OpenAI 请求打入 Vertex Native 物理端点", "trace_id", traceID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": {"message": "Please use /openai/v1/chat/completions endpoint for OpenAI protocol.", "type": "protocol_mismatch"}}`))
		return
	}

	methodName := extractMethodName(r.URL.Path)
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()

	// 🆕 单点调度：去除死循环逻辑
	atomic.AddInt32(&webapi.WaitingCount, 1)
	chosenState, isProbationRun, err := acquireAccount(ctx)
	atomic.AddInt32(&webapi.WaitingCount, -1)

	if err != nil || chosenState == nil {
		http.Error(w, "Vertex Gateway: Queue Timeout", 503)
		return
	}

	atomic.AddInt32(&webapi.ActiveCount, 1)
	defer atomic.AddInt32(&webapi.ActiveCount, -1)

	targetURL := buildTargetURL(chosenState.AccountDetail, r.URL.Path)
	modelName := extractModelName(targetURL)

	if isProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路", "trace_id", traceID, "account", chosenState.Name)
	}

	// 🆕 纯净版网络透传
	finalResp, err := executeProxyRequest(ctx, r, targetURL, bodyBytes, chosenState.AccountDetail)

	if err != nil {
		errMsg := err.Error()
		db.SaveUsage("vertex", chosenState.Name, clientType, methodName, 0, 0, 0, http.StatusBadGateway)
		updateAccountStateOnFailure(chosenState, isProbationRun, traceID)
		slog.Error("Vertex 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Vertex Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	// 🆕 高精度熔断机制
	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		db.SaveUsage("vertex", chosenState.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("Vertex 节点异常/限流，记入熔断惩罚队列", "trace_id", traceID, "status", statusCode)
	} else {
		if statusCode >= 400 {
			db.SaveUsage("vertex", chosenState.Name, clientType, methodName, 0, 0, 0, statusCode)
			slog.Warn("Vertex 客户端业务请求参数错误", "trace_id", traceID, "status", statusCode)
		}
	}

	streamAndSettleUsage(w, finalResp, chosenState, modelName, clientType, methodName, traceID, startTime)

	// 🔴 所有数据流式传输完成（或断开）后，再最终释放账号和更新状态
	if isNodeFailure {
		updateAccountStateOnFailure(chosenState, isProbationRun, traceID)
	} else {
		updateAccountStateOnSuccess(chosenState)
	}
}

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
			isProbationRun := (chosen.Status == StatusProbation)
			chosen.Status = StatusBusy
			// 🆕 修正点：使用正确的变量名 isProbationRun
			return chosen, isProbationRun, nil
		}
		return nil, false, nil
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-time.After(120 * time.Second):
		return nil, false, fmt.Errorf("queue timeout")
	}
}

// 🆕 网络发射层，透传 Query 参数并注入鉴权，不再插手 HTTP Code 拦截
func executeProxyRequest(ctx context.Context, r *http.Request, targetURL string, bodyBytes []byte, acc config.AccountDetail) (*http.Response, error) {
	bodyReader := bytes.NewReader(bodyBytes)
	proxyReq, _ := http.NewRequestWithContext(ctx, r.Method, targetURL, bodyReader)
	proxyReq.Header.Del("Authorization")

	// 🛠️ 核心修复点：将客户端原始的 URL Query 参数无损合并过来 (解决 ?alt=sse 丢失导致 OpenCode 静默失败的问题)
	q := proxyReq.URL.Query()
	for k, vv := range r.URL.Query() {
		for _, v := range vv {
			q.Add(k, v)
		}
	}

	// 注入 Vertex 专属鉴权 Key
	q.Set("key", acc.Key)
	proxyReq.URL.RawQuery = q.Encode()

	for k, vv := range r.Header {
		if !strings.EqualFold(k, "Host") && !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Accept-Encoding") && !strings.EqualFold(k, "Authorization") {
			for _, v := range vv {
				proxyReq.Header.Add(k, v)
			}
		}
	}

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
		slog.Warn("🧊 [Vertex 熔断]", "trace_id", traceID, "account", state.Name, "duration", state.CurrentCooldown, "until", state.CooldownUntil.Format("15:04:05"))

		state.CurrentCooldown *= 2
		maxDuration := time.Duration(config.AppConfig.Breaker.MaxCooldownSeconds) * time.Second
		if state.CurrentCooldown > maxDuration {
			state.CurrentCooldown = maxDuration
		}
	} else {
		state.Status = StatusIdle
		availableSem <- struct{}{}
	}
}

func streamAndSettleUsage(w http.ResponseWriter, finalResp *http.Response, state *AccountState, modelName, clientType, methodName, traceID string, startTime time.Time) {
	defer finalResp.Body.Close()

	// 🛠️ 修复点 1：严格清洗 Hop-by-Hop 脏头，防止客户端 SSE 解析器崩溃
	for k, vv := range finalResp.Header {
		if !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Content-Encoding") &&
			!strings.EqualFold(k, "Transfer-Encoding") && // 必须剔除，交由 Go 自动处理
			!strings.EqualFold(k, "Connection") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(finalResp.StatusCode)

	flusher, _ := w.(http.Flusher)
	// 🛠️ 修复点 2：将缓冲池从 8KB 扩大至 32KB 工业级，防止超大文件读写 Tool 的 JSON 被意外切片
	buf := make([]byte, 32*1024)
	var tailBuf []byte
	const tailWindowSize = 8192

	for {
		n, readErr := finalResp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				break
			}
			if flusher != nil {
				flusher.Flush()
			}

			// 仅截取尾部用于计费统计
			tailBuf = append(tailBuf, buf[:n]...)
			if len(tailBuf) > tailWindowSize {
				tailBuf = tailBuf[len(tailBuf)-tailWindowSize:]
			}
		}
		if readErr != nil {
			break
		}
	}

	// 计费截获逻辑支持 Context Caching
	if bytes.Contains(tailBuf, []byte("usageMetadata")) {
		pMatch := promptRegex.FindSubmatch(tailBuf)
		cMatch := candidateRegex.FindSubmatch(tailBuf)
		cacheMatch := cachedContentRegex.FindSubmatch(tailBuf)

		if len(pMatch) > 1 && len(cMatch) > 1 {
			p := parseToInt(pMatch[1])
			c := parseToInt(cMatch[1])
			var cached int64
			if len(cacheMatch) > 1 {
				cached = parseToInt(cacheMatch[1])
			}

			cost := calculateCost(modelName, p, c, cached)

			db.SaveUsage("vertex", state.Name, clientType, methodName, p, c, cost, finalResp.StatusCode)

			poolMutex.Lock()
			state.TotalConsumed += cost
			if state.Budget > 0 && state.CutoffPercent > 0 {
				usagePercent := (state.TotalConsumed / state.Budget) * 100
				if usagePercent >= state.CutoffPercent {
					if state.Status != StatusExhausted {
						state.Status = StatusExhausted
						slog.Warn("🚫 [Vertex 运行期熔断] 账号触达上限，物理隔离", "account", state.Name)
					}
				}
			}
			poolMutex.Unlock()

			if cached > 0 {
				slog.Info("💰 结算完成", "trace_id", traceID, "account", state.Name, "model", modelName, "p", p, "cached", cached, "c", c, "cost", fmt.Sprintf("%.4f", cost))
			} else {
				slog.Info("💰 结算完成", "trace_id", traceID, "account", state.Name, "model", modelName, "p", p, "c", c, "cost", fmt.Sprintf("%.4f", cost))
			}
		}
	}
}
