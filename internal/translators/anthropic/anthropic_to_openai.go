package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// AnthropicToOpenAI converts Anthropic Messages API requests to OpenAI Chat Completions format
// and forwards to any OpenAI-compatible backend (OpenAI, Gemini API Key, etc.)
// WITH full token counting and cost tracking.

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaiRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
	TopP        *float64     `json:"top_p,omitempty"`
	Stream      bool         `json:"stream,omitempty"`
}

var oaiHTTPClient = &http.Client{Timeout: 180 * time.Second}

func AnthropicToOpenAI(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := "Anthropic-Adapter"

	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type": "error", "error": {"type": "invalid_request_error", "message": "invalid json"}}`, 400)
		return
	}

	// Build OpenAI-format request from Anthropic messages
	oaiReq := oaiRequest{
		Model:     "gemini-1.5-pro", // default fallback
		Stream:    req.Stream,
		MaxTokens: req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	if dest.TargetModel != "" {
		oaiReq.Model = dest.TargetModel
	} else if req.Model != "" && !strings.Contains(req.Model, "claude") {
		oaiReq.Model = req.Model
	}

	// Convert Anthropic messages to OpenAI messages
	for _, msg := range req.Messages {
		role := msg.Role
		if role == "assistant" {
			role = "assistant"
		}
		content := ""
		switch v := msg.Content.(type) {
		case string:
			content = v
		case []interface{}:
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					if m["type"] == "text" {
						if t, ok := m["text"].(string); ok {
							content += t
						}
					}
				}
			}
		}
		if content != "" {
			oaiReq.Messages = append(oaiReq.Messages, oaiMessage{Role: role, Content: content})
		}
	}

	if req.System != "" {
		// Prepend system message
		oaiReq.Messages = append([]oaiMessage{{Role: "system", Content: req.System}}, oaiReq.Messages...)
	}

	oaiBody, _ := json.Marshal(oaiReq)

	targetURL := strings.TrimSuffix(dest.Node.BaseURL, "/")
	if targetURL == "" {
		targetURL = "https://api.openai.com/v1"
	}
	targetURL = targetURL + "/chat/completions"

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路 (Anthropic→OpenAI)", "trace_id", traceID, "account", dest.Node.Name)
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(oaiBody))
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+dest.Node.Credentials)

	finalResp, err := oaiHTTPClient.Do(proxyReq)
	if err != nil {
		errMsg := err.Error()
		db.SaveUsage("openai", dest.Node.Name, clientType, "anthropic_adapter", 0, 0, 0, http.StatusBadGateway)
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
		slog.Error("Anthropic→OpenAI 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		db.SaveUsage("openai", dest.Node.Name, clientType, "anthropic_adapter", 0, 0, 0, statusCode)
		slog.Warn("Anthropic→OpenAI 节点异常/限流，记入熔断惩罚队列", "trace_id", traceID, "status", statusCode)
	} else if statusCode >= 400 {
		db.SaveUsage("openai", dest.Node.Name, clientType, "anthropic_adapter", 0, 0, 0, statusCode)
		slog.Warn("Anthropic→OpenAI 客户端业务请求参数错误", "trace_id", traceID, "status", statusCode)
	}

	// Use anthropic stream handler for response (Anthropic format output to client)
	if oaiReq.Stream {
		anthropicStreamOpenAI(w, finalResp, traceID, dest, clientType, oaiReq.Model)
	} else {
		anthropicNonStreamOpenAI(w, finalResp, traceID, dest, clientType, oaiReq.Model)
	}

	if isNodeFailure {
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
	} else {
		dest.Node.UpdateOnSuccess()
	}
}

