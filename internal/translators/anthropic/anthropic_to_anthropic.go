package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"regexp"
	"strings"

	"polaris-gateway/internal/router"
)

// AnthropicToAnthropic is a pure passthrough: no protocol conversion, just load balancing + billing.
// It proxies the Anthropic request body as-is to the target upstream and streams the response back.
func AnthropicToAnthropic(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	// count_tokens 端点优化：直接使用本地估算，避免网络请求带来的延迟及 UI 撕裂
	if isCountTokensPath(r.URL.Path) {
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}

	clientType := "Anthropic-Passthrough"

	// Parse request minimally to get model name for billing
	var req MessageRequest
	_ = json.Unmarshal(bodyBytes, &req)

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
	subPath := strings.TrimPrefix(r.URL.Path, "/v1")
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}
	targetURL = targetURL + subPath

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

	router.ExecuteAndStream(w, proxyReq, dest, "anthropic", clientType, "passthrough", traceID, "Anthropic Passthrough",
		func(finalResp *http.Response, startTime time.Time) bool {
			if req.Stream {
				anthropicPassthroughStream(w, finalResp, traceID, dest, clientType, modelName, bodyBytes)
			} else {
				anthropicPassthroughNonStream(w, finalResp, traceID, dest, clientType, modelName, bodyBytes)
			}
			return false
		})
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

	tailBuf, _ := router.ForwardStreamBody(w, upstreamResp.Body)

	if bytes.Contains(tailBuf, []byte("output_tokens")) {
		extractAndRecordAnthropicUsage("anthropic", tailBuf, modelName, dest, clientType, "passthrough", traceID, reqBody)
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
	_, _ = w.Write(bodyBytes)
}

// extractAndRecordAnthropicUsage 从 Anthropic SSE 流的 tailBuf 中提取 usage 并完成计费
//
// Anthropic SSE 流中 output_tokens 出现两次：message_start 里是 0，message_delta 里才是真实值。
// 必须取所有匹配的最大值，而非第一个（否则短流时总会得到 0）。
// input_tokens 在 message_start（流头部），短流时 tailBuf 包含全文可以提取；
// 长流时 tailBuf 只有尾部，此时降级用请求体字节估算。
func extractAndRecordAnthropicUsage(provider string, tailBuf []byte, modelName string, dest *router.MatchedDestination, clientType, methodName, traceID string, reqBody []byte) {
	// output_tokens：取 tailBuf 中所有匹配的最大值
	outputRe := regexp.MustCompile(`"output_tokens"\s*:\s*(\d+)`)
	var outputTokens int64
	for _, m := range outputRe.FindAllSubmatch(tailBuf, -1) {
		var v int64
		_, _ = fmt.Sscanf(string(m[1]), "%d", &v)
		if v > outputTokens {
			outputTokens = v
		}
	}
	if outputTokens == 0 {
		return
	}

	// input_tokens：在 tailBuf 中查找（短流全量可找到），长流时用估算兜底
	var inputTokens int64
	inputRe := regexp.MustCompile(`"input_tokens"\s*:\s*(\d+)`)
	if m := inputRe.FindSubmatch(tailBuf); len(m) > 1 {
		_, _ = fmt.Sscanf(string(m[1]), "%d", &inputTokens)
	}
	if inputTokens == 0 {
		inputTokens = router.EstimatePromptTokens(reqBody)
	}

	// cache_read_input_tokens：Anthropic prompt cache 命中
	var cacheReadTokens int64
	cacheReadRe := regexp.MustCompile(`"cache_read_input_tokens"\s*:\s*(\d+)`)
	if m := cacheReadRe.FindSubmatch(tailBuf); len(m) > 1 {
		_, _ = fmt.Sscanf(string(m[1]), "%d", &cacheReadTokens)
	}

	router.SettleBilling(provider, dest.Node.Name, clientType, methodName, modelName, inputTokens, outputTokens, cacheReadTokens, http.StatusOK, dest, reqBody, traceID)
}

func init() {
	router.RegisterTranslator("anthropic", "anthropic", AnthropicToAnthropic)
}
