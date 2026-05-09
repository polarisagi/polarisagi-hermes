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

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// streamAnthropicResponse 从 Vertex 后端读取流式 SSE 响应，边读边转为 Anthropic SSE 格式
// 事件序列: message_start → content_block_start → content_block_delta* → content_block_stop → message_delta → message_stop
// 同时在最后解析 usageMetadata 完成计费结算
func streamAnthropicResponse(w http.ResponseWriter, vertexResp *http.Response, req MessageRequest, traceID string, dest *router.MatchedDestination, clientType, modelName string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	// Send message_start
	startEvent := StreamEvent{
		Type: "message_start",
		Message: &MessageResponse{
			ID:      fmt.Sprintf("msg_%s", traceID),
			Type:    "message",
			Role:    "assistant",
			Content: []Content{}, // Prevents "content": null
			Model:   modelName,
			Usage:   Usage{},
		},
	}
	writeSSE(w, flusher, "message_start", startEvent)

	reader := bufio.NewReader(vertexResp.Body)
	var promptTokens, completionTokens, cachedTokens int

	blockIndex := 0
	inText := false
	var toolID string
	stopReason := "end_turn"

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
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
			continue
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
			if finishReason == "MAX_TOKENS" {
				stopReason = "max_tokens"
			} else if finishReason == "STOP" {
				// We'll leave it as end_turn or tool_use based on whether we were in a tool
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
				
				argsStr := string(argsBytes)
				chunkSize := 40
				for i := 0; i < len(argsStr); i += chunkSize {
					end := i + chunkSize
					if end > len(argsStr) {
						end = len(argsStr)
					}
					
					writeSSE(w, flusher, "content_block_delta", StreamEvent{
						Type:  "content_block_delta",
						Index: ptrInt(blockIndex),
						Delta: &Delta{
							Type:        "input_json_delta",
							PartialJson: argsStr[i:end],
						},
					})
				}
				
				writeSSE(w, flusher, "content_block_stop", StreamEvent{
					Type:  "content_block_stop",
					Index: ptrInt(blockIndex),
				})

				blockIndex++
				stopReason = "tool_use"
			}
		}
	}

	// Close any open blocks
	if inText {
		writeSSE(w, flusher, "content_block_stop", StreamEvent{
			Type:  "content_block_stop",
			Index: ptrInt(blockIndex),
		})
	}

	// Send message_delta (stop reason + usage)
	msgDeltaEvent := StreamEvent{
		Type: "message_delta",
		Delta: &Delta{
			StopReason: stopReason,
		},
		Usage: &Usage{
			OutputTokens: completionTokens,
		},
	}
	writeSSE(w, flusher, "message_delta", msgDeltaEvent)

	// Send message_stop
	msgStopEvent := StreamEvent{
		Type: "message_stop",
	}
	writeSSE(w, flusher, "message_stop", msgStopEvent)

	// Settle Usage
	if promptTokens > 0 || completionTokens > 0 {
		cost := utils.CalculateCost(modelName, int64(promptTokens), int64(completionTokens), int64(cachedTokens))
		db.SaveUsage("vertex", dest.Node.Name, clientType, "anthropic_adapter", int64(promptTokens), int64(completionTokens), cost, http.StatusOK)
		dest.Node.RecordCost(cost, traceID)

		if cachedTokens > 0 {
			slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", promptTokens, "cached", cachedTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))
		} else {
			slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", promptTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))
		}
	}
}

// handleAnthropicNonStreamResponse 处理 Vertex 非流式响应，提取文本和用量，转为 Anthropic JSON 格式返回
func handleAnthropicNonStreamResponse(w http.ResponseWriter, vertexResp *http.Response, req MessageRequest, traceID string, dest *router.MatchedDestination, clientType, modelName string) {
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

	var contents []Content
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

	if promptTokens > 0 || completionTokens > 0 {
		cost := utils.CalculateCost(modelName, int64(promptTokens), int64(completionTokens), int64(cachedTokens))
		db.SaveUsage("vertex", dest.Node.Name, clientType, "anthropic_adapter", int64(promptTokens), int64(completionTokens), cost, vertexResp.StatusCode)
		dest.Node.RecordCost(cost, traceID)

		if cachedTokens > 0 {
			slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", promptTokens, "cached", cachedTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))
		} else {
			slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", promptTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))
		}
	}

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