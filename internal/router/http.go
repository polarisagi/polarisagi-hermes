package router

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	traceID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	slog.Debug("📥 [入口] 请求到达", "trace_id", traceID, "method", r.Method, "path", r.URL.Path, "user_agent", r.UserAgent())

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

	// Strip the protocol prefix from the URL so translators receive clean paths
	// /v1/openai/chat/completions → /v1/chat/completions
	r.URL.Path = stripProtocolPrefix(r.URL.Path)
	slog.Debug("🔧 [入口] URL 路径清洗后", "trace_id", traceID, "clean_path", r.URL.Path, "source_protocol", sourceProtocol)
	slog.Debug("🔍 [入口] 协议检测结果", "trace_id", traceID, "source_protocol", sourceProtocol, "path", r.URL.Path, "body_size", len(bodyBytes))

	if sourceProtocol == "unknown" {
		slog.Warn("⚠️ [入口] 无法识别协议", "trace_id", traceID, "path", r.URL.Path, "body_preview", string(bodyBytes[:min(len(bodyBytes), 200)]))
		http.Error(w, `{"error": "Unsupported protocol endpoint"}`, http.StatusBadRequest)
		return
	}

	modelName := extractModelName(bodyBytes, sourceProtocol)

	// For Vertex native protocol, extract model name from URL path if not found in body
	if sourceProtocol == "vertex" && (modelName == "" || modelName == "_vertex_native_") {
		modelName = extractModelFromVertexPath(r.URL.Path)
		slog.Debug("🔍 [入口] Vertex 从 URL 路径提取模型名", "trace_id", traceID, "model", modelName, "path", r.URL.Path)
	}

	slog.Debug("📥 [入口] 模型提取结果", "trace_id", traceID, "source_protocol", sourceProtocol, "model", modelName)

	if modelName == "" || modelName == "_vertex_native_" {
		slog.Warn("⚠️ [入口] 无法提取模型名", "trace_id", traceID, "source_protocol", sourceProtocol, "path", r.URL.Path, "body_preview", string(bodyBytes[:min(len(bodyBytes), 200)]))
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

	slog.Debug("🔄 [协议转换] 开始执行翻译器", "trace_id", traceID, "translator_key", translatorKey, "target_node", dest.Node.Name, "is_probation", dest.IsProbationRun)

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
