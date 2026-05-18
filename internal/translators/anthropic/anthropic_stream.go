// Anthropic 响应流式/非流式处理 + SSE 写入工具
// 从 Google Agent Platform 后端读取 GenerateContentResponse，实时转换为 Anthropic SSE 格式并推送给客户端
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// streamAnthropicResponse 从 Vertex 后端读取流式 SSE 响应，边读边转为 Anthropic SSE 格式
// 事件序列: message_start → content_block_start → content_block_delta* → content_block_stop → message_delta → message_stop
// 同时在最后解析 usageMetadata 完成计费结算
// 返回 streamOK: false 表示流式传输中发生不可恢复错误（Vertex 返回错误、IO 断联等），调用方应标记节点失败
func streamAnthropicResponse(ctx context.Context, w http.ResponseWriter, vertexResp *http.Response, req MessageRequest, traceID string, dest *router.MatchedDestination, clientType, modelName string, reqBody []byte) (streamOK bool) {
	defer vertexResp.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	// 用本地估算填充 message_start 的 input_tokens，让 /context 命令能在首事件就显示进度
	// usageMetadata 抵达后，下方 message_delta 会以精确值覆盖
	estimatedInput := estimateAnthropicTokens(req)
	writeSSEMessageStart(w, flusher, traceID, modelName, estimatedInput)

	reader := bufio.NewReader(vertexResp.Body)
	var promptTokens, completionTokens, cachedTokens int
	var totalWritten int64

	blockIndex := 0
	inText := false
	inThinking := false       // 追踪是否有开放中的 thinking 内容块（用于合并多个 thought 分片）
	var thoughtSig string     // 当前 thinking 块对应的 thoughtSignature（Gemini 返回，转存为 Anthropic signature）
	var toolID string
	stopReason := "end_turn"
	var matchedStopSeq string // 触发停止的序列（Gemini 不直接返回，通过文本尾部推断）
	var streamError string    // 流中途出错信息

	for {
		// 客户端断开时 r.Context() 被取消，提前退出避免继续消耗上游连接
		if ctx.Err() != nil {
			slog.Debug("🔌 [Stream] 客户端已断开，终止 GEAP 流式响应", "trace_id", traceID, "account", dest.Node.Name)
			return true
		}

		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			totalWritten += int64(len(line))
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			// Non-EOF IO error: connection reset, timeout, etc.
			streamError = fmt.Sprintf("stream read error: %v", err)
			slog.Error("❌ [Stream] GEAP SSE 流读取失败，连接可能中断", "trace_id", traceID, "account", dest.Node.Name, "error", err.Error())
			break
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		data := bytes.TrimPrefix(line, []byte("data: "))
		if string(data) == "[DONE]" {
			break
		}

		var vResp map[string]interface{}
		if err := json.Unmarshal(data, &vResp); err != nil {
			slog.Warn("⚠️ [Stream] GEAP SSE 行 JSON 解析失败", "trace_id", traceID, "account", dest.Node.Name, "data_preview", string(data[:min(len(data), 200)]))
			continue
		}

		// Detect GEAP API error inside SSE stream (e.g., mid-stream rate limit)
		if errData, ok := vResp["error"].(map[string]interface{}); ok {
			errMsg := "GEAP API error in stream"
			if msg, ok := errData["message"].(string); ok {
				errMsg = msg
			}
			streamError = errMsg
			slog.Error("❌ [Stream] GEAP 在流中返回 API 错误（可能是中途触发限流）", "trace_id", traceID, "account", dest.Node.Name, "error", errMsg)
			break
		}

		// Detect promptFeedback block (content policy refusal)
		if pf, ok := vResp["promptFeedback"].(map[string]interface{}); ok {
			if blockReason, ok := pf["blockReason"].(string); ok && blockReason != "" {
				streamError = fmt.Sprintf("request blocked by GEAP safety filter: %s", blockReason)
				slog.Error("❌ [Stream] GEAP promptFeedback 阻断请求", "trace_id", traceID, "account", dest.Node.Name, "block_reason", blockReason)
				break
			}
		}

		// 解析 usageMetadata（最终 chunk 才有完整数值，每次更新覆盖旧值）
		// thoughtsTokenCount：思考 token 计入 output_tokens（Anthropic 将 thinking 并入 output 计费）
		// toolUsePromptTokenCount：工具定义 token 计入 input_tokens（属于 prompt 端消耗）
		if usage, ok := vResp["usageMetadata"].(map[string]interface{}); ok {
			if p, ok := usage["promptTokenCount"].(float64); ok {
				promptTokens = int(p)
			}
			if tool, ok := usage["toolUsePromptTokenCount"].(float64); ok {
				promptTokens += int(tool)
			}
			if c, ok := usage["candidatesTokenCount"].(float64); ok {
				completionTokens = int(c)
			}
			if thoughts, ok := usage["thoughtsTokenCount"].(float64); ok {
				completionTokens += int(thoughts)
			}
			if cache, ok := usage["cachedContentTokenCount"].(float64); ok {
				cachedTokens = int(cache)
			}
		}

		candidates, ok := vResp["candidates"].([]interface{})
		if !ok || len(candidates) == 0 {
			continue
		}

		cand, _ := candidates[0].(map[string]interface{})
		
		if finishReason, ok := cand["finishReason"].(string); ok && finishReason != "" {
			switch finishReason {
			case "MAX_TOKENS":
				stopReason = "max_tokens"
			case "STOP":
				// STOP 可能是自然结束或命中 stopSequences
				// Gemini 不返回具体命中的序列，仅当请求有 stopSequences 时标记类型
				if len(req.StopSequences) > 0 {
					stopReason = "stop_sequence"
					_ = matchedStopSeq // 保持空字符串（Gemini 不返回命中的具体序列）
				}
			case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII",
				"IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT", "IMAGE_RECITATION", "IMAGE_OTHER",
				"NO_IMAGE", "OTHER":
				// 安全过滤器、版权、图片生成失败等——映射为 end_turn 让客户端优雅降级
				stopReason = "end_turn"
				slog.Warn("⚠️ [Stream] GEAP 非正常停止原因", "trace_id", traceID, "account", dest.Node.Name, "finish_reason", finishReason)
			case "MALFORMED_FUNCTION_CALL", "UNEXPECTED_TOOL_CALL":
				// 模型尝试了工具调用但格式错误/非预期——实际的 tool_use 内容块由 part 处理决定
				// 此处保持 end_turn；若 part 里确实有 functionCall，part 处理会覆盖为 tool_use
				stopReason = "end_turn"
				slog.Warn("⚠️ [Stream] GEAP 工具调用格式异常", "trace_id", traceID, "account", dest.Node.Name, "finish_reason", finishReason)
			default:
				stopReason = "end_turn"
			}
		}

		content, ok := cand["content"].(map[string]interface{})
		if !ok {
			continue
		}

		parts, ok := content["parts"].([]interface{})
		if !ok || len(parts) == 0 {
			continue
		}

		for _, partIntf := range parts {
			part, _ := partIntf.(map[string]interface{})

			// Gemini 在启用 thinkingConfig.includeThoughts 时，以 thought=true 的 part 返回推理过程。
			// Gemini 流式可能把同一次思考分成多个 chunk 推送，全部合并到同一个 Anthropic thinking 块
			// 以符合 Anthropic SSE 协议（一次完整思考对应一个 thinking block，内含多个 thinking_delta）。
			// thoughtSignature：Gemini 返回的不透明签名，客户端下一轮需要原样传回以维持思考连贯性；
			// 对应 Anthropic 的 signature_delta 事件，在 content_block_stop 之前发出。
			if isThought, _ := part["thought"].(bool); isThought {
				// 无论客户端是否显式设置 thinking，只要 Gemini 返回 thought part 就处理
				// （Gemini 2.5-pro 可能在 thinkingBudget 未显式禁用时自动思考）

				// 优先捕获 thoughtSignature（可能在任意 chunk 出现，只需记录最新值）
				if sig, ok := part["thoughtSignature"].(string); ok && sig != "" {
					thoughtSig = sig
				}

				if thinkText, ok := part["text"].(string); ok && thinkText != "" {
					// 有正文块打开时先关闭（thinking 必须排在 text 之前）
					if inText {
						writeSSEContentBlockStop(w, flusher, blockIndex)
						inText = false
						blockIndex++
					}
					// 首个 thought chunk：开启 thinking 块
					if !inThinking {
						writeSSE(w, flusher, "content_block_start", StreamEvent{
							Type:  "content_block_start",
							Index: ptrInt(blockIndex),
							ContentBlock: &Content{
								Type:     "thinking",
								Thinking: "",
							},
						})
						inThinking = true
					}
					// 追加 thinking_delta（多 chunk 持续写入同一块）
					writeSSE(w, flusher, "content_block_delta", StreamEvent{
						Type:  "content_block_delta",
						Index: ptrInt(blockIndex),
						Delta: &Delta{
							Type:     "thinking_delta",
							Thinking: thinkText,
						},
					})
				}
				continue
			}

			// 遇到非 thought part 时，关闭尚未关闭的 thinking 块
			// 关闭前先发 signature_delta（Anthropic 要求 signature 在 content_block_stop 之前）
			if inThinking {
				if thoughtSig != "" {
					writeSSE(w, flusher, "content_block_delta", StreamEvent{
						Type:  "content_block_delta",
						Index: ptrInt(blockIndex),
						Delta: &Delta{
							Type:      "signature_delta",
							Signature: thoughtSig,
						},
					})
					thoughtSig = ""
				}
				writeSSEContentBlockStop(w, flusher, blockIndex)
				inThinking = false
				blockIndex++
			}

			if text, ok := part["text"].(string); ok {
				if !inText {
					writeSSE(w, flusher, "content_block_start", StreamEvent{
						Type:  "content_block_start",
						Index: ptrInt(blockIndex),
						ContentBlock: &Content{
							Type: "text",
							Text: "",
						},
					})
					inText = true
				}

				if text != "" {
					writeSSE(w, flusher, "content_block_delta", StreamEvent{
						Type:  "content_block_delta",
						Index: ptrInt(blockIndex),
						Delta: &Delta{
							Type: "text_delta",
							Text: text,
						},
					})
				}
			}

			if fc, ok := part["functionCall"].(map[string]interface{}); ok {
				if inText {
					writeSSE(w, flusher, "content_block_stop", StreamEvent{
						Type:  "content_block_stop",
						Index: ptrInt(blockIndex),
					})
					inText = false
					blockIndex++
				}

				name, _ := fc["name"].(string)
				if name == "" {
					slog.Warn("⚠️ [Stream] functionCall 缺少 name 字段，跳过", "trace_id", traceID)
					continue
				}
				toolID = fmt.Sprintf("toolu_%s_%d", traceID, blockIndex)
				
				writeSSE(w, flusher, "content_block_start", StreamEvent{
					Type:  "content_block_start",
					Index: ptrInt(blockIndex),
					ContentBlock: &Content{
						Type:  "tool_use",
						ID:    toolID,
						Name:  name,
						Input: struct{}{}, // Ensures "input": {} instead of missing field due to omitempty
					},
				})
				
				argsBytes := normalizeFunctionCallArgs(fc["args"])
				
				argsRunes := []rune(string(argsBytes))
				chunkSize := 40
				for i := 0; i < len(argsRunes); i += chunkSize {
					end := i + chunkSize
					if end > len(argsRunes) {
						end = len(argsRunes)
					}
					
					writeSSE(w, flusher, "content_block_delta", StreamEvent{
						Type:  "content_block_delta",
						Index: ptrInt(blockIndex),
						Delta: &Delta{
							Type:        "input_json_delta",
							PartialJson: string(argsRunes[i:end]),
						},
					})
				}
				
				writeSSEContentBlockStop(w, flusher, blockIndex)

				blockIndex++
				stopReason = "tool_use"
			}
		}
	}

	// 流式正常结束但无任何内容块 → 注入空文本块兜底，防止 Claude Code /compact 等操作报错
	if streamError == "" && blockIndex == 0 && !inText && !inThinking {
		slog.Warn("⚠️ [Stream] GEAP 流式响应未包含任何内容块，注入空文本块兜底",
			"trace_id", traceID, "account", dest.Node.Name,
			"stop_reason", stopReason, "prompt_tokens", promptTokens)
		
		writeSSE(w, flusher, "content_block_start", StreamEvent{
			Type:  "content_block_start",
			Index: ptrInt(blockIndex),
			ContentBlock: &Content{
				Type: "text",
				Text: "",
			},
		})
		writeSSEContentBlockStop(w, flusher, blockIndex)
		blockIndex++
	}

	if streamError != "" {
		// 先关闭所有开放中的内容块，让客户端能干净解析已收到的部分
		if inThinking {
			if thoughtSig != "" {
				writeSSE(w, flusher, "content_block_delta", StreamEvent{
					Type:  "content_block_delta",
					Index: ptrInt(blockIndex),
					Delta: &Delta{Type: "signature_delta", Signature: thoughtSig},
				})
			}
			writeSSEContentBlockStop(w, flusher, blockIndex)
		}
		if inText {
			writeSSEContentBlockStop(w, flusher, blockIndex)
		}
		// Anthropic 错误事件格式：event: error / data: {"type":"error","error":{"type":"...","message":"..."}}
		// 用 json.Marshal 保证 message 中的特殊字符正确转义
		errPayload, _ := json.Marshal(map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "api_error",
				"message": streamError,
			},
		})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", errPayload)
		if flusher != nil {
			flusher.Flush()
		}
		return false
	}

	// 关闭所有尚未关闭的内容块
	// thinking 块关闭前先发 signature_delta，让客户端在多轮对话时能正确传回签名
	if inThinking {
		if thoughtSig != "" {
			writeSSE(w, flusher, "content_block_delta", StreamEvent{
				Type:  "content_block_delta",
				Index: ptrInt(blockIndex),
				Delta: &Delta{
					Type:      "signature_delta",
					Signature: thoughtSig,
				},
			})
		}
		writeSSEContentBlockStop(w, flusher, blockIndex)
		blockIndex++
	}
	if inText {
		writeSSEContentBlockStop(w, flusher, blockIndex)
	}

	// message_delta：Anthropic 协议要求附带最终 stop_reason 与精确 usage
	// Gemini 的 cachedContentTokenCount 映射成 cache_read_input_tokens，
	// Claude Code 的 /cost 命令据此识别 prompt cache 命中以反映真实费用
	msgDeltaEvent := StreamEvent{
		Type: "message_delta",
		Delta: &Delta{
			StopReason: stopReason,
		},
		Usage: &Usage{
			InputTokens:          promptTokens,
			OutputTokens:         completionTokens,
			CacheReadInputTokens: cachedTokens,
		},
	}
	writeSSE(w, flusher, "message_delta", msgDeltaEvent)

	writeSSEMessageStop(w, flusher)

	// GEAP 偶发不返回 usageMetadata（极少见），此时用字节估算兜底保证计费不丢失
	// 注：!streamOK 在此处永远为 false（命名返回值尚未赋值），去掉该条件让逻辑更清晰
	if promptTokens == 0 && completionTokens == 0 {
		promptTokens = int(utils.EstimatePromptTokens(reqBody))
		completionTokens = int(utils.EstimateCompletionTokens(totalWritten))
		slog.Warn("⚠️ GEAP 未返回 usageMetadata，启用 token 估算兜底", "trace_id", traceID, "node", dest.Node.Name, "prompt", promptTokens, "completion", completionTokens)
	}

	settleBilling("google", dest.Node.Name, clientType, "anthropic_adapter", modelName, int64(promptTokens), int64(completionTokens), int64(cachedTokens), http.StatusOK, dest, reqBody, traceID)

	return true
}

