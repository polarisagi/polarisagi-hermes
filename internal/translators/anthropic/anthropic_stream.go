// Anthropic 响应流式/非流式处理 + SSE 写入工具
// 从 Vertex 后端读取 GenerateContentResponse，实时转换为 Anthropic SSE 格式并推送给客户端
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
	var toolID string
	stopReason := "end_turn"
	var streamError string // tracks mid-stream error for reporting

	for {
		// 客户端断开时 r.Context() 被取消，提前退出避免继续消耗上游连接
		if ctx.Err() != nil {
			slog.Debug("🔌 [Stream] 客户端已断开，终止 Vertex 流式响应", "trace_id", traceID, "account", dest.Node.Name)
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
			slog.Error("❌ [Stream] Vertex SSE 流读取失败，连接可能中断", "trace_id", traceID, "account", dest.Node.Name, "error", err.Error())
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
			slog.Warn("⚠️ [Stream] Vertex SSE 行 JSON 解析失败", "trace_id", traceID, "account", dest.Node.Name, "data_preview", string(data[:min(len(data), 200)]))
			continue
		}

		// Detect Vertex API error inside SSE stream (e.g., mid-stream rate limit)
		if errData, ok := vResp["error"].(map[string]interface{}); ok {
			errMsg := "vertex API error in stream"
			if msg, ok := errData["message"].(string); ok {
				errMsg = msg
			}
			streamError = errMsg
			slog.Error("❌ [Stream] Vertex 在流中返回 API 错误（可能是中途触发限流）", "trace_id", traceID, "account", dest.Node.Name, "error", errMsg)
			break
		}

		// Detect promptFeedback block (content policy refusal)
		if pf, ok := vResp["promptFeedback"].(map[string]interface{}); ok {
			if blockReason, ok := pf["blockReason"].(string); ok && blockReason != "" {
				streamError = fmt.Sprintf("request blocked by Vertex safety filter: %s", blockReason)
				slog.Error("❌ [Stream] Vertex promptFeedback 阻断请求", "trace_id", traceID, "account", dest.Node.Name, "block_reason", blockReason)
				break
			}
		}

		// Parse Usage
		if usage, ok := vResp["usageMetadata"].(map[string]interface{}); ok {
			if p, ok := usage["promptTokenCount"].(float64); ok {
				promptTokens = int(p)
			}
			if c, ok := usage["candidatesTokenCount"].(float64); ok {
				completionTokens = int(c)
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
				// 若下方循环检测到 functionCall，会覆盖为 "tool_use"
				// 此处保持 stopReason 不变（默认 "end_turn"）
			case "MALFORMED_FUNCTION_CALL", "UNEXPECTED_TOOL_CALL":
				// 工具调用相关的非致命错误：映射为 tool_use 让客户端重试
				stopReason = "tool_use"
			case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY":
				stopReason = "end_turn"
				slog.Warn("⚠️ [Stream] Vertex 返回安全相关停止原因", "trace_id", traceID, "account", dest.Node.Name, "finish_reason", finishReason)
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

			// Gemini 在启用 thinkingConfig 时会以 thought=true 的 part 返回内部推理过程
			// 不应作为正式答案输出给客户端（否则思考内容会前置到最终答案前）
			if isThought, _ := part["thought"].(bool); isThought {
				continue
			}

			if text, ok := part["text"].(string); ok && text != "" {
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

				writeSSE(w, flusher, "content_block_delta", StreamEvent{
					Type:  "content_block_delta",
					Index: ptrInt(blockIndex),
					Delta: &Delta{
						Type: "text_delta",
						Text: text,
					},
				})
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
				
				var argsBytes []byte
				if args, ok := fc["args"].(map[string]interface{}); ok {
					buffer := &bytes.Buffer{}
					encoder := json.NewEncoder(buffer)
					encoder.SetEscapeHTML(false)
					_ = encoder.Encode(args)
					argsBytes = buffer.Bytes()
					if len(argsBytes) > 0 && argsBytes[len(argsBytes)-1] == '\n' {
						argsBytes = argsBytes[:len(argsBytes)-1]
					}
				}
				if len(argsBytes) == 0 || string(argsBytes) == "null" {
					argsBytes = []byte("{}")
				}
				
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

	// If the stream completed normally but Vertex generated no content, report as error
	// This surfaces the "no assistant message" failure as a proper error rather than silently
	// returning an empty assistant message that confuses Claude Code's /compact.
	if streamError == "" && blockIndex == 0 && !inText {
		streamError = "Vertex returned empty response with no text content (possible safety filter or empty candidates)"
		slog.Warn("⚠️ [Stream] Vertex 流式响应未包含任何文本内容，将返回错误供客户端重试",
			"trace_id", traceID, "account", dest.Node.Name,
			"stop_reason", stopReason, "prompt_tokens", promptTokens)
	}

	// If streamError is set, send Anthropic-compatible error SSE and abort
	if streamError != "" {
		// Close any open block first so Claude Code can cleanly parse partial content
		if inText {
			writeSSEContentBlockStop(w, flusher, blockIndex)
		}
		// Send Anthropic SSE error event
		writeSSE(w, flusher, "error", StreamEvent{
			Type: "error",
			Message: &MessageResponse{
				Type: "error",
				Role: "assistant",
				Content: []Content{},
			},
		})
		fmt.Fprintf(w, "data: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"%s\"}}\n\n", streamError)
		if flusher != nil {
			flusher.Flush()
		}
		return false
	}

	// Close any open blocks
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

	if promptTokens == 0 && completionTokens == 0 && !streamOK {
		promptTokens = int(utils.EstimatePromptTokens(reqBody))
		completionTokens = int(utils.EstimateCompletionTokens(totalWritten))
		slog.Warn("⚠️ 响应流中断，启用 token 估算补偿", "trace_id", traceID, "node", dest.Node.Name, "prompt", promptTokens, "completion", completionTokens)
	}

	settleBilling("vertex", dest.Node.Name, clientType, "anthropic_adapter", modelName, int64(promptTokens), int64(completionTokens), int64(cachedTokens), http.StatusOK, dest, reqBody, traceID)

	return true
}

// handleAnthropicNonStreamResponse 处理 Vertex 非流式响应，提取文本和用量，转为 Anthropic JSON 格式返回
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
		if c, ok := usage["candidatesTokenCount"].(float64); ok {
			completionTokens = int(c)
		}
		if cache, ok := usage["cachedContentTokenCount"].(float64); ok {
			cachedTokens = int(cache)
		}
	}

	// Detect promptFeedback block (safety refusal before any candidates)
	if pf, ok := vResp["promptFeedback"].(map[string]interface{}); ok {
		if blockReason, ok := pf["blockReason"].(string); ok && blockReason != "" {
			slog.Error("❌ [NonStream] Vertex promptFeedback 阻断请求", "trace_id", traceID, "account", dest.Node.Name, "block_reason", blockReason)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": fmt.Sprintf("request blocked by Vertex safety filter: %s", blockReason),
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
				case "MALFORMED_FUNCTION_CALL", "UNEXPECTED_TOOL_CALL":
					stopReason = "tool_use"
				case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY":
					stopReason = "end_turn"
					slog.Warn("⚠️ [NonStream] Vertex 返回安全相关停止原因",
						"trace_id", traceID, "account", dest.Node.Name, "finish_reason", finishReason)
				}
			}
			if content, ok := cand["content"].(map[string]interface{}); ok {
				if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
					for i, partIntf := range parts {
						part, _ := partIntf.(map[string]interface{})
						// 跳过 Gemini 思考过程 part，避免混入最终答案
						if isThought, _ := part["thought"].(bool); isThought {
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
							args, _ := fc["args"].(map[string]interface{})
							if args == nil {
								args = make(map[string]interface{})
							}
							contents = append(contents, Content{
								Type:  "tool_use",
								ID:    fmt.Sprintf("toolu_%s_%d", traceID, i),
								Name:  name,
								Input: args,
							})
							stopReason = "tool_use"
						}
					}
				}
			}
		}
	}

	if len(contents) == 0 || (len(contents) == 1 && contents[0].Text == "") {
		slog.Warn("⚠️ [NonStream] Vertex 返回响应但无有效文本内容（可能被安全过滤器屏蔽或返回空候选项），填充默认占位符防止客户端崩溃",
			"trace_id", traceID, "account", dest.Node.Name,
			"stop_reason", stopReason, "vertex_resp_preview", string(bodyBytes[:min(len(bodyBytes), 500)]))

		contents = []Content{
			{Type: "text", Text: "[Summary skipped: Vertex API returned an empty response]"},
		}
	}

	settleBilling("vertex", dest.Node.Name, clientType, "anthropic_adapter", modelName, int64(promptTokens), int64(completionTokens), int64(cachedTokens), vertexResp.StatusCode, dest, reqBody, traceID)

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