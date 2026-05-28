package toanthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/polarisagi/polarisagi-hermes/internal/domain"
	"github.com/polarisagi/polarisagi-hermes/internal/pkg/httpclient"
	"github.com/polarisagi/polarisagi-hermes/internal/service/channel"
	"github.com/polarisagi/polarisagi-hermes/internal/translator"
)

type GoogleToAnthropicTranslator struct{}

func NewGoogleToAnthropicTranslator() *GoogleToAnthropicTranslator {
	return &GoogleToAnthropicTranslator{}
}

func (t *GoogleToAnthropicTranslator) TranslateAndExecute(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	bodyBytes []byte,
	ch *channel.ActiveChannel,
	targetEndpoint *domain.SysAccessEndpoint,
	targetModel string,
) error {
	var gReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &gReq); err != nil {
		http.Error(w, "Invalid Google JSON payload", http.StatusBadRequest)
		return nil
	}

	aReq := make(map[string]interface{})
	aReq["model"] = targetModel

	isStream := strings.Contains(r.URL.RawQuery, "alt=sse") || strings.Contains(r.URL.Path, "stream")
	aReq["stream"] = isStream
	aReq["max_tokens"] = 4096

	var msgs []map[string]interface{}
	if contents, ok := gReq["contents"].([]interface{}); ok {
		for _, c := range contents {
			if content, ok := c.(map[string]interface{}); ok {
				role := "user"
				if r, ok := content["role"].(string); ok {
					if r == "model" {
						role = "assistant"
					}
				}

				text := ""
				if parts, ok := content["parts"].([]interface{}); ok {
					for _, p := range parts {
						if part, ok := p.(map[string]interface{}); ok {
							if t, ok := part["text"].(string); ok {
								text += t
							}
						}
					}
				}
				msgs = append(msgs, map[string]interface{}{
					"role":    role,
					"content": text,
				})
			}
		}
	}
	aReq["messages"] = msgs

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

	if isStream {
		t.handleStream(w, resp, targetModel)
	} else {
		t.handleNonStream(w, resp, targetModel)
	}

	return nil
}

func (t *GoogleToAnthropicTranslator) handleNonStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
	body, _ := io.ReadAll(resp.Body)
	var aResp map[string]interface{}
	_ = json.Unmarshal(body, &aResp)

	text := ""
	if contents, ok := aResp["content"].([]interface{}); ok && len(contents) > 0 {
		if c, ok := contents[0].(map[string]interface{}); ok {
			if t, ok := c["text"].(string); ok {
				text = t
			}
		}
	}

	gResp := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role": "model",
					"parts": []map[string]interface{}{
						{"text": text},
					},
				},
				"finishReason": "STOP",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(gResp)
}

func (t *GoogleToAnthropicTranslator) handleStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)

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

			if eventType, ok := event["type"].(string); ok && eventType == "content_block_delta" {
				if delta, ok := event["delta"].(map[string]interface{}); ok {
					if text, ok := delta["text"].(string); ok && text != "" {
						gChunk := map[string]interface{}{
							"candidates": []map[string]interface{}{
								{
									"content": map[string]interface{}{
										"role": "model",
										"parts": []map[string]interface{}{
											{"text": text},
										},
									},
								},
							},
						}
						b, _ := json.Marshal(gChunk)
						fmt.Fprintf(w, "data: %s\n\n", b)
						if flusher != nil {
							flusher.Flush()
						}
					}
				}
			}
		}
	}
}
