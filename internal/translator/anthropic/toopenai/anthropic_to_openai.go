package toopenai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"polaris-hermes/internal/domain"
	"polaris-hermes/internal/pkg/httpclient"
	"polaris-hermes/internal/service/channel"
	"polaris-hermes/internal/translator"
)

type AnthropicToOpenAITranslator struct{}

func NewAnthropicToOpenAITranslator() *AnthropicToOpenAITranslator {
	return &AnthropicToOpenAITranslator{}
}

// TranslateAndExecute implements translator.Translator
func (t *AnthropicToOpenAITranslator) TranslateAndExecute(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	bodyBytes []byte,
	ch *channel.ActiveChannel,
	targetEndpoint *domain.SysAccessEndpoint,
	targetModel string,
) error {
	var aReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &aReq); err != nil {
		http.Error(w, "Invalid Anthropic JSON payload", http.StatusBadRequest)
		return nil
	}

	oReq := make(map[string]interface{})
	oReq["model"] = targetModel
	if oReq["model"] == "" || oReq["model"] == nil {
		oReq["model"] = aReq["model"]
	}

	if stream, ok := aReq["stream"].(bool); ok {
		oReq["stream"] = stream
	}
	if temp, ok := aReq["temperature"].(float64); ok {
		oReq["temperature"] = temp
	}
	if topP, ok := aReq["top_p"].(float64); ok {
		oReq["top_p"] = topP
	}
	if maxTokens, ok := aReq["max_tokens"].(float64); ok {
		oReq["max_tokens"] = maxTokens
	}

	var msgs []map[string]interface{}
	// Insert system prompt first if exists
	if sys, ok := aReq["system"].(string); ok && sys != "" {
		msgs = append(msgs, map[string]interface{}{
			"role":    "system",
			"content": sys,
		})
	}

	// Anthropic format uses messages array
	if amsgs, ok := aReq["messages"].([]interface{}); ok {
		for _, m := range amsgs {
			if mm, ok := m.(map[string]interface{}); ok {
				msgs = append(msgs, mm)
			}
		}
	}
	oReq["messages"] = msgs

	oReqBytes, _ := json.Marshal(oReq)

	targetURL := translator.BuildTargetURL(ch, targetEndpoint, "/chat/completions")

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(oReqBytes))
	if err != nil {
		return err
	}

	proxyReq.Header.Set("Content-Type", "application/json")
	var creds map[string]interface{}
	if err := json.Unmarshal(ch.Provider.AuthCredentials, &creds); err == nil {
		if key, ok := creds["api_key"].(string); ok {
			proxyReq.Header.Set("Authorization", "Bearer "+key)
		}
	} else {
		proxyReq.Header.Set("Authorization", "Bearer "+string(ch.Provider.AuthCredentials))
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

	stream, _ := aReq["stream"].(bool)
	if stream {
		t.handleStream(w, resp, targetModel)
	} else {
		t.handleNonStream(w, resp, targetModel)
	}

	return nil
}

func (t *AnthropicToOpenAITranslator) handleNonStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
	body, _ := io.ReadAll(resp.Body)
	var oResp map[string]interface{}
	_ = json.Unmarshal(body, &oResp)

	text := ""
	stopReason := "end_turn"
	if choices, ok := oResp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				if c, ok := msg["content"].(string); ok {
					text = c
				}
			}
			if fr, ok := choice["finish_reason"].(string); ok && fr == "length" {
				stopReason = "max_tokens"
			}
		}
	}

	id := "msg_unknown"
	if rid, ok := oResp["id"].(string); ok {
		id = rid
	}

	var inputTokens, outputTokens int
	if usage, ok := oResp["usage"].(map[string]interface{}); ok {
		if pt, ok := usage["prompt_tokens"].(float64); ok {
			inputTokens = int(pt)
		}
		if ct, ok := usage["completion_tokens"].(float64); ok {
			outputTokens = int(ct)
		}
	}

	aResp := map[string]interface{}{
		"id":   id,
		"type": "message",
		"role": "assistant",
		"content": []map[string]string{
			{
				"type": "text",
				"text": text,
			},
		},
		"model":       targetModel,
		"stop_reason": stopReason,
		"usage": map[string]int{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(aResp)
}

func (t *AnthropicToOpenAITranslator) handleStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)

	var msgID string
	firstChunk := true

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

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
				continue
			}

			if id, ok := chunk["id"].(string); ok {
				msgID = id
			}

			if firstChunk {
				startEvent := map[string]interface{}{
					"type": "message_start",
					"message": map[string]interface{}{
						"id":      msgID,
						"type":    "message",
						"role":    "assistant",
						"model":   targetModel,
						"content": []interface{}{},
						"usage":   map[string]int{"input_tokens": 0, "output_tokens": 0},
					},
				}
				b, _ := json.Marshal(startEvent)
				fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", b)

				blockStart := map[string]interface{}{
					"type":  "content_block_start",
					"index": 0,
					"content_block": map[string]interface{}{
						"type": "text",
						"text": "",
					},
				}
				b, _ = json.Marshal(blockStart)
				fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", b)
				firstChunk = false
			}

			choices, ok := chunk["choices"].([]interface{})
			if !ok || len(choices) == 0 {
				continue
			}
			choice, ok := choices[0].(map[string]interface{})
			if !ok {
				continue
			}

			delta, ok := choice["delta"].(map[string]interface{})
			if ok {
				if content, ok := delta["content"].(string); ok && content != "" {
					deltaEvent := map[string]interface{}{
						"type":  "content_block_delta",
						"index": 0,
						"delta": map[string]interface{}{
							"type": "text_delta",
							"text": content,
						},
					}
					b, _ := json.Marshal(deltaEvent)
					fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", b)
					if flusher != nil {
						flusher.Flush()
					}
				}
			}

			if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
				stopEvent := map[string]interface{}{
					"type":  "content_block_stop",
					"index": 0,
				}
				b, _ := json.Marshal(stopEvent)
				fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", b)

				stopReason := "end_turn"
				if fr == "length" {
					stopReason = "max_tokens"
				}

				msgDelta := map[string]interface{}{
					"type": "message_delta",
					"delta": map[string]interface{}{
						"stop_reason": stopReason,
					},
					"usage": map[string]int{"output_tokens": 0},
				}
				b, _ = json.Marshal(msgDelta)
				fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", b)

				msgStop := map[string]interface{}{
					"type": "message_stop",
				}
				b, _ = json.Marshal(msgStop)
				fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", b)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	}
}