// handleAnthropicNonStreamResponse 处理 Google Agent Platform 非流式响应，提取文本和用量，转为 Anthropic JSON 格式返回
func handleAnthropicNonStreamResponse(w http.ResponseWriter, vertexResp *http.Response, req MessageRequest, traceID string, dest *router.MatchedDestination, clientType, modelName string, reqBody []byte) {
	defer vertexResp.Body.Close()
	bodyBytes, err := io.ReadAll(vertexResp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	var vResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &vResp); err != nil {
		http.Error(w, "Invalid response from Vertex", http.StatusBadGateway)
		return
	}

	var promptTokens, completionTokens, cachedTokens int
	if usage, ok := vResp["usageMetadata"].(map[string]interface{}); ok {
		if p, ok := usage["promptTokenCount"].(float64); ok {
			promptTokens = int(p)
		}
		if tool, ok := usage["toolUsePromptTokenCount"].(float64); ok {
			promptTokens += int(tool) // 工具定义 token 属于 prompt 端消耗
		}
		if c, ok := usage["candidatesTokenCount"].(float64); ok {
			completionTokens = int(c)
		}
		if thoughts, ok := usage["thoughtsTokenCount"].(float64); ok {
			completionTokens += int(thoughts) // 思考 token 并入 output_tokens
		}
		if cache, ok := usage["cachedContentTokenCount"].(float64); ok {
			cachedTokens = int(cache)
		}
	}

	// Detect promptFeedback block (safety refusal before any candidates)
	if pf, ok := vResp["promptFeedback"].(map[string]interface{}); ok {
		if blockReason, ok := pf["blockReason"].(string); ok && blockReason != "" {
			slog.Error("❌ [NonStream] GEAP promptFeedback 阻断请求", "trace_id", traceID, "account", dest.Node.Name, "block_reason", blockReason)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": fmt.Sprintf("request blocked by GEAP safety filter: %s", blockReason),
				},
			})
			return
		}
	}

	contents := []Content{}
	stopReason := "end_turn"

	if candidates, ok := vResp["candidates"].([]interface{}); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]interface{}); ok {
			// finishReason 映射与流式分支保持一致，避免客户端 stop_reason 检查不一致
			if finishReason, ok := cand["finishReason"].(string); ok && finishReason != "" {
				switch finishReason {
				case "MAX_TOKENS":
					stopReason = "max_tokens"
				case "STOP":
					if len(req.StopSequences) > 0 {
						stopReason = "stop_sequence"
					}
				case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII",
					"IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT", "IMAGE_RECITATION", "IMAGE_OTHER",
					"NO_IMAGE", "OTHER":
					stopReason = "end_turn"
					slog.Warn("⚠️ [NonStream] GEAP 非正常停止原因",
						"trace_id", traceID, "account", dest.Node.Name, "finish_reason", finishReason)
				case "MALFORMED_FUNCTION_CALL", "UNEXPECTED_TOOL_CALL":
					stopReason = "end_turn"
					slog.Warn("⚠️ [NonStream] GEAP 工具调用格式异常",
						"trace_id", traceID, "account", dest.Node.Name, "finish_reason", finishReason)
				}
			}
			if content, ok := cand["content"].(map[string]interface{}); ok {
				if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
					toolIdx := 0
					for _, partIntf := range parts {
						part, _ := partIntf.(map[string]interface{})
						// Gemini thought 部分 → Anthropic thinking 内容块
						// 无论客户端是否显式设置 thinking，只要 Gemini 返回 thought part 就转换
						// thoughtSignature 对应 Anthropic thinking.signature，客户端下一轮须原样传回
						if isThought, _ := part["thought"].(bool); isThought {
							thinkText, _ := part["text"].(string)
							sig, _ := part["thoughtSignature"].(string)
							if thinkText != "" {
								contents = append(contents, Content{
									Type:      "thinking",
									Thinking:  thinkText,
									Signature: sig,
								})
							}
							continue
						}
						if t, ok := part["text"].(string); ok {
							// 允许空字符串，避免 contents 为空导致 Claude Code /compact 报错 "empty response"
							contents = append(contents, Content{
								Type: "text",
								Text: t,
							})
						}
						if fc, ok := part["functionCall"].(map[string]interface{}); ok {
							name, _ := fc["name"].(string)
							if name == "" {
								slog.Warn("⚠️ [NonStream] functionCall 缺少 name 字段，跳过", "trace_id", traceID)
								continue
							}
							argsBytes := normalizeFunctionCallArgs(fc["args"])
							var args map[string]interface{}
							if err := json.Unmarshal(argsBytes, &args); err != nil {
								args = make(map[string]interface{})
							}
							contents = append(contents, Content{
								Type:  "tool_use",
								ID:    fmt.Sprintf("toolu_%s_%d", traceID, toolIdx),
								Name:  name,
								Input: args,
							})
							stopReason = "tool_use"
							toolIdx++
						}
					}
				}
			}
		}
	}

	// 判断是否存在真正有意义的内容块：
	// tool_use / thinking 不属于"空"，只有 contents 为空才需要注入兜底
	hasRealContent := false
	for _, c := range contents {
		if c.Type == "tool_use" || c.Type == "thinking" || c.Type == "text" {
			hasRealContent = true
			break
		}
	}
	if !hasRealContent {
		slog.Warn("⚠️ [NonStream] GEAP 返回无有效文本内容（安全过滤或空候选），注入空文本块防止客户端报错",
			"trace_id", traceID, "account", dest.Node.Name,
			"stop_reason", stopReason, "geap_resp_preview", string(bodyBytes[:min(len(bodyBytes), 500)]))
		contents = []Content{
			{Type: "text", Text: ""},
		}
	}

	settleBilling("google", dest.Node.Name, clientType, "anthropic_adapter", modelName, int64(promptTokens), int64(completionTokens), int64(cachedTokens), vertexResp.StatusCode, dest, reqBody, traceID)

	anthropicResp := MessageResponse{
		ID:           fmt.Sprintf("msg_%s", traceID),
		Type:         "message",
		Role:         "assistant",
		Model:        modelName,
		StopReason:   stopReason,
		StopSequence: "",
		Usage: Usage{
			InputTokens:          promptTokens,
			OutputTokens:         completionTokens,
			CacheReadInputTokens: cachedTokens,
		},
		Content: contents,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(vertexResp.StatusCode)
	json.NewEncoder(w).Encode(anthropicResp)
}

