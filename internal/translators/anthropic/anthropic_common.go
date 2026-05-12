package anthropic

import (
	"fmt"
	"log/slog"
	"net/http"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// writeSSEMessageStart sends the Anthropic SSE message_start event
func writeSSEMessageStart(w http.ResponseWriter, flusher http.Flusher, traceID, modelName string) {
	writeSSE(w, flusher, "message_start", StreamEvent{
		Type: "message_start",
		Message: &MessageResponse{
			ID:      fmt.Sprintf("msg_%s", traceID),
			Type:    "message",
			Role:    "assistant",
			Content: []Content{},
			Model:   modelName,
			Usage:   Usage{},
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
