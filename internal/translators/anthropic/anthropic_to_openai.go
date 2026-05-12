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

// Anthropic → OpenAI 协议转换器
// 将 Anthropic Messages API 格式转换为 OpenAI Chat Completions 格式
// 支持流式和非流式，全程计费和 token 统计

// oaiMessage OpenAI Chat Completions 消息格式
type oaiMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
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

// AnthropicToOpenAI 将 Anthropic Messages API 请求转换为 OpenAI Chat Completions 格式
// 转换流程: 解析 Anthropic 消息 → 构建 OpenAI 请求 → 发送到 OpenAI 兼容后端 → 流式回写 Anthropic SSE 格式
// 全程支持 token 计数和费用结算
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
		
		var oaiParts []map[string]interface{}
		var textContent string

		switch v := msg.Content.(type) {
		case string:
			textContent = v
		case []interface{}:
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					switch m["type"] {
					case "text":
						if t, ok := m["text"].(string); ok {
							oaiParts = append(oaiParts, map[string]interface{}{
								"type": "text",
								"text": t,
							})
							textContent += t
						}
					case "image", "document", "audio", "video", "media":
						if source, ok := m["source"].(map[string]interface{}); ok {
							var url string
							if source["type"] == "base64" {
								mediaType, _ := source["media_type"].(string)
								data, _ := source["data"].(string)
								url = fmt.Sprintf("data:%s;base64,%s", mediaType, data)
							} else if source["type"] == "url" {
								url, _ = source["url"].(string)
							}
							
							if url != "" {
								oaiParts = append(oaiParts, map[string]interface{}{
									"type": "image_url",
									"image_url": map[string]interface{}{
										"url": url,
									},
								})
							}
						}
					}
				}
			}
		}

		if len(oaiParts) > 0 {
			if len(oaiParts) == 1 && oaiParts[0]["type"] == "text" {
				oaiReq.Messages = append(oaiReq.Messages, oaiMessage{Role: role, Content: textContent})
			} else {
				oaiReq.Messages = append(oaiReq.Messages, oaiMessage{Role: role, Content: oaiParts})
			}
		} else if textContent != "" {
			oaiReq.Messages = append(oaiReq.Messages, oaiMessage{Role: role, Content: textContent})
		}
	}

	if req.System != nil {
		var sysText string
		switch sys := req.System.(type) {
		case string:
			sysText = sys
		case []interface{}:
			for _, item := range sys {
				if m, ok := item.(map[string]interface{}); ok {
					if m["type"] == "text" {
						if t, ok := m["text"].(string); ok {
							sysText += t
						}
					}
				}
			}
		}
		if sysText != "" {
			// Prepend system message
			oaiReq.Messages = append([]oaiMessage{{Role: "system", Content: sysText}}, oaiReq.Messages...)
		}
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
		utils.HandleNetworkError(w, err, dest, "openai", clientType, "anthropic_adapter", traceID, "Anthropic→OpenAI")
		return
	}

	isNodeFailure, isQuotaExhausted := utils.CheckResponseStatus(finalResp, dest, "openai", clientType, "anthropic_adapter", traceID, "Anthropic→OpenAI")

	if oaiReq.Stream {
		anthropicStreamOpenAI(w, finalResp, traceID, dest, clientType, oaiReq.Model, bodyBytes)
	} else {
		anthropicNonStreamOpenAI(w, finalResp, traceID, dest, clientType, oaiReq.Model, bodyBytes)
	}

	utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
}

// anthropicStreamOpenAI 读取 OpenAI 后端流式 SSE 响应，实时转为 Anthropic SSE 格式写入客户端
func anthropicStreamOpenAI(w http.ResponseWriter, oaiResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string, reqBody []byte) {
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
	var totalWritten int64

	for {
		n, readErr := oaiResp.Body.Read(buf)
		if n > 0 {
			totalWritten += int64(n)
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
	if !found {
		prompt = utils.EstimatePromptTokens(reqBody)
		completion = utils.EstimateCompletionTokens(totalWritten)
		slog.Warn("⚠️ 响应流中断，启用 token 估算补偿", "trace_id", traceID, "node", dest.Node.Name, "prompt", prompt, "completion", completion)
	}

	if prompt > 0 || completion > 0 {
		cost := utils.CalculateCost(dest.Node.Provider, modelName, prompt, completion, cached, reqBody)
		db.SaveUsage("openai", dest.Node.Name, clientType, "anthropic_adapter", prompt, completion, cost, oaiResp.StatusCode)
		dest.Node.RecordCost(cost, traceID)
		if cached > 0 {
			slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", prompt, "cached", cached, "completion", completion, "cost", fmt.Sprintf("%.4f", cost))
		} else {
			slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", prompt, "completion", completion, "cost", fmt.Sprintf("%.4f", cost))
		}
	}
}

// anthropicNonStreamOpenAI 处理 OpenAI 后端的非流式响应，提取内容转为 Anthropic JSON 格式
func anthropicNonStreamOpenAI(w http.ResponseWriter, oaiResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string, reqBody []byte) {
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
	cost := utils.CalculateCost(dest.Node.Provider, modelName, promptTokens, completionTokens, 0, reqBody)
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
