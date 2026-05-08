package router

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"polaris-gateway/internal/webapi"
)

type TranslatorFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *MatchedDestination, traceID string)

var Translators = make(map[string]TranslatorFunc)

func RegisterTranslator(incomingProtocol, targetProvider string, f TranslatorFunc) {
	key := fmt.Sprintf("%s_to_%s", incomingProtocol, targetProvider)
	Translators[key] = f
}

func getIncomingProtocol(path string) string {
	if strings.Contains(path, "chat/completions") || strings.Contains(path, "embeddings") || strings.Contains(path, "models") {
		return "openai"
	}
	if strings.Contains(path, "messages") {
		return "anthropic"
	}
	return "unknown"
}

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	traceID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method == http.MethodGet && (r.URL.Path == "/v1" || r.URL.Path == "/v1/" || r.URL.Path == "/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "Polaris Gateway Universal Router Active"}`))
		return
	}

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()

	sourceProtocol := getIncomingProtocol(r.URL.Path)
	if sourceProtocol == "unknown" {
		http.Error(w, `{"error": "Unsupported protocol endpoint"}`, http.StatusBadRequest)
		return
	}

	modelName := extractModelName(bodyBytes, sourceProtocol)
	if modelName == "" {
		http.Error(w, `{"error": "Missing model field in request"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()

	atomic.AddInt32(&webapi.WaitingCount, 1)
	dest, err := MatchAndAcquireRoute(ctx, sourceProtocol, modelName)
	atomic.AddInt32(&webapi.WaitingCount, -1)

	if err != nil || dest == nil {
		slog.Error("路由分发失败或队列超时", "trace_id", traceID, "source_protocol", sourceProtocol, "model", modelName, "error", err)
		http.Error(w, fmt.Sprintf("Polaris Gateway: No active routes available for model %s (%s) or queue timeout", modelName, sourceProtocol), http.StatusServiceUnavailable)
		return
	}

	atomic.AddInt32(&webapi.ActiveCount, 1)
	defer atomic.AddInt32(&webapi.ActiveCount, -1)
	defer ReleaseNode(dest.Node.ID)

	translatorKey := fmt.Sprintf("%s_to_%s", sourceProtocol, dest.TargetProtocol)
	translator, exists := Translators[translatorKey]

	if !exists {
		slog.Error("未找到对应的协议转换器", "trace_id", traceID, "key", translatorKey)
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
		http.Error(w, fmt.Sprintf("Polaris Gateway: No translator available for %s", translatorKey), http.StatusNotImplemented)
		return
	}

	slog.Info("🔗 [路由转发]", "trace_id", traceID, "source_protocol", sourceProtocol, "target_protocol", dest.TargetProtocol, "target_node", dest.Node.Name, "original_model", modelName, "target_model", dest.TargetModel)

	// Inject usage for OpenAI streams
	if sourceProtocol == "openai" {
		if bytes.Contains(bodyBytes, []byte(`"stream": true`)) || bytes.Contains(bodyBytes, []byte(`"stream":true`)) {
			if !bytes.Contains(bodyBytes, []byte(`"include_usage"`)) {
				bodyBytes = bytes.Replace(bodyBytes, []byte(`"stream": true`), []byte(`"stream": true, "stream_options": {"include_usage": true}`), 1)
				bodyBytes = bytes.Replace(bodyBytes, []byte(`"stream":true`), []byte(`"stream":true,"stream_options":{"include_usage":true}`), 1)
			}
		}
	}

	translator(ctx, w, r, bodyBytes, dest, traceID)
}
