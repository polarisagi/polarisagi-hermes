package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"polaris-gateway/internal/core/router"
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

// parseAndSettleAnthropicResponse 从 Anthropic 格式的非流式响应体中提取 usage 并完成计费
// 两个直通处理器（anthropic→anthropic 和 anthropic→geap-claude）均走此路径，仅 provider 不同
// cache_read_input_tokens：prompt cache 命中；cache_creation_input_tokens：写入 cache（计为 prompt 消耗）
func parseAndSettleAnthropicResponse(provider string, bodyBytes []byte, dest *router.MatchedDestination, clientType, methodName, modelName, traceID string, statusCode int, reqBody []byte) {
	var resp struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(bodyBytes, &resp) == nil {
		// cache_creation 按正常 prompt 计费；cache_read 按折扣价，由 CalculateCost 处理
		inputTokens := int64(resp.Usage.InputTokens) + int64(resp.Usage.CacheCreationInputTokens)
		router.SettleBilling(provider, dest.Node.Name, clientType, methodName, modelName,
			inputTokens, int64(resp.Usage.OutputTokens), int64(resp.Usage.CacheReadInputTokens),
			statusCode, dest, reqBody, traceID)
	}
}
