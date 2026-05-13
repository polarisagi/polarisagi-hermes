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

	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// AnthropicToAnthropic is a pure passthrough: no protocol conversion, just load balancing + billing.
// It proxies the Anthropic request body as-is to the target upstream and streams the response back.
func AnthropicToAnthropic(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	// count_tokens 端点直接透传到上游 Anthropic 节点以获得官方精确计数
	// 失败时函数内会降级到本地估算
	if isCountTokensPath(r.URL.Path) {
		handleCountTokensPassthrough(ctx, w, r, bodyBytes, dest, traceID)
		return
	}

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

	finalResp, err := httpClient.Do(proxyReq)
	if err != nil {
		utils.HandleNetworkError(w, err, dest, "anthropic", clientType, "passthrough", traceID, "Anthropic Passthrough")
		return
	}

	isNodeFailure, isQuotaExhausted := utils.CheckResponseStatus(finalResp, dest, "anthropic", clientType, "passthrough", traceID, "Anthropic Passthrough")

	if req.Stream {
		anthropicPassthroughStream(w, finalResp, traceID, dest, clientType, modelName, bodyBytes)
	} else {
		anthropicPassthroughNonStream(w, finalResp, traceID, dest, clientType, modelName, bodyBytes)
	}

	utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
}

// anthropicPassthroughStream 直通流式响应：读取上游 Anthropic SSE 流，原样写回客户端并结算
func anthropicPassthroughStream(w http.ResponseWriter, upstreamResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string, reqBody []byte) {
	defer upstreamResp.Body.Close()

	for k, vv := range upstreamResp.Header {
		if !strings.EqualFold(k, "Content-Length") && !strings.EqualFold(k, "Transfer-Encoding") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(upstreamResp.StatusCode)

	tailBuf, _ := utils.ForwardStreamBody(w, upstreamResp.Body)

	if bytes.Contains(tailBuf, []byte("output_tokens")) {
		extractAndRecordAnthropicUsage(tailBuf, modelName, dest, clientType, "passthrough", traceID, reqBody)
	}
}

// anthropicPassthroughNonStream 直通非流式响应：读取上游 Anthropic JSON 响应，解析 usage 并结算后原样写回
func anthropicPassthroughNonStream(w http.ResponseWriter, upstreamResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string, reqBody []byte) {
	defer upstreamResp.Body.Close()
	bodyBytes, err := io.ReadAll(upstreamResp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	parseAndSettleAnthropicResponse("anthropic", bodyBytes, dest, clientType, "passthrough", modelName, traceID, upstreamResp.StatusCode, reqBody)

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
func extractAndRecordAnthropicUsage(tailBuf []byte, modelName string, dest *router.MatchedDestination, clientType, methodName, traceID string, reqBody []byte) {
	// Try to find output_tokens in the tail buffer (Anthropic stream format)
	re := regexp.MustCompile(`"output_tokens"\s*:\s*(\d+)`)
	match := re.FindSubmatch(tailBuf)
	if len(match) > 1 {
		var outputTokens int64
		fmt.Sscanf(string(match[1]), "%d", &outputTokens)
		settleBilling("anthropic", dest.Node.Name, clientType, methodName, modelName, 0, outputTokens, 0, http.StatusOK, dest, reqBody, traceID)
	}
}

func init() {
	router.RegisterTranslator("anthropic", "anthropic", AnthropicToAnthropic)
}
