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
	"os"
	"path/filepath"
	"strings"
	"time"

	"polaris-gateway/internal/router"
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

	slog.Debug("🔍 [DEBUG] Anthropic Headers", "trace_id", traceID, "headers", fmt.Sprintf("%+v", r.Header))
	
	// 仅在开启 DEBUG 日志级别时，将请求体写入网关根目录（~/.polaris-gateway）供本地排查
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		home, err := os.UserHomeDir()
		var dumpPath string
		if err == nil {
			dumpPath = filepath.Join(home, ".polaris-gateway", "claude_debug_body.json")
		} else {
			dumpPath = "claude_debug_body.json"
		}
		errDump := os.WriteFile(dumpPath, bodyBytes, 0644)
		if errDump == nil {
			slog.Debug(fmt.Sprintf("🔍 [DEBUG] 已将完整的请求体保存到文件: %s", dumpPath), "trace_id", traceID)
		} else {
			slog.Debug("🔍 [DEBUG] 保存请求体到文件失败", "trace_id", traceID, "error", errDump)
		}
	}

	if len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		lastMsgBytes, _ := json.Marshal(lastMsg)
		preview := string(lastMsgBytes)
		if len(preview) > 500 {
			preview = preview[:500] + "... [truncated]"
		}
		slog.Debug("🔍 [DEBUG] Last message preview", "trace_id", traceID, "last_msg", preview)
	} else {
		slog.Debug("🔍 [DEBUG] req.Messages is EMPTY!", "trace_id", traceID)
	}

	// 检测 Claude Code /compact 触发的上下文压缩请求
	// 随着版本更新，Claude Code 已不再发送专用的 compact-2026-01-12 头
	// 为防止未来 Claude Code 修改提示词，这里采用多维度特征检测：
	// 如果最后一条用户消息命中至少 3 个核心摘要/纯文本特征（或包含特定的上下文管理参数），即判定为压缩请求
	isCompact := false
	if len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role == "user" {
			lastMsgBytes, _ := json.Marshal(lastMsg.Content)
			lastMsgStr := string(lastMsgBytes)
			features := 0
			if strings.Contains(lastMsgStr, "TEXT ONLY") { features++ }
			if strings.Contains(strings.ToLower(lastMsgStr), "summary") { features++ }
			if strings.Contains(lastMsgStr, "Do NOT call any tools") { features++ }
			if strings.Contains(lastMsgStr, "<analysis>") { features++ }
			if strings.Contains(lastMsgStr, "<summary>") { features++ }
			
			// Claude Code 或其他官方客户端使用了上下文管理 API (去掉了日期后缀以防版本更新)
			if req.ContextManagement != nil {
				for _, edit := range req.ContextManagement.Edits {
					if strings.HasPrefix(edit.Type, "clear_thinking_") {
						features++
					}
					if strings.HasPrefix(edit.Type, "compact_") {
						features++
					}
				}
			}
			
			if features >= 3 {
				isCompact = true
				slog.Info("🔍 [DEBUG] 成功检测到 /compact 上下文压缩请求！", "trace_id", traceID, "features_hit", features)
			}
		}
	}

	if isCompact {
		// 终极优化：上下文压缩请求只要求模型输出纯文本摘要，
		// 强制剥夺所有可用工具，杜绝 Gemini 被 Instruction 混淆而尝试输出 functionCall 导致 499 报错
		req.Tools = nil
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

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(geapBody))
	proxyReq.Header.Set("Content-Type", "application/json")

	// 透传 Anthropic 扩展功能头：beta 功能（扩展思考、prompt cache）通过 anthropic-beta 头激活
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

	router.ExecuteAndStream(w, proxyReq, dest, "google", clientType, "anthropic_geap_claude", traceID, "Anthropic(GEAP-Claude)",
		func(finalResp *http.Response, startTime time.Time) bool {
			if finalResp.StatusCode != http.StatusOK {
				errBody, _ := io.ReadAll(finalResp.Body)
				finalResp.Body.Close()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(finalResp.StatusCode)
				_, _ = w.Write(errBody)
				return false // 状态码错误已由 CheckResponseStatus 标记，无需额外 streamFailed
			}

			if stream {
				streamGEAPClaude(w, finalResp, dest, clientType, model, traceID, bodyBytes)
			} else {
				nonStreamGEAPClaude(w, finalResp, dest, clientType, model, traceID, bodyBytes)
			}
			return false
		})
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

	tailBuf, _ := router.ForwardStreamBody(w, upstreamResp.Body)

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
	_, _ = w.Write(bodyBytes)
}



