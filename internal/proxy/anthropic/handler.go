package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
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
					slog.Warn("🚫 [Anthropic 启动期熔断] 账号物理隔离", "name", state.Name, "percent", state.CutoffPercent, "consumed", state.TotalConsumed, "budget", state.Budget)
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
				slog.Info("⏳ [冷却守护] Anthropic(Vertex) 账号冷却结束", "name", state.Name, "status", "Probation")
				availableSem <- struct{}{}
			}
		}
		poolMutex.Unlock()
	}
}

type Handler struct{}

func NewHandler(accounts []config.AccountDetail) *Handler {
	initPool(accounts)
	return &Handler{}
}

func (h *Handler) ProxyHandler(w http.ResponseWriter, r *http.Request) {
	traceID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	clientType := "Anthropic-Adapter"
	
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()

	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type": "error", "error": {"type": "invalid_request_error", "message": "invalid json"}}`, 400)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()

	atomic.AddInt32(&webapi.WaitingCount, 1)
	chosenState, isProbationRun, err := acquireAccount(ctx)
	atomic.AddInt32(&webapi.WaitingCount, -1)

	if err != nil || chosenState == nil {
		http.Error(w, "Anthropic Gateway: Queue Timeout", 503)
		return
	}

	atomic.AddInt32(&webapi.ActiveCount, 1)
	defer atomic.AddInt32(&webapi.ActiveCount, -1)

	vReq, _ := mapToVertexRequest(req)
	vReqBytes, _ := json.Marshal(vReq)

	model := req.Model
	if model == "" || strings.Contains(model, "claude") {
		model = "gemini-1.5-pro" // fallback to pro if using claude names
	}
	targetURL := buildTargetURL(chosenState.AccountDetail, model, req.Stream)

	if isProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路 (Anthropic Adapter)", "trace_id", traceID, "account", chosenState.Name)
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(vReqBytes))
	proxyReq.Header.Set("Content-Type", "application/json")

	q := proxyReq.URL.Query()
	q.Set("key", chosenState.Key)
	if req.Stream {
		q.Set("alt", "sse")
	}
	proxyReq.URL.RawQuery = q.Encode()

	finalResp, err := httpClient.Do(proxyReq)
	if err != nil {
		errMsg := err.Error()
		db.SaveUsage("vertex", chosenState.Name, clientType, "anthropic_adapter", 0, 0, 0, http.StatusBadGateway)
		updateAccountStateOnFailure(chosenState, isProbationRun, traceID)
		slog.Error("Anthropic(Vertex) 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		db.SaveUsage("vertex", chosenState.Name, clientType, "anthropic_adapter", 0, 0, 0, statusCode)
		slog.Warn("Anthropic(Vertex) 节点异常/限流，记入熔断惩罚队列", "trace_id", traceID, "status", statusCode)
	} else if statusCode >= 400 {
		db.SaveUsage("vertex", chosenState.Name, clientType, "anthropic_adapter", 0, 0, 0, statusCode)
		slog.Warn("Anthropic 客户端业务请求参数错误", "trace_id", traceID, "status", statusCode)
	}

	if req.Stream {
		streamAnthropicResponse(w, finalResp, req, traceID, chosenState, clientType, model)
	} else {
		// Simplistic non-stream handler for now, simply pipes vertex response back.
		// In a production app, we would map the JSON back to Anthropic structure.
		for k, vv := range finalResp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(finalResp.StatusCode)
		io.Copy(w, finalResp.Body)
		finalResp.Body.Close()
	}

	if isNodeFailure {
		updateAccountStateOnFailure(chosenState, isProbationRun, traceID)
	} else {
		updateAccountStateOnSuccess(chosenState)
	}
}

func buildTargetURL(acc config.AccountDetail, model string, stream bool) string {
	baseURL := acc.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1", acc.Location)
	}
	endpoint := "generateContent"
	if stream {
		endpoint = "streamGenerateContent"
	}
	return fmt.Sprintf("%s/projects/%s/locations/%s/publishers/google/models/%s:%s",
		baseURL, acc.ProjectID, acc.Location, model, endpoint)
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
			return chosen, isProbationRun, nil
		}
		return nil, false, nil
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-time.After(120 * time.Second):
		return nil, false, fmt.Errorf("queue timeout")
	}
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
		slog.Warn("🧊 [Anthropic(Vertex) 熔断]", "trace_id", traceID, "account", state.Name, "duration", state.CurrentCooldown, "until", state.CooldownUntil.Format("15:04:05"))

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
