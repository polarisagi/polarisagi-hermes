package togoogle

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/polarisagi/polarisagi-hermes/internal/domain"
	"github.com/polarisagi/polarisagi-hermes/pkg/httpclient"
	"github.com/polarisagi/polarisagi-hermes/internal/service/channel"
	"github.com/polarisagi/polarisagi-hermes/internal/translator"
)

type OpenAIToGoogleTranslator struct{}

func NewOpenAIToGoogleTranslator() *OpenAIToGoogleTranslator {
	return &OpenAIToGoogleTranslator{}
}

func (t *OpenAIToGoogleTranslator) TranslateAndExecute(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	bodyBytes []byte,
	ch *channel.ActiveChannel,
	targetEndpoint *domain.SysAccessEndpoint,
	targetModel string,
) error {
	var oReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &oReq); err != nil {
		http.Error(w, "Invalid OpenAI JSON payload", http.StatusBadRequest)
		return nil
	}

	gReq := make(map[string]interface{})
	var contents []map[string]interface{}
	var sysInstruction string

	if msgs, ok := oReq["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if msg, ok := m.(map[string]interface{}); ok {
				role, _ := msg["role"].(string)
				content, _ := msg["content"].(string)

				if role == "system" {
					sysInstruction += content + "\n"
					continue
				}

				gRole := "user"
				if role == "assistant" {
					gRole = "model"
				}

				contents = append(contents, map[string]interface{}{
					"role": gRole,
					"parts": []map[string]interface{}{
						{"text": content},
					},
				})
			}
		}
	}

	gReq["contents"] = contents
	if sysInstruction != "" {
		gReq["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": strings.TrimSpace(sysInstruction)},
			},
		}
	}

	if temp, ok := oReq["temperature"]; ok {
		gReq["generationConfig"] = map[string]interface{}{
			"temperature": temp,
		}
	}

	gReqBytes, _ := json.Marshal(gReq)

	isStream := false
	if s, ok := oReq["stream"].(bool); ok && s {
		isStream = true
	}

	if targetModel == "" {
		if m, ok := oReq["model"].(string); ok {
			targetModel = m
		} else {
			targetModel = "gemini-1.5-pro"
		}
	}

	subpath := fmt.Sprintf("/models/%s:generateContent", targetModel)
	if isStream {
		subpath = fmt.Sprintf("/models/%s:streamGenerateContent", targetModel)
	}

	targetURL := translator.BuildTargetURL(ch, targetEndpoint, subpath)

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(gReqBytes))
	if err != nil {
		return err
	}

	proxyReq.Header.Set("Content-Type", "application/json")

	q := proxyReq.URL.Query()
	var creds map[string]interface{}
	if err := json.Unmarshal(ch.Provider.AuthCredentials, &creds); err == nil {
		if key, ok := creds["api_key"].(string); ok {
			q.Set("key", key)
		}
	} else {
		q.Set("key", string(ch.Provider.AuthCredentials))
	}
	if isStream {
		q.Set("alt", "sse")
	}
	proxyReq.URL.RawQuery = q.Encode()

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

func (t *OpenAIToGoogleTranslator) handleNonStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
	body, _ := io.ReadAll(resp.Body)
	var gResp map[string]interface{}
	_ = json.Unmarshal(body, &gResp)

	text := ""
	if candidates, ok := gResp["candidates"].([]interface{}); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]interface{}); ok {
			if content, ok := cand["content"].(map[string]interface{}); ok {
				if parts, ok := content["parts"].([]interface{}); ok {
					for _, p := range parts {
						if part, ok := p.(map[string]interface{}); ok {
							if t, ok := part["text"].(string); ok {
								text += t
							}
						}
					}
				}
			}
		}
	}

	oResp := map[string]interface{}{
		"id":      "chatcmpl-gemini",
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
				"finish_reason": "stop",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(oResp)
}

func (t *OpenAIToGoogleTranslator) handleStream(w http.ResponseWriter, resp *http.Response, targetModel string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)
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

			var gResp map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &gResp); err != nil {
				continue
			}

			if candidates, ok := gResp["candidates"].([]interface{}); ok && len(candidates) > 0 {
				if cand, ok := candidates[0].(map[string]interface{}); ok {
					if content, ok := cand["content"].(map[string]interface{}); ok {
						if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
							if part, ok := parts[0].(map[string]interface{}); ok {
								if text, ok := part["text"].(string); ok && text != "" {
									oChunk := map[string]interface{}{
										"id":      "chatcmpl-gemini-stream",
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
									b, _ := json.Marshal(oChunk)
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
	}

	endChunk := map[string]interface{}{
		"id":      "chatcmpl-gemini-stream",
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   targetModel,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": "stop",
			},
		},
	}
	b, _ := json.Marshal(endChunk)
	fmt.Fprintf(w, "data: %s\n\n", b)

	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}