// ─── Gemini 转换路径 ─────────────────────────────────────────────────────────
// 请求体：Anthropic 格式 → Gemini GenerateContent 格式（mapToVertexRequest）
// 端点路径：publishers/google/models/{model}:generateContent / :streamGenerateContent
// 响应体：Gemini 格式 → Anthropic 格式（streamAnthropicResponse / handleAnthropicNonStreamResponse）

// handleGemini 将 Anthropic 请求转换为 Gemini GenerateContent 格式后转发
func handleGemini(ctx context.Context, w http.ResponseWriter, bodyBytes []byte, dest *router.MatchedDestination, traceID, model string, req MessageRequest, isCompact bool) {
	const clientType = "Anthropic-Adapter"

	vReq, _ := mapToVertexRequest(req, model)
	if isCompact {
		// 用户的终极重构方案：借鉴之前我们处理 thoughtSignature 缺失的思路
		// 直接把完整的 Claude Code 请求（包含系统提示和所有的历史消息、工具调用记录）
		// 序列化成一段纯文本/XML/JSON 包装起来，作为一个单纯的 user 文本消息发给 Gemini！
		// 这样 Gemini 就再也不会受到复杂的 history 结构、未知的工具调用、角色交替等因素影响，
		// 从而彻底解决它拒绝生成摘要（返回空响应）或报错的问题！
		
		var sb strings.Builder
		for _, msg := range req.Messages {
			sb.WriteString(fmt.Sprintf("<turn role=\"%s\">\n", msg.Role))
			switch c := msg.Content.(type) {
			case string:
				sb.WriteString(c)
			case []interface{}:
				for _, item := range c {
					if m, ok := item.(map[string]interface{}); ok {
						if t, ok := m["type"].(string); ok {
							switch t {
							case "text":
								if text, ok := m["text"].(string); ok {
									sb.WriteString(text)
									sb.WriteString("\n")
								}
							case "tool_use":
								name, _ := m["name"].(string)
								input, _ := json.Marshal(m["input"])
								sb.WriteString(fmt.Sprintf("[Tool Use: %s, Args: %s]\n", name, string(input)))
							case "tool_result":
								content, _ := json.Marshal(m["content"])
								sb.WriteString(fmt.Sprintf("[Tool Result: %s]\n", string(content)))
							}
						}
					}
				}
			}
			sb.WriteString("\n</turn>\n")
		}
		historyXML := sb.String()
		systemPrompt := flattenAnthropicSystem(req.System)
		promptInjection := fmt.Sprintf("System Context: %s\n\n<conversation_history>\n%s\n</conversation_history>\n\nSystem Task: You are performing a context compaction. Please distill the conversation history above into a highly compressed, concise summary. Focus strictly on preserving critical facts, the user's main intent, important context, and any established rules or constraints. Discard all conversational fluff, routine tool outputs, and redundant steps.", systemPrompt, historyXML)
		
		// Preserve all other settings (like thinkingConfig, safetySettings, labels) from mapToVertexRequest
		vReq["contents"] = []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": promptInjection},
				},
			},
		}
		
		delete(vReq, "systemInstruction")
		delete(vReq, "tools")
		delete(vReq, "toolConfig")
		
		if genCfg, ok := vReq["generationConfig"].(map[string]interface{}); ok {
			genCfg["temperature"] = 0.0
		} else {
			vReq["generationConfig"] = map[string]interface{}{
				"temperature": 0.0,
			}
		}
	}

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

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(vReqBytes))
	proxyReq.Header.Set("Content-Type", "application/json")
	q := proxyReq.URL.Query()
	q.Set("key", dest.Node.Credentials)
	if req.Stream {
		q.Set("alt", "sse")
	}
	proxyReq.URL.RawQuery = q.Encode()

	router.ExecuteAndStream(w, proxyReq, dest, "google", clientType, "anthropic_adapter", traceID, "Anthropic(Gemini)",
		func(finalResp *http.Response, startTime time.Time) bool {
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
				_ = json.NewEncoder(w).Encode(errResp)
				return false
			}

			if req.Stream {
				streamOK := streamAnthropicResponse(ctx, w, finalResp, req, traceID, dest, clientType, model, bodyBytes, isCompact)
				return !streamOK
			}
			handleAnthropicNonStreamResponse(w, finalResp, req, traceID, dest, clientType, model, bodyBytes, isCompact)
			return false
		})
}

func init() {
	router.RegisterTranslator("anthropic", "google", AnthropicToGoogle)
}
