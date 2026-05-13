// Anthropic → Gemini Enterprise Agent Platform (Claude 合作伙伴模型) 直通处理器
//
// 当目标模型是 claude-* 时，GEAP 平台提供以下原生端点：
//   - 非流式: POST aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/anthropic/models/{model}:rawPredict
//   - 流式:   POST aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/anthropic/models/{model}:streamRawPredict
//   - 计数:   POST aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/anthropic/models/count-tokens:rawPredict
//
// 这些端点接收 Anthropic 原生请求体（仅需添加 anthropic_version 字段、剥离 model 字段），
// 返回 Anthropic 原生响应（含 SSE 流），从而避免 Anthropic→Gemini→Anthropic 双向转换的语义损失
//
// 参考文档：
//   https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/claude/use-claude
//   https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/claude/count-tokens
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

	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// anthropicVersionGEAP GEAP 平台要求的 Claude 请求体版本字段
// 此版本号在 Google 官方文档中明确写死，目前尚无可选值
const anthropicVersionGEAP = "vertex-2023-10-16"

// isClaudeModel 判断目标模型是否为 Anthropic Claude
// GEAP 上 Claude 模型 ID 一律以 "claude-" 前缀开头（claude-opus-4-7、claude-sonnet-4-6 等）
func isClaudeModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "claude-")
}

// claudeGEAPLocation 选择 Claude 模型可用的 GEAP 区域
// 当前 Claude 仅在 us-east5/europe-west1/asia-southeast1 三个区域可用，默认 us-east5
func claudeGEAPLocation(node *router.NodeState) string {
	if node.Location != "" {
		return node.Location
	}
	return "us-east5"
}

// buildGEAPClaudeURL 构造 GEAP 上 Claude 模型的端点 URL
//   - method ∈ {rawPredict, streamRawPredict}
//   - countTokens=true 时 model 段强制为字面量 "count-tokens"
func buildGEAPClaudeURL(node *router.NodeState, model string, stream bool, isCountTokens bool) string {
	location := claudeGEAPLocation(node)

	template := node.BaseURL
	if template == "" {
		template = "https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/anthropic/{subpath}"
	}

	var subpath string
	if isCountTokens {
		subpath = "models/count-tokens:rawPredict"
	} else if stream {
		subpath = fmt.Sprintf("models/%s:streamRawPredict", model)
	} else {
		subpath = fmt.Sprintf("models/%s:rawPredict", model)
	}

	url := strings.ReplaceAll(template, "{project_id}", node.ProjectID)
	url = strings.ReplaceAll(url, "{location}", location)
	url = strings.ReplaceAll(url, "{subpath}", subpath)

	return url
}

// rewriteBodyForGEAPClaude 重写 Anthropic 请求体以符合 GEAP 要求
// 改动：注入 anthropic_version、删除 model 字段（model 在 URL 中指定）
// countTokens 场景下保留 model 字段，因为 count-tokens 端点需要从 body 区分目标模型
func rewriteBodyForGEAPClaude(bodyBytes []byte, isCountTokens bool, targetModel string) ([]byte, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &m); err != nil {
		return nil, err
	}
	m["anthropic_version"] = anthropicVersionGEAP
	if isCountTokens {
		// count-tokens 端点需要 body 里有 model 字段指明目标模型
		if targetModel != "" {
			m["model"] = targetModel
		}
		// count_tokens 不该带 stream/max_tokens 等推理参数
		delete(m, "stream")
		delete(m, "max_tokens")
		delete(m, "temperature")
		delete(m, "top_p")
		delete(m, "top_k")
		delete(m, "stop_sequences")
	} else {
		// rawPredict 端点要求 body 中没有 model 字段（GEAP 校验会拒绝）
		delete(m, "model")
	}
	return json.Marshal(m)
}

// passthroughToGEAPClaude 把 Anthropic 请求直通到 GEAP 的 :rawPredict / :streamRawPredict 端点
// 全程透传：请求体只做最小化改写，响应保持 Anthropic 原生格式（含 SSE 事件流）
func passthroughToGEAPClaude(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string, model string) {
	clientType := "Anthropic-GEAP-Claude"

	// 解析 Stream 标志以决定端点和后续处理路径
	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"invalid json"}}`, http.StatusBadRequest)
		return
	}

	geapBody, err := rewriteBodyForGEAPClaude(bodyBytes, false, "")
	if err != nil {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"failed to rewrite body"}}`, http.StatusBadRequest)
		return
	}

	targetURL := buildGEAPClaudeURL(dest.Node, model, req.Stream, false)

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路 (Anthropic GEAP-Claude)", "trace_id", traceID, "account", dest.Node.Name)
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(geapBody))
	proxyReq.Header.Set("Content-Type", "application/json")
	q := proxyReq.URL.Query()
	q.Set("key", dest.Node.Credentials)
	proxyReq.URL.RawQuery = q.Encode()

	finalResp, err := httpClient.Do(proxyReq)
	if err != nil {
		utils.HandleNetworkError(w, err, dest, "vertex", clientType, "anthropic_geap_claude", traceID, "Anthropic(GEAP-Claude)")
		return
	}

	isNodeFailure, isQuotaExhausted := utils.CheckResponseStatus(finalResp, dest, "vertex", clientType, "anthropic_geap_claude", traceID, "Anthropic(GEAP-Claude)")

	if finalResp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(finalResp.Body)
		finalResp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(finalResp.StatusCode)
		w.Write(errBody)
		utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
		return
	}

	if req.Stream {
		streamGEAPClaude(w, finalResp, dest, clientType, model, traceID, bodyBytes)
	} else {
		nonStreamGEAPClaude(w, finalResp, dest, clientType, model, traceID, bodyBytes)
	}

	utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
}

// streamGEAPClaude 透传 GEAP Claude SSE 流到客户端
// GEAP 返回的 SSE 事件与 Anthropic 官方完全一致：message_start → content_block_* → message_delta → message_stop
// 仅在尾部 buffer 中解析 output_tokens 用于计费
func streamGEAPClaude(w http.ResponseWriter, upstreamResp *http.Response, dest *router.MatchedDestination, clientType, modelName, traceID string, reqBody []byte) {
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

	// 复用 anthropic_to_anthropic.go 中的 usage 提取逻辑
	if bytes.Contains(tailBuf, []byte("output_tokens")) {
		extractAndRecordAnthropicUsage(tailBuf, modelName, dest, clientType, "anthropic_geap_claude", traceID, reqBody)
	}
}

// nonStreamGEAPClaude 透传 GEAP Claude 非流式响应并提取 usage 完成计费
func nonStreamGEAPClaude(w http.ResponseWriter, upstreamResp *http.Response, dest *router.MatchedDestination, clientType, modelName, traceID string, reqBody []byte) {
	defer upstreamResp.Body.Close()
	bodyBytes, err := io.ReadAll(upstreamResp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	var resp struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(bodyBytes, &resp) == nil {
		settleBilling("vertex", dest.Node.Name, clientType, "anthropic_geap_claude", modelName,
			int64(resp.Usage.InputTokens), int64(resp.Usage.OutputTokens), 0,
			upstreamResp.StatusCode, dest, reqBody, traceID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(upstreamResp.StatusCode)
	w.Write(bodyBytes)
}

// handleGEAPClaudeCountTokens 调用 GEAP 上 Claude 模型的 count-tokens 端点获取精确计数
// 端点：publishers/anthropic/models/count-tokens:rawPredict
// 请求体格式：Anthropic 原生 + anthropic_version + model 字段
// 响应：{"input_tokens": N}
// 失败时降级到本地估算
func handleGEAPClaudeCountTokens(ctx context.Context, w http.ResponseWriter, bodyBytes []byte, dest *router.MatchedDestination, traceID string, model string) {
	geapBody, err := rewriteBodyForGEAPClaude(bodyBytes, true, model)
	if err != nil {
		slog.Warn("⚠️ [CountTokens-GEAP-Claude] body 重写失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}

	targetURL := buildGEAPClaudeURL(dest.Node, model, false, true)

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(geapBody))
	if err != nil {
		slog.Warn("⚠️ [CountTokens-GEAP-Claude] 请求构建失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	q := proxyReq.URL.Query()
	q.Set("key", dest.Node.Credentials)
	proxyReq.URL.RawQuery = q.Encode()

	resp, err := httpClient.Do(proxyReq)
	if err != nil {
		slog.Warn("⚠️ [CountTokens-GEAP-Claude] 调用失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		slog.Warn("⚠️ [CountTokens-GEAP-Claude] 上游非 200，降级本地估算",
			"trace_id", traceID, "status", resp.StatusCode,
			"body_preview", string(respBody[:min(len(respBody), 200)]))
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}

	slog.Debug("📏 [CountTokens-GEAP-Claude] 上游精确计数", "trace_id", traceID, "model", model)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}
