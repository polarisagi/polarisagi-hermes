package proxy

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"polaris-hermes/internal/service/channel"
	"polaris-hermes/internal/service/router"
	"polaris-hermes/internal/translator"
)

// Server 网关的 HTTP 代理数据面
type Server struct {
	pipeline     *router.Pipeline
	chanManager  *channel.Manager
	transFactory *translator.TranslatorFactory
}

func NewServer(
	pipeline *router.Pipeline,
	chanManager *channel.Manager,
	transFactory *translator.TranslatorFactory,
) *Server {
	return &Server{
		pipeline:     pipeline,
		chanManager:  chanManager,
		transFactory: transFactory,
	}
}

// ServeHTTP 是网关处理一切客户端流量的主入口
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 简单 CORS 处理
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 支持 OpenAI 格式（/v1/chat/completions）和 Anthropic 原生格式（/v1/messages）。
	// Claude Code 使用 /v1/messages，Codex 使用 /v1/chat/completions。
	// 两种格式的请求体中均有 "model" 字段，可统一提取。
	isOpenAI := strings.HasPrefix(r.URL.Path, "/v1/chat/completions")
	isAnthropic := strings.HasPrefix(r.URL.Path, "/v1/messages")
	if !isOpenAI && !isAnthropic {
		http.Error(w, "Only /v1/chat/completions and /v1/messages are supported", http.StatusNotFound)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var baseReq struct {
		Model  string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &baseReq); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if baseReq.Model == "" {
		http.Error(w, "Missing 'model' parameter in payload", http.StatusBadRequest)
		return
	}

	// 呼叫大脑：4级智能降维路由
	activeChan, actualModel, err := s.pipeline.RouteRequest(r.Context(), baseReq.Model)
	if err != nil {
		slog.Error("路由匹配失败", "requested_model", baseReq.Model, "error", err)
		// 根据传入协议选择错误响应格式
		if isAnthropic {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"No available upstream models to serve this request"}}`))
		} else {
			http.Error(w, "No available upstream models to serve this request: "+err.Error(), http.StatusServiceUnavailable)
		}
		return
	}

	// 完美确保并发锁无论如何都能释放
	defer s.chanManager.ReleaseChannel(activeChan)

	// 呼叫协议翻译插件
	trans := s.transFactory.GetTranslator(activeChan.APIProtocol)
	if trans == nil {
		slog.Warn("未找到对应的翻译器插件，暂不支持该协议", "api_protocol", activeChan.APIProtocol, "provider", activeChan.Endpoint.ProviderID)
		if isAnthropic {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"Translator not implemented for protocol: ` + activeChan.APIProtocol + `"}}`))
		} else {
			http.Error(w, "Translator not implemented for protocol: "+activeChan.APIProtocol, http.StatusNotImplemented)
		}
		return
	}

	// 将整个网络请求周期交给对应大厂的翻译器处理
	if err := trans.TranslateAndExecute(r.Context(), w, r, bodyBytes, activeChan, actualModel); err != nil {
		slog.Error("翻译器执行失败", "error", err)
	}
}

