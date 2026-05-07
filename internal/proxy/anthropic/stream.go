package anthropic

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"polaris-gateway/internal/db"
)

func streamAnthropicResponse(w http.ResponseWriter, vertexResp *http.Response, req MessageRequest, traceID string, state *AccountState, clientType, modelName string) {
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
	var promptTokens, completionTokens int

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
		// Use "vertex" platform for db tracking since we are hitting Vertex
		db.SaveUsage("vertex", state.Name, clientType, "anthropic_adapter", int64(promptTokens), int64(completionTokens), 0, http.StatusOK)

		// Update account total consumed (rough estimation or just tokens for now)
		// We'd ideally calculate cost here. For simplicity, just recording tokens via db.
	}
}

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
