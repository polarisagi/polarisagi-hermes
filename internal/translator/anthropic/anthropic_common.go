package anthropic

import (
	"fmt"
	"net/http"
	"strings"
)


// writeSSEMessageStart 发送 message_start 事件
// estimatedInputTokens > 0 时填入 Usage.InputTokens，让 Claude Code 的 /context 命令
// 在第一个事件就能显示上下文占比；后续 message_delta 会以精确值覆盖
func writeSSEMessageStart(w http.ResponseWriter, flusher http.Flusher, traceID, modelName string, estimatedInputTokens int) {
	writeSSE(w, flusher, "message_start", StreamEvent{
		Type: "message_start",
		Message: &MessageResponse{
			ID:      fmt.Sprintf("msg_%s", traceID),
			Type:    "message",
			Role:    "assistant",
			Content: []Content{},
			Model:   modelName,
			Usage:   Usage{InputTokens: estimatedInputTokens},
		},
	})
}

// writeSSEContentBlockStop sends the Anthropic SSE content_block_stop event
func writeSSEContentBlockStop(w http.ResponseWriter, flusher http.Flusher, index int) {
	writeSSE(w, flusher, "content_block_stop", StreamEvent{
		Type:  "content_block_stop",
		Index: ptrInt(index),
	})
}

// writeSSEMessageStop sends the Anthropic SSE message_stop event
func writeSSEMessageStop(w http.ResponseWriter, flusher http.Flusher) {
	writeSSE(w, flusher, "message_stop", StreamEvent{
		Type: "message_stop",
	})
}

// ExtractAndStripBillingHeader 检查并移除 System prompt 中的 x-anthropic-billing-header。
// 返回提取到的 header 字符串，以便在返回给客户端时将其带上。
// 支持 string 和 []interface{} 两种解析格式。
func ExtractAndStripBillingHeader(req *MessageRequest) string {
	if req.System == nil {
		return ""
	}

	var extracted string
	prefix := "x-anthropic-billing-header:"

	switch sys := req.System.(type) {
	case string:
		if strings.HasPrefix(strings.TrimSpace(sys), prefix) {
			// Find the first newline or end of string
			parts := strings.SplitN(sys, "\n", 2)
			extracted = strings.TrimSpace(parts[0])
			if len(parts) > 1 {
				req.System = strings.TrimSpace(parts[1])
			} else {
				req.System = nil
			}
		}
	case []interface{}:
		var newSys []interface{}
		for _, item := range sys {
			if m, ok := item.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if text, ok := m["text"].(string); ok {
						if strings.HasPrefix(strings.TrimSpace(text), prefix) {
							extracted = strings.TrimSpace(text)
							continue // skip this block
						}
					}
				}
			}
			newSys = append(newSys, item)
		}
		if len(newSys) == 0 {
			req.System = nil
		} else {
			req.System = newSys
		}
	}
	return extracted
}


