package togoogle

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"polaris-hermes/internal/domain"
	"polaris-hermes/internal/pkg/httpclient"
	"polaris-hermes/internal/service/channel"
	"polaris-hermes/internal/translator"
)

type GoogleToGoogleTranslator struct{}

func NewGoogleToGoogleTranslator() *GoogleToGoogleTranslator {
	return &GoogleToGoogleTranslator{}
}

func (t *GoogleToGoogleTranslator) TranslateAndExecute(
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

	// Update model in URL path if targetModel is specified.
	// We don't need to change the body because Google API passes model in URL mostly,
	// but we can update it in body just in case.
	if targetModel != "" {
		gReq["model"] = targetModel
	}
	newBodyBytes, _ := json.Marshal(gReq)

	subPath := strings.TrimPrefix(r.URL.Path, "/v1/google")
	if targetModel != "" {
		// Replace model in subpath e.g. /models/gemini-pro:generateContent
		parts := strings.Split(subPath, ":")
		if len(parts) > 1 {
			pathPrefix := parts[0] // e.g. /models/gemini-pro
			pathPrefixParts := strings.Split(pathPrefix, "/")
			if len(pathPrefixParts) > 0 {
				pathPrefixParts[len(pathPrefixParts)-1] = targetModel
			}
			subPath = strings.Join(pathPrefixParts, "/") + ":" + parts[1]
		}
	}

	targetURL := translator.BuildTargetURL(ch, targetEndpoint, subPath)

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(newBodyBytes))
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

	// Preserve SSE flag if the client requested it
	if strings.Contains(r.URL.RawQuery, "alt=sse") {
		q.Set("alt", "sse")
	}

	proxyReq.URL.RawQuery = q.Encode()

	resp, err := httpclient.Client.Do(proxyReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	translator.CopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if strings.Contains(proxyReq.URL.RawQuery, "alt=sse") || strings.Contains(r.URL.Path, "stream") {
		translator.ForwardStreamBody(w, resp.Body)
	} else {
		io.Copy(w, resp.Body)
	}
	return nil
}
