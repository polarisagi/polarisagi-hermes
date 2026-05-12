// Anthropic 响应流式/非流式处理 + SSE 写入工具
// 从 Vertex 后端读取 GenerateContentResponse，实时转换为 Anthropic SSE 格式并推送给客户端
package anthropic

import (
	"bufio"
	"bytes"
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
func streamAnthropicResponse(w http.ResponseWriter, vertexResp *http.Response, req MessageRequest, traceID string, dest *router.MatchedDestination, clientType, modelName string, reqBody []byte) (streamOK bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	writeSSEMessageStart(w, flusher, traceID, modelName)

	reader := bufio.NewReader(vertexResp.Body)
	var promptTokens, completionTokens, cachedTokens int
	var totalWritten int64

	blockIndex := 0
	inText := false
	var toolID string
	stopReason := "end_turn"
	var streamError string // tracks mid-stream error for reporting

	for {
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
				// stopReason will be set to "tool_use" below if a functionCall is detected
			case "MALFORMED_FUNCTION_CALL":
				// Model attempted a tool call but produced malformed args; treat as tool_use
				// so Claude Code knows to retry rather than stopping the conversation
				stopReason = "tool_use"
			case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
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

	// Send message_delta (stop reason + usage)
	msgDeltaEvent := StreamEvent{
		Type: "message_delta",
		Delta: &Delta{
			StopReason: stopReason,
		},
		Usage: &Usage{
			InputTokens:  promptTokens,
			OutputTokens: completionTokens,
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

	contents := []Content{}
	stopReason := "end_turn"

	if candidates, ok := vResp["candidates"].([]interface{}); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]interface{}); ok {
			if finishReason, ok := cand["finishReason"].(string); ok && finishReason != "" {
				if finishReason == "MAX_TOKENS" {
					stopReason = "max_tokens"
				}
			}
			if content, ok := cand["content"].(map[string]interface{}); ok {
				if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
					for i, partIntf := range parts {
						part, _ := partIntf.(map[string]interface{})
						if t, ok := part["text"].(string); ok && t != "" {
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

	settleBilling("vertex", dest.Node.Name, clientType, "anthropic_adapter", modelName, int64(promptTokens), int64(completionTokens), int64(cachedTokens), vertexResp.StatusCode, dest, reqBody, traceID)

	anthropicResp := MessageResponse{
		ID:           fmt.Sprintf("msg_%s", traceID),
		Type:         "message",
		Role:         "assistant",
		Model:        modelName,
		StopReason:   stopReason,
		StopSequence: "",
		Usage: Usage{
			InputTokens:  promptTokens,
			OutputTokens: completionTokens,
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