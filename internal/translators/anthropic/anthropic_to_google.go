// Anthropic → Google Agent Platform 转换处理器
//
// AnthropicToGoogle 作为统一入口，根据最终目标模型名自动分流到两条路径：
//
//	claude-* 前缀 → GEAP Claude 直通（rawPredict 端点，保持 Anthropic 原生协议）
//	其他模型名   → Gemini 转换（GenerateContent 端点，完整协议格式转换）
//
// 两条路径共用相同的 GEAP 平台（aiplatform.googleapis.com）和 API Key 认证，
// 区别在于 URL 中的 publisher 段（anthropic / google）以及请求/响应体的协议格式。
//
// GEAP Claude 端点参考：
//
//	https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/claude/use-claude
//
// Gemini GenerateContent 端点参考：
//
//	https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest
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

// anthropicVersionGEAP GEAP 平台要求的 Claude 请求体版本字段，Google 官方文档中写死
const anthropicVersionGEAP = "vertex-2023-10-16"

// isClaudeModel 判断目标模型是否为 GEAP Claude 合作伙伴模型（claude-* 前缀）
func isClaudeModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "claude-")
}

// buildGEAPURL 构建 Google Agent Platform 端点 URL
//
//   - publisher:       "google"（Gemini 路径）或 "anthropic"（Claude 合作伙伴模型路径）
//   - subpath:         如 "models/gemini-2.5-pro:generateContent"、"models/claude-sonnet-4-6:rawPredict"
//   - defaultLocation: Gemini 建议 "global"；Claude 仅在 us-east5/europe-west1/asia-southeast1 可用
//
// 支持自定义 base_url 模板变量 {project_id}, {location}, {subpath}（已含 publisher 时 publisher 参数仅用于默认 URL）
func buildGEAPURL(node *router.NodeState, publisher, subpath, defaultLocation string) string {
	tmpl := node.BaseURL
	if tmpl == "" {
		tmpl = "https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/" + publisher + "/{subpath}"
	}
	location := node.Location
	if location == "" {
		location = defaultLocation
	}
	url := strings.ReplaceAll(tmpl, "{project_id}", node.ProjectID)
	url = strings.ReplaceAll(url, "{location}", location)
	url = strings.ReplaceAll(url, "{subpath}", subpath)
	return url
}

// AnthropicToGoogle 解析请求后按目标模型名分流：
//
//	match=claude-*, target=claude-sonnet-4-6  → GEAP Claude 直通（Google 官方 Claude）
//	match=claude-*, target=gemini-2.5-pro     → Gemini 格式转换
//	match=*, target=""                        → 按客户端 req.Model 前缀自动分流
func AnthropicToGoogle(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type": "error", "error": {"type": "invalid_request_error", "message": "invalid json"}}`, 400)
		return
	}

	extractedBillingHeader := ExtractAndStripBillingHeader(&req)
	if extractedBillingHeader != "" {
		w.Header().Set("X-Anthropic-Billing-Header", extractedBillingHeader)
		bodyBytes, _ = json.Marshal(req)
	}

	// TargetModel（路由映射 target 字段）非空时覆盖客户端模型名
	finalModel := dest.TargetModel
	if finalModel == "" {
		finalModel = req.Model
	}
	useGEAPClaude := isClaudeModel(finalModel)

	if isCountTokensPath(r.URL.Path) {
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}

	// 检测 compact-2026-01-12 beta：Claude Code /compact 触发的上下文压缩请求
	// Gemini 不原生支持 compaction 块，由网关在响应侧合成正确的 compaction 内容块格式
	isCompact := false
	for _, betaVal := range r.Header["Anthropic-Beta"] {
		if strings.Contains(betaVal, "compact-2026-01-12") {
			isCompact = true
			break
		}
	}

	if useGEAPClaude {
		handleGEAPClaude(ctx, w, r, bodyBytes, dest, traceID, finalModel, req.Stream)
	} else {
		handleGemini(ctx, w, bodyBytes, dest, traceID, finalModel, req, isCompact)
	}
}

// ─── GEAP Claude 直通路径 ────────────────────────────────────────────────────
// 请求体：Anthropic 原生（添加 anthropic_version，移除 model 字段）
// 端点路径：publishers/anthropic/models/{model}:rawPredict / :streamRawPredict
// 响应体：Anthropic 原生 SSE，透传无转换

// handleGEAPClaude 将 Anthropic 请求直通到 GEAP Claude 端点
// r 用于透传 anthropic-beta 等扩展头（如 interleaved-thinking-2025-05-14、prompt-caching-2024-07-31）
func handleGEAPClaude(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID, model string, stream bool) {
	const clientType = "Anthropic-GEAP-Claude"

	geapBody, err := rewriteBodyForGEAPClaude(bodyBytes, false, "")
	if err != nil {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"failed to rewrite body"}}`, http.StatusBadRequest)
		return
	}

	var subpath string
	if stream {
		subpath = fmt.Sprintf("models/%s:streamRawPredict", model)
	} else {
		subpath = fmt.Sprintf("models/%s:rawPredict", model)
	}
	targetURL := buildGEAPURL(dest.Node, "anthropic", subpath, "us-east5")

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路 (GEAP-Claude)", "trace_id", traceID, "account", dest.Node.Name)
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(geapBody))
	proxyReq.Header.Set("Content-Type", "application/json")

	// 透传 Anthropic 扩展功能头：beta 功能（扩展思考、prompt cache）通过 anthropic-beta 头激活
	// anthropic-version 已经由 rewriteBodyForGEAPClaude 注入 body 的 anthropic_version 字段，无需重复
	if r != nil {
		for k, vv := range r.Header {
			lk := strings.ToLower(k)
			if lk == "anthropic-beta" {
				for _, v := range vv {
					proxyReq.Header.Add(k, v)
				}
			}
		}
	}

	q := proxyReq.URL.Query()
	q.Set("key", dest.Node.Credentials)
	proxyReq.URL.RawQuery = q.Encode()

	finalResp, err := httpClient.Do(proxyReq)
	if err != nil {
		utils.HandleNetworkError(w, err, dest, "google", clientType, "anthropic_geap_claude", traceID, "Anthropic(GEAP-Claude)")
		return
	}

	isNodeFailure, isQuotaExhausted := utils.CheckResponseStatus(finalResp, dest, "google", clientType, "anthropic_geap_claude", traceID, "Anthropic(GEAP-Claude)")

	if finalResp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(finalResp.Body)
		finalResp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(finalResp.StatusCode)
		w.Write(errBody)
		utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
		return
	}

	if stream {
		streamGEAPClaude(w, finalResp, dest, clientType, model, traceID, bodyBytes)
	} else {
		nonStreamGEAPClaude(w, finalResp, dest, clientType, model, traceID, bodyBytes)
	}
	utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
}

// rewriteBodyForGEAPClaude 注入 anthropic_version，删除 model 字段（model 在 URL 中指定）
// countTokens 场景保留 model 字段且删除推理参数（count-tokens 端点的额外限制）
func rewriteBodyForGEAPClaude(bodyBytes []byte, isCountTokens bool, targetModel string) ([]byte, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &m); err != nil {
		return nil, err
	}
	m["anthropic_version"] = anthropicVersionGEAP
	if isCountTokens {
		if targetModel != "" {
			m["model"] = targetModel
		}
		// count-tokens 端点只接受 messages/system/tools/tool_choice/thinking/model/anthropic_version
		// 推理参数和流控字段全部删除
		delete(m, "stream")
		delete(m, "max_tokens")
		delete(m, "temperature")
		delete(m, "top_p")
		delete(m, "top_k")
		delete(m, "stop_sequences")
		delete(m, "metadata")
	} else {
		delete(m, "model")
	}
	return json.Marshal(m)
}

// streamGEAPClaude 透传 GEAP Claude SSE 流（Anthropic 原生事件格式，无需转换）
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

	tailBuf, _ := utils.ForwardStreamBody(w, upstreamResp.Body)

	if bytes.Contains(tailBuf, []byte("output_tokens")) {
		extractAndRecordAnthropicUsage("google", tailBuf, modelName, dest, clientType, "anthropic_geap_claude", traceID, reqBody)
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
	parseAndSettleAnthropicResponse("google", bodyBytes, dest, clientType, "anthropic_geap_claude", modelName, traceID, upstreamResp.StatusCode, reqBody)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(upstreamResp.StatusCode)
	w.Write(bodyBytes)
}

// handleGEAPClaudeCountTokens 调用 GEAP Claude count-tokens 端点获取精确计数，失败时降级本地估算
// 端点：publishers/anthropic/models/count-tokens:rawPredict
func handleGEAPClaudeCountTokens(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string, model string) {
	geapBody, err := rewriteBodyForGEAPClaude(bodyBytes, true, model)
	if err != nil {
		slog.Warn("⚠️ [CountTokens-GEAP-Claude] body 重写失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}

	targetURL := buildGEAPURL(dest.Node, "anthropic", "models/count-tokens:rawPredict", "us-east5")

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(geapBody))
	if err != nil {
		slog.Warn("⚠️ [CountTokens-GEAP-Claude] 请求构建失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	// 同样透传 anthropic-beta 头，确保 extended thinking 等 beta 特性在计数时得到正确统计
	if r != nil {
		for k, vv := range r.Header {
			if strings.EqualFold(k, "anthropic-beta") {
				for _, v := range vv {
					proxyReq.Header.Add(k, v)
				}
			}
		}
	}
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

// ─── Gemini 转换路径 ─────────────────────────────────────────────────────────
// 请求体：Anthropic 格式 → Gemini GenerateContent 格式（mapToVertexRequest）
// 端点路径：publishers/google/models/{model}:generateContent / :streamGenerateContent
// 响应体：Gemini 格式 → Anthropic 格式（streamAnthropicResponse / handleAnthropicNonStreamResponse）

// handleGemini 将 Anthropic 请求转换为 Gemini GenerateContent 格式后转发
func handleGemini(ctx context.Context, w http.ResponseWriter, bodyBytes []byte, dest *router.MatchedDestination, traceID, model string, req MessageRequest, isCompact bool) {
	const clientType = "Anthropic-Adapter"

	vReq, _ := mapToVertexRequest(req, model)
	vReqBytes, _ := json.Marshal(vReq)

	if model == "" {
		model = "gemini-3.1-pro-preview"
	}

	var subpath string
	if req.Stream {
		subpath = fmt.Sprintf("models/%s:streamGenerateContent", model)
	} else {
		subpath = fmt.Sprintf("models/%s:generateContent", model)
	}
	targetURL := buildGEAPURL(dest.Node, "google", subpath, "global")

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路 (Gemini)", "trace_id", traceID, "account", dest.Node.Name)
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(vReqBytes))
	proxyReq.Header.Set("Content-Type", "application/json")
	q := proxyReq.URL.Query()
	q.Set("key", dest.Node.Credentials)
	if req.Stream {
		q.Set("alt", "sse")
	}
	proxyReq.URL.RawQuery = q.Encode()

	finalResp, err := httpClient.Do(proxyReq)
	if err != nil {
		utils.HandleNetworkError(w, err, dest, "google", clientType, "anthropic_adapter", traceID, "Anthropic(Gemini)")
		return
	}

	isNodeFailure, isQuotaExhausted := utils.CheckResponseStatus(finalResp, dest, "google", clientType, "anthropic_adapter", traceID, "Anthropic(Gemini)")

	if finalResp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(finalResp.Body)
		finalResp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(finalResp.StatusCode)
		errResp := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "api_error",
				"message": fmt.Sprintf("Google Agent Platform API returned status %d: %s", finalResp.StatusCode, string(errBody)),
			},
		}
		json.NewEncoder(w).Encode(errResp)
		utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
		return
	}

	if req.Stream {
		streamOK := streamAnthropicResponse(ctx, w, finalResp, req, traceID, dest, clientType, model, bodyBytes, isCompact)
		if !streamOK {
			isNodeFailure = true
		}
	} else {
		handleAnthropicNonStreamResponse(w, finalResp, req, traceID, dest, clientType, model, bodyBytes, isCompact)
	}
	utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
}

func init() {
	router.RegisterTranslator("anthropic", "google", AnthropicToGoogle)
}
