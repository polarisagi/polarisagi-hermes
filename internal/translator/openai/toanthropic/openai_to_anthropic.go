package toanthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/polarisagi/polarisagi-hermes/internal/domain"
	"github.com/polarisagi/polarisagi-hermes/pkg/httpclient"
	"github.com/polarisagi/polarisagi-hermes/internal/service/channel"
	"github.com/polarisagi/polarisagi-hermes/internal/translator"
)

type OpenAIToAnthropicTranslator struct{}

func NewOpenAIToAnthropicTranslator() *OpenAIToAnthropicTranslator {
	return &OpenAIToAnthropicTranslator{}
}

type openaiReq struct {
	Model       string                   `json:"model"`
	Messages    []map[string]interface{} `json:"messages"`
	Stream      bool                     `json:"stream,omitempty"`
	MaxTokens   *int                     `json:"max_tokens,omitempty"`
	Temperature *float64                 `json:"temperature,omitempty"`
	TopP        *float64                 `json:"top_p,omitempty"`
}

type anthropicReq struct {
	Model       string                   `json:"model"`
	System      string                   `json:"system,omitempty"`
	Messages    []map[string]interface{} `json:"messages"`
	Stream      bool                     `json:"stream,omitempty"`
	MaxTokens   int                      `json:"max_tokens"`
	Temperature *float64                 `json:"temperature,omitempty"`
	TopP        *float64                 `json:"top_p,omitempty"`
}

func (t *OpenAIToAnthropicTranslator) TranslateAndExecute(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	bodyBytes []byte,
	ch *channel.ActiveChannel,
	targetEndpoint *domain.SysAccessEndpoint,
	targetModel string,
) error {
	var oReq openaiReq
	if err := json.Unmarshal(bodyBytes, &oReq); err != nil {
		http.Error(w, "Invalid OpenAI JSON payload", http.StatusBadRequest)
		return nil
	}

	aReq := anthropicReq{
		Model:       targetModel,
		Stream:      oReq.Stream,
		Temperature: oReq.Temperature,
		TopP:        oReq.TopP,
	}
	if oReq.MaxTokens != nil {
		aReq.MaxTokens = *oReq.MaxTokens
	} else {
		aReq.MaxTokens = 4096
	}

	var sysPrompts []string
	var msgs []map[string]interface{}

	for _, m := range oReq.Messages {
		role, _ := m["role"].(string)
		if role == "system" {
			if content, ok := m["content"].(string); ok {
				sysPrompts = append(sysPrompts, content)
			}
		} else {
			msgs = append(msgs, m)
		}
	}
	aReq.System = strings.Join(sysPrompts, "\n")
	aReq.Messages = msgs

	aReqBytes, _ := json.Marshal(aReq)

	targetURL := translator.BuildTargetURL(ch, targetEndpoint, "/messages")

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(aReqBytes))
	if err != nil {
		return err
	}

	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("anthropic-version", "2023-06-01")

	var creds map[string]interface{}
	if err := json.Unmarshal(ch.Provider.AuthCredentials, &creds); err == nil {
		if key, ok := creds["api_key"].(string); ok {
			proxyReq.Header.Set("x-api-key", key)
		}
	} else {
		proxyReq.Header.Set("x-api-key", string(ch.Provider.AuthCredentials))
	}

	resp, err := httpclient.Client.Do(proxyReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(errBody)
		return nil
	}

	if oReq.Stream {
		t.handleStream(w, resp, targetModel)
	} else {
		t.handleNonStream(w, resp, targetModel)
	}

	return nil
}

type anthropicResp struct {
	ID      string `json:"id"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (t *OpenAIToAnthropicTranslator) handleNonStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
	body, _ := io.ReadAll(resp.Body)
	var aResp anthropicResp
	_ = json.Unmarshal(body, &aResp)

	text := ""
	if len(aResp.Content) > 0 {
		text = aResp.Content[0].Text
	}
	finishReason := "stop"
	if aResp.StopReason == "max_tokens" {
		finishReason = "length"
	}

	oResp := map[string]interface{}{
		"id":      aResp.ID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   targetModel,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": text,
				},
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     aResp.Usage.InputTokens,
			"completion_tokens": aResp.Usage.OutputTokens,
			"total_tokens":      aResp.Usage.InputTokens + aResp.Usage.OutputTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(oResp)
}

func (t *OpenAIToAnthropicTranslator) handleStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)

	var msgID string
	created := time.Now().Unix()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			if dataStr == "[DONE]" || dataStr == "" {
				continue
			}

			var event map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
				continue
			}

			eventType, _ := event["type"].(string)
			switch eventType {
			case "message_start":
				if msg, ok := event["message"].(map[string]interface{}); ok {
					if id, ok := msg["id"].(string); ok {
						msgID = id
					}
					chunk := map[string]interface{}{
						"id":      msgID,
						"object":  "chat.completion.chunk",
						"created": created,
						"model":   targetModel,
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"delta": map[string]string{
									"role": "assistant",
								},
								"finish_reason": nil,
							},
						},
					}
					b, _ := json.Marshal(chunk)
					fmt.Fprintf(w, "data: %s\n\n", b)
					if flusher != nil {
						flusher.Flush()
					}
				}
			case "content_block_delta":
				if delta, ok := event["delta"].(map[string]interface{}); ok {
					if text, ok := delta["text"].(string); ok {
						chunk := map[string]interface{}{
							"id":      msgID,
							"object":  "chat.completion.chunk",
							"created": created,
							"model":   targetModel,
							"choices": []map[string]interface{}{
								{
									"index": 0,
									"delta": map[string]string{
										"content": text,
									},
									"finish_reason": nil,
								},
							},
						}
						b, _ := json.Marshal(chunk)
						fmt.Fprintf(w, "data: %s\n\n", b)
						if flusher != nil {
							flusher.Flush()
						}
					}
				}
			case "message_delta":
				if delta, ok := event["delta"].(map[string]interface{}); ok {
					if stopReason, ok := delta["stop_reason"].(string); ok && stopReason != "" {
						reason := "stop"
						if stopReason == "max_tokens" {
							reason = "length"
						}
						chunk := map[string]interface{}{
							"id":      msgID,
							"object":  "chat.completion.chunk",
							"created": created,
							"model":   targetModel,
							"choices": []map[string]interface{}{
								{
									"index":         0,
									"delta":         map[string]interface{}{},
									"finish_reason": reason,
								},
							},
						}
						b, _ := json.Marshal(chunk)
						fmt.Fprintf(w, "data: %s\n\n", b)
						if flusher != nil {
							flusher.Flush()
						}
					}
				}
			case "error":
				if errObj, ok := event["error"].(map[string]interface{}); ok {
					slog.Error("Anthropic stream error", "error", errObj)
				}
			}
		}
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}