// writeSSE 写入一条 Anthropic SSE 事件到 HTTP 响应流
// 格式: event: <type>\ndata: <json>\n\n
func writeSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b)
	if flusher != nil {
		flusher.Flush()
	}
}

func ptrInt(i int) *int {
	return &i
}

// normalizeFunctionCallArgs 统一将 Gemini functionCall.args 转换为规范的 JSON 字节。
// Gemini 可能将 args 返回为 map[string]interface{}、JSON 字符串或其他类型，
// 此函数负责将所有可能的形式归一化为紧凑的 JSON 字节数组。
func normalizeFunctionCallArgs(args interface{}) []byte {
	if args == nil {
		return []byte("{}")
	}

	switch v := args.(type) {
	case map[string]interface{}:
		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		_ = encoder.Encode(v)
		result := buffer.Bytes()
		if len(result) > 0 && result[len(result)-1] == '\n' {
			result = result[:len(result)-1]
		}
		if len(result) == 0 || string(result) == "null" {
			return []byte("{}")
		}
		return result
	case string:
		if v == "" || v == "null" {
			return []byte("{}")
		}
		// args 可能是 JSON 字符串，尝试解析后再序列化以规范化格式
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			buffer := &bytes.Buffer{}
			encoder := json.NewEncoder(buffer)
			encoder.SetEscapeHTML(false)
			_ = encoder.Encode(parsed)
			result := buffer.Bytes()
			if len(result) > 0 && result[len(result)-1] == '\n' {
				result = result[:len(result)-1]
			}
			return result
		}
		// 不是合法 JSON，直接当纯文本返回
		return []byte(v)
	default:
		raw, _ := json.Marshal(v)
		if len(raw) == 0 || string(raw) == "null" {
			return []byte("{}")
		}
		return raw
	}
}