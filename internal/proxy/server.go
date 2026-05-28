package proxy

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"polaris-hermes/internal/domain"
	"polaris-hermes/internal/service/channel"
	"polaris-hermes/internal/service/router"
	"polaris-hermes/internal/translator"
	"polaris-hermes/pkg/logger"
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

func detectPayloadProtocol(reqMap map[string]interface{}) string {
	// 检查是否有顶级 system 字段 (Anthropic 独有)
	if _, ok := reqMap["system"]; ok {
		return "anthropic"
	}
	// Anthropic 必须包含 max_tokens
	_, hasMaxTokens := reqMap["max_tokens"]
	// 检查 messages 内部是否有 role="system" (OpenAI 特有，Anthropic 不允许在 messages 中用 system)
	hasOpenAISystem := false
	if msgs, ok := reqMap["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if msgMap, ok := m.(map[string]interface{}); ok {
				if role, _ := msgMap["role"].(string); role == "system" {
					hasOpenAISystem = true
				}
			}
		}
	}
	// Google 原生一般包含 contents 而非 messages
	if _, ok := reqMap["contents"]; ok {
		return "google"
	}

	if hasOpenAISystem {
		return "openai"
	}
	if hasMaxTokens {
		return "anthropic" // Anthropic 强制要求 max_tokens，OpenAI 偶尔有
	}
	
	// 默认退化为不确定
	return ""
}

// 协议优先级矩阵: 针对不同的客户端协议，我们首选什么协议作为后端请求
var targetPriority = map[string][]string{
	"anthropic": {"anthropic", "google", "openai"},
	"openai":    {"openai", "anthropic", "google"},
	"google":    {"google", "openai", "anthropic"},
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

	path := r.URL.Path
	var clientProtocol string
	if strings.HasPrefix(path, "/v1/openai/") {
		clientProtocol = "openai"
	} else if strings.HasPrefix(path, "/v1/anthropic/") {
		clientProtocol = "anthropic"
	} else if strings.HasPrefix(path, "/v1/google/") {
		clientProtocol = "google"
	} else {
		http.Error(w, "Invalid gateway endpoint. Please use /v1/openai/, /v1/anthropic/, or /v1/google/", http.StatusNotFound)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var reqMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &reqMap); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Payload Validation: 拦截错配的报文
	detected := detectPayloadProtocol(reqMap)
	if detected != "" && detected != clientProtocol {
		// e.g. Client requested /v1/anthropic/ but sent OpenAI payload
		// 特例：OpenAI 也可以包含 max_tokens，不能单靠 max_tokens 判断它是 anthropic 并拦截 openai 客户端。
		// 但如果我们在 /v1/anthropic/ 收到 role="system" 的 openai 报文，则报错。
		if (clientProtocol == "anthropic" && detected == "openai") || (clientProtocol == "openai" && detected == "anthropic") || (clientProtocol != "google" && detected == "google") {
			slog.Warn("Protocol Payload Mismatch", "client_protocol", clientProtocol, "detected_payload", detected)
			http.Error(w, "Protocol Payload Mismatch: You connected to /v1/"+clientProtocol+"/ but sent a "+detected+" payload.", http.StatusBadRequest)
			return
		}
	}

	// 提取 Model 字段（三大协议通常第一层或深层都有 model，但为了兼容性最好手写一点逻辑，或者直接要求客户端传）
	var modelName string
	if m, ok := reqMap["model"].(string); ok {
		modelName = m
	}
	if modelName == "" {
		http.Error(w, "Missing 'model' parameter in payload", http.StatusBadRequest)
		return
	}

	if logger.IsDebugEnabled() {
		slog.Debug("Client Request Debug Info",
			"url", r.URL.String(),
			"method", r.Method,
			"client_protocol", clientProtocol,
			"model", modelName,
			"body", string(bodyBytes),
		)
	}

	// 呼叫大脑：4级智能降维路由
	activeChan, actualModel, err := s.pipeline.RouteRequest(r.Context(), modelName)
	if err != nil {
		slog.Error("路由匹配失败", "requested_model", modelName, "error", err)
		if clientProtocol == "anthropic" {
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

	// 智能挑选 TargetProtocol 和 TargetEndpoint
	var targetProtocol string
	var targetEndpoint *domain.SysAccessEndpoint
	priorityList := targetPriority[clientProtocol]
	
	for _, p := range priorityList {
		if ep, exists := activeChan.Endpoints[p]; exists {
			targetProtocol = p
			targetEndpoint = ep
			break
		}
	}
	
	if targetProtocol == "" {
		slog.Error("无兼容的后端协议端点", "channel", activeChan.Provider.Name, "client_protocol", clientProtocol)
		http.Error(w, "No compatible backend protocol endpoint for channel", http.StatusBadGateway)
		return
	}

	// 组装翻译器键名: "clientProtocol_targetProtocol"
	translatorKey := clientProtocol + "_" + targetProtocol
	trans := s.transFactory.GetTranslator(translatorKey)
	
	if trans == nil {
		slog.Warn("未找到对应的翻译器插件", "translator_key", translatorKey, "provider", activeChan.Provider.ProviderID)
		if clientProtocol == "anthropic" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"Translator not implemented: ` + translatorKey + `"}}`))
		} else {
			http.Error(w, "Translator not implemented: "+translatorKey, http.StatusNotImplemented)
		}
		return
	}

	// [Dynamic Prefix Logic] 为 OpenAI 端点动态补充模型前缀
	if targetProtocol == "openai" && targetEndpoint.ProviderID == "gemini_enterprise_agent_platform" {
		if strings.HasPrefix(actualModel, "gemini-claude-") || strings.HasPrefix(actualModel, "claude-") {
			actualModel = "anthropic/" + actualModel
		} else {
			actualModel = "google/" + actualModel
		}
	}

	// 将整个网络请求周期交给对应大厂的翻译器处理
	if err := trans.TranslateAndExecute(r.Context(), w, r, bodyBytes, activeChan, targetEndpoint, actualModel); err != nil {
		slog.Error("翻译器执行失败", "error", err)
	}
}

