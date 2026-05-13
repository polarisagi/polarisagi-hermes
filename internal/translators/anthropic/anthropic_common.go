package anthropic

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// httpClient 包级共享 HTTP 客户端，所有 Anthropic 包内的转换器均通过此实例发出上游请求
var httpClient = utils.SharedHTTPClient

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

// parseAndSettleAnthropicResponse 从 Anthropic 格式的非流式响应体中提取 usage 并完成计费
// 两个直通处理器（anthropic→anthropic 和 anthropic→geap-claude）均走此路径，仅 provider 不同
func parseAndSettleAnthropicResponse(provider string, bodyBytes []byte, dest *router.MatchedDestination, clientType, methodName, modelName, traceID string, statusCode int, reqBody []byte) {
	var resp struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(bodyBytes, &resp) == nil {
		settleBilling(provider, dest.Node.Name, clientType, methodName, modelName,
			int64(resp.Usage.InputTokens), int64(resp.Usage.OutputTokens), 0,
			statusCode, dest, reqBody, traceID)
	}
}

// settleBilling records usage and cost for a completed API request
func settleBilling(provider, nodeName, clientType, adapterName, modelName string, promptTokens, completionTokens, cachedTokens int64, statusCode int, dest *router.MatchedDestination, reqBody []byte, traceID string) {
	if promptTokens <= 0 && completionTokens <= 0 {
		return
	}
	cost := utils.CalculateCost(provider, modelName, promptTokens, completionTokens, cachedTokens, reqBody)
	db.SaveUsage(provider, nodeName, clientType, adapterName, promptTokens, completionTokens, cost, statusCode)
	dest.Node.RecordCost(cost, traceID)
	if cachedTokens > 0 {
		slog.Info("💰 结算完成", "trace_id", traceID, "account", nodeName, "model", modelName, "prompt", promptTokens, "cached", cachedTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))
	} else {
		slog.Info("💰 结算完成", "trace_id", traceID, "account", nodeName, "model", modelName, "prompt", promptTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))
	}
}
