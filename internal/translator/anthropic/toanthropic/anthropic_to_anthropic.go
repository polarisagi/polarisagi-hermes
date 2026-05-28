package toanthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/polarisagi/polarisagi-hermes/internal/domain"
	"github.com/polarisagi/polarisagi-hermes/internal/pkg/httpclient"
	"github.com/polarisagi/polarisagi-hermes/internal/service/channel"
	"github.com/polarisagi/polarisagi-hermes/internal/translator"
)

type AnthropicToAnthropicTranslator struct{}

func NewAnthropicToAnthropicTranslator() *AnthropicToAnthropicTranslator {
	return &AnthropicToAnthropicTranslator{}
}

func (t *AnthropicToAnthropicTranslator) TranslateAndExecute(
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
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return nil
	}

	if targetModel != "" {
		aReq["model"] = targetModel
	}

	newBodyBytes, _ := json.Marshal(aReq)

	targetURL := translator.BuildTargetURL(ch, targetEndpoint, "/messages")

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(newBodyBytes))
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

	if r != nil {
		for k, vv := range r.Header {
			if strings.ToLower(k) == "anthropic-beta" {
				for _, v := range vv {
					proxyReq.Header.Add(k, v)
				}
			}
		}
	}

	resp, err := httpclient.Client.Do(proxyReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	translator.CopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	stream, _ := aReq["stream"].(bool)
	if stream {
		translator.ForwardStreamBody(w, resp.Body)
	} else {
		_, _ = io.Copy(w, resp.Body)
	}
	return nil
}