func anthropicStreamOpenAI(w http.ResponseWriter, oaiResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string) {
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
		Type:  "content_block_start",
		Index: ptrInt(0),
		ContentBlock: &Content{
			Type: "text",
			Text: "",
		},
	}
	writeSSE(w, flusher, "content_block_start", cbStartEvent)

	buf := make([]byte, 32*1024)
	var tailBuf []byte
	const tailWindowSize = 8192
	var fullText string

	for {
		n, readErr := oaiResp.Body.Read(buf)
		if n > 0 {
			// Parse SSE chunks from OpenAI
			chunk := buf[:n]
			tailBuf = append(tailBuf, chunk...)
			if len(tailBuf) > tailWindowSize {
				tailBuf = tailBuf[len(tailBuf)-tailWindowSize:]
			}

			lines := bytes.Split(chunk, []byte("\n"))
			for _, line := range lines {
				line = bytes.TrimSpace(line)
				if !bytes.HasPrefix(line, []byte("data: ")) {
					continue
				}
				data := bytes.TrimPrefix(line, []byte("data: "))
				if string(data) == "[DONE]" {
					continue
				}

				var chunkJSON map[string]interface{}
				if err := json.Unmarshal(data, &chunkJSON); err != nil {
					continue
				}

				choices, ok := chunkJSON["choices"].([]interface{})
				if !ok || len(choices) == 0 {
					continue
				}
				choice, _ := choices[0].(map[string]interface{})

				delta, ok := choice["delta"].(map[string]interface{})
				if !ok {
					continue
				}
				content, _ := delta["content"].(string)
				if content != "" {
					fullText += content
					deltaEvent := StreamEvent{
						Type:  "content_block_delta",
						Index: ptrInt(0),
						Delta: &Delta{
							Type: "text_delta",
							Text: content,
						},
					}
					writeSSE(w, flusher, "content_block_delta", deltaEvent)
				}
			}
		}
		if readErr != nil {
			break
		}
	}

	// Send content_block_stop
	cbStopEvent := StreamEvent{
		Type:  "content_block_stop",
		Index: ptrInt(0),
	}
	writeSSE(w, flusher, "content_block_stop", cbStopEvent)

	// Send message_delta + message_stop
	msgDeltaEvent := StreamEvent{
		Type: "message_delta",
		Delta: &Delta{
			StopReason: "end_turn",
		},
	}
	writeSSE(w, flusher, "message_delta", msgDeltaEvent)

	msgStopEvent := StreamEvent{
		Type: "message_stop",
	}
	writeSSE(w, flusher, "message_stop", msgStopEvent)

	// Parse usage from tail buffer (OpenAI format)
	prompt, completion, cached, found := utils.ParseUsageFromStreamTail(tailBuf)
	if found {
		cost := utils.CalculateCost(modelName, prompt, completion, cached)
		db.SaveUsage("openai", dest.Node.Name, clientType, "anthropic_adapter", prompt, completion, cost, oaiResp.StatusCode)
		dest.Node.RecordCost(cost, traceID)
		if cached > 0 {
			slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", prompt, "cached", cached, "completion", completion, "cost", fmt.Sprintf("%.4f", cost))
		} else {
			slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", prompt, "completion", completion, "cost", fmt.Sprintf("%.4f", cost))
		}
	}
}

func anthropicNonStreamOpenAI(w http.ResponseWriter, oaiResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string) {
	defer oaiResp.Body.Close()

	var oaiResponse struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(oaiResp.Body).Decode(&oaiResponse); err != nil {
		http.Error(w, "Failed to parse response", http.StatusBadGateway)
		return
	}

	var text string
	if len(oaiResponse.Choices) > 0 {
		text = oaiResponse.Choices[0].Message.Content
	}

	// Record billing
	promptTokens := int64(oaiResponse.Usage.PromptTokens)
	completionTokens := int64(oaiResponse.Usage.CompletionTokens)
	cost := utils.CalculateCost(modelName, promptTokens, completionTokens, 0)
	db.SaveUsage("openai", dest.Node.Name, clientType, "anthropic_adapter", promptTokens, completionTokens, cost, oaiResp.StatusCode)
	dest.Node.RecordCost(cost, traceID)
	slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", promptTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))

	// Return in Anthropic format
	anthropicResp := MessageResponse{
		ID:           fmt.Sprintf("msg_%s", traceID),
		Type:         "message",
		Role:         "assistant",
		Model:        modelName,
		StopReason:   "end_turn",
		StopSequence: "",
		Usage: Usage{
			InputTokens:  oaiResponse.Usage.PromptTokens,
			OutputTokens: oaiResponse.Usage.CompletionTokens,
		},
		Content: []Content{
			{
				Type: "text",
				Text: text,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(oaiResp.StatusCode)
	json.NewEncoder(w).Encode(anthropicResp)
}

func init() {
	router.RegisterTranslator("anthropic", "openai", AnthropicToOpenAI)
	router.RegisterTranslator("anthropic", "gemini", AnthropicToOpenAI)
}
