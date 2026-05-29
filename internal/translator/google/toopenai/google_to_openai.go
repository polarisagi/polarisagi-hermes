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

	"github.com/polarisagi/polarisagi-hermes/internal/domain"
	"github.com/polarisagi/polarisagi-hermes/pkg/httpclient"
	"github.com/polarisagi/polarisagi-hermes/internal/service/channel"
	"github.com/polarisagi/polarisagi-hermes/internal/translator"
)

type GoogleToOpenAITranslator struct{}

func NewGoogleToOpenAITranslator() *GoogleToOpenAITranslator {
	return &GoogleToOpenAITranslator{}
}

func (t *GoogleToOpenAITranslator) TranslateAndExecute(
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

	oReq := make(map[string]interface{})
	oReq["model"] = targetModel

	isStream := strings.Contains(r.URL.RawQuery, "alt=sse") || strings.Contains(r.URL.Path, "stream")
	oReq["stream"] = isStream

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

	if isStream {
		t.handleStream(w, resp, targetModel)
	} else {
		t.handleNonStream(w, resp, targetModel)
	}

	return nil
}

func (t *GoogleToOpenAITranslator) handleNonStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
	body, _ := io.ReadAll(resp.Body)
	var oResp map[string]interface{}
	_ = json.Unmarshal(body, &oResp)

	text := ""
	if choices, ok := oResp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				if c, ok := msg["content"].(string); ok {
					text = c
				}
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

func (t *GoogleToOpenAITranslator) handleStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
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

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
				continue
			}

			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if text, ok := delta["content"].(string); ok && text != "" {
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
}
