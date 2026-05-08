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
			ID:    fmt.Sprintf("msg_%s", traceID),
			Type:  "message",
			Role:  "assistant",
			Model: modelName,
			Usage: Usage{},
		},
	}
	writeSSE(w, flusher, "message_start", startEvent)

	// Send content_block_start
	cbStartEvent := StreamEvent{
		Type: "content_block_start",
		Index: ptrInt(0),
		ContentBlock: &Content{
			Type: "text",
			Text: "",
		},
	}
	writeSSE(w, flusher, "content_block_start", cbStartEvent)

	reader := bufio.NewReader(vertexResp.Body)
	var promptTokens, completionTokens, cachedTokens int

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

		// Extract text delta
		candidates, ok := vResp["candidates"].([]interface{})
		if !ok || len(candidates) == 0 {
			continue
		}

		cand, _ := candidates[0].(map[string]interface{})
		content, ok := cand["content"].(map[string]interface{})
		if !ok {
			continue
		}

		parts, ok := content["parts"].([]interface{})
		if !ok || len(parts) == 0 {
			continue
		}

		part, _ := parts[0].(map[string]interface{})
		text, _ := part["text"].(string)

		if text != "" {
			deltaEvent := StreamEvent{
				Type:  "content_block_delta",
				Index: ptrInt(0),
				Delta: &Delta{
					Type: "text_delta",
					Text: text,
				},
			}
			writeSSE(w, flusher, "content_block_delta", deltaEvent)
		}
	}

	// Send content_block_stop
	cbStopEvent := StreamEvent{
		Type:  "content_block_stop",
		Index: ptrInt(0),
	}
	writeSSE(w, flusher, "content_block_stop", cbStopEvent)

	// Send message_delta (stop reason + usage)
	msgDeltaEvent := StreamEvent{
		Type: "message_delta",
		Delta: &Delta{
			StopReason: "end_turn",
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

	var text string
	if candidates, ok := vResp["candidates"].([]interface{}); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]interface{}); ok {
			if content, ok := cand["content"].(map[string]interface{}); ok {
				if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
					if part, ok := parts[0].(map[string]interface{}); ok {
						if t, ok := part["text"].(string); ok {
							text = t
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
		StopReason:   "end_turn",
		StopSequence: "",
		Usage: Usage{
			InputTokens:  promptTokens,
			OutputTokens: completionTokens,
		},
		Content: []Content{
			{
				Type: "text",
				Text: text,
			},
		},
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