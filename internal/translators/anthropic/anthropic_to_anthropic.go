package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

var passthroughHTTPClient = &http.Client{Timeout: 180 * time.Second}

// AnthropicToAnthropic is a pure passthrough: no protocol conversion, just load balancing + billing.
// It proxies the Anthropic request body as-is to the target upstream and streams the response back.
func AnthropicToAnthropic(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := "Anthropic-Passthrough"

	// Parse request minimally to get model name for billing
	var req MessageRequest
	json.Unmarshal(bodyBytes, &req)

	modelName := req.Model
	if dest.TargetModel != "" {
		modelName = dest.TargetModel
		// Rewrite model in body if mapping is set
		bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(fmt.Sprintf(`"model":"%s"`, req.Model)), []byte(fmt.Sprintf(`"model":"%s"`, modelName)))
		bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(fmt.Sprintf(`"model": "%s"`, req.Model)), []byte(fmt.Sprintf(`"model": "%s"`, modelName)))
	}
	if modelName == "" {
		modelName = "unknown"
	}

	targetURL := strings.TrimSuffix(dest.Node.BaseURL, "/")
	if targetURL == "" {
		targetURL = "https://api.anthropic.com/v1"
	}
	// Append the path: the stripped path is like /v1/messages
	// Anthropic base URL already includes /v1, so we need just /messages
	subPath := strings.TrimPrefix(r.URL.Path, "/v1")
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}
	targetURL = targetURL + subPath

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路 (Anthropic Passthrough)", "trace_id", traceID, "account", dest.Node.Name)
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	for k, vv := range r.Header {
		if !strings.EqualFold(k, "Host") && !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Accept-Encoding") && !strings.EqualFold(k, "Authorization") &&
			!strings.EqualFold(k, "X-Api-Key") && !strings.EqualFold(k, "Anthropic-Version") {
			for _, v := range vv {
				proxyReq.Header.Add(k, v)
			}
		}
	}
	proxyReq.Header.Set("x-api-key", dest.Node.Credentials)
	proxyReq.Header.Set("anthropic-version", "2023-06-01")
	proxyReq.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	finalResp, err := passthroughHTTPClient.Do(proxyReq)
	if err != nil {
		errMsg := err.Error()
		db.SaveUsage("anthropic", dest.Node.Name, clientType, "passthrough", 0, 0, 0, http.StatusBadGateway)
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
		slog.Error("Anthropic Passthrough 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		db.SaveUsage("anthropic", dest.Node.Name, clientType, "passthrough", 0, 0, 0, statusCode)
		slog.Warn("Anthropic Passthrough 节点异常/限流", "trace_id", traceID, "status", statusCode)
	} else if statusCode >= 400 {
		db.SaveUsage("anthropic", dest.Node.Name, clientType, "passthrough", 0, 0, 0, statusCode)
		slog.Warn("Anthropic Passthrough 客户端参数错误", "trace_id", traceID, "status", statusCode)
	}

	if req.Stream {
		anthropicPassthroughStream(w, finalResp, traceID, dest, clientType, modelName)
	} else {
		anthropicPassthroughNonStream(w, finalResp, traceID, dest, clientType, modelName)
	}

	_ = startTime
	if isNodeFailure {
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
	} else {
		dest.Node.UpdateOnSuccess()
	}
}

// anthropicPassthroughStream 直通流式响应：读取上游 Anthropic SSE 流，原样写回客户端并结算
func anthropicPassthroughStream(w http.ResponseWriter, upstreamResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string) {
	defer upstreamResp.Body.Close()

	for k, vv := range upstreamResp.Header {
		if !strings.EqualFold(k, "Content-Length") && !strings.EqualFold(k, "Transfer-Encoding") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(upstreamResp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	var tailBuf []byte
	const tailWindowSize = 8192

	for {
		n, readErr := upstreamResp.Body.Read(buf)
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
		if readErr != nil {
			break
		}
	}

	// Parse Anthropic SSE message_delta for usage
	// Anthropic stream format: event: message_delta\n data: {"usage":{"output_tokens": 123}}
	// OR non-stream Anthropic response JSON contains "usage":{"input_tokens":N,"output_tokens":N}
	// For streaming, we look for "output_tokens" in the tail buffer
	if bytes.Contains(tailBuf, []byte("output_tokens")) {
		extractAndRecordAnthropicUsage(tailBuf, modelName, dest, clientType, "passthrough", traceID)
	}
}

// anthropicPassthroughNonStream 直通非流式响应：读取上游 Anthropic JSON 响应，解析 usage 并结算后原样写回
func anthropicPassthroughNonStream(w http.ResponseWriter, upstreamResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string) {
	defer upstreamResp.Body.Close()
	bodyBytes, err := io.ReadAll(upstreamResp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	// Extract usage from Anthropic response JSON
	var resp struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(bodyBytes, &resp) == nil {
		promptTokens := int64(resp.Usage.InputTokens)
		completionTokens := int64(resp.Usage.OutputTokens)
		cost := utils.CalculateCost(modelName, promptTokens, completionTokens, 0)
		db.SaveUsage("anthropic", dest.Node.Name, clientType, "passthrough", promptTokens, completionTokens, cost, upstreamResp.StatusCode)
		dest.Node.RecordCost(cost, traceID)
		slog.Info("💰 结算完成 (Anthropic)", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", promptTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))
	}

	// Pass through headers and body
	for k, vv := range upstreamResp.Header {
		if !strings.EqualFold(k, "Content-Length") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(upstreamResp.StatusCode)
	w.Write(bodyBytes)
}

// extractAndRecordAnthropicUsage 从 Anthropic SSE 流的尾部 buffer 中提取 output_tokens 并完成计费
func extractAndRecordAnthropicUsage(tailBuf []byte, modelName string, dest *router.MatchedDestination, clientType, methodName, traceID string) {
	// Try to find output_tokens in the tail buffer (Anthropic stream format)
	re := regexp.MustCompile(`"output_tokens"\s*:\s*(\d+)`)
	match := re.FindSubmatch(tailBuf)
	if len(match) > 1 {
		var outputTokens int64
		fmt.Sscanf(string(match[1]), "%d", &outputTokens)
		cost := utils.CalculateCost(modelName, 0, outputTokens, 0)
		db.SaveUsage("anthropic", dest.Node.Name, clientType, methodName, 0, outputTokens, cost, http.StatusOK)
		dest.Node.RecordCost(cost, traceID)
		slog.Info("💰 结算完成 (Anthropic Stream)", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "output_tokens", outputTokens, "cost", fmt.Sprintf("%.4f", cost))
	}
}

func init() {
	router.RegisterTranslator("anthropic", "anthropic", AnthropicToAnthropic)
}
