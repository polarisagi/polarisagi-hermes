// Package router 提供多协议路由分发引擎
// 核心职责：
// 1. 接收客户端请求，检测源协议类型（OpenAI/Anthropic/Google Agent Platform）
// 2. 提取请求中的模型名，匹配路由表中的映射规则
// 3. 从节点池中选择可用的目标节点（负载均衡）
// 4. 调用协议转换器将请求转发到上游
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
)

// TranslatorFunc 协议转换器函数签名
// ctx: 请求上下文（含 180s 超时）
// w: HTTP 响应写入器
// r: 原始 HTTP 请求（路径已清洗）
// bodyBytes: 请求体字节
// dest: 匹配到的目标路由（含节点、目标模型、目标协议）
// traceID: 请求追踪 ID
type TranslatorFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *MatchedDestination, traceID string) error

// Translators 全局协议转换器注册表
// key 格式: "{源协议}_to_{目标协议}", 例如 "anthropic_to_google"
var Translators = make(map[string]TranslatorFunc)

// RegisterTranslator 注册一个协议转换器
// incomingProtocol: 客户端使用的协议 (openai/anthropic/google)
// targetProvider: 上游节点的协议类型 (openai/anthropic/google)
func RegisterTranslator(incomingProtocol, targetProvider string, f TranslatorFunc) {
	key := fmt.Sprintf("%s_to_%s", incomingProtocol, targetProvider)
	Translators[key] = f
}

// CountTokensHandlers 独立处理 count_tokens 请求的短路处理器
var CountTokensHandlers = make(map[string]func(w http.ResponseWriter, bodyBytes []byte, traceID string))

// RegisterCountTokensHandler 注册 count_tokens 本地计算器
func RegisterCountTokensHandler(protocol string, f func(w http.ResponseWriter, bodyBytes []byte, traceID string)) {
	CountTokensHandlers[protocol] = f
}

// ServeHTTP 是所有 API 请求的统一入口处理函数
// 完整请求处理流程:
//  1. 读取请求体 → 2. 检测源协议 → 3. 清洗 URL 路径
//  4. 提取模型名 → 5. 匹配路由和节点 → 6. 查找转换器 → 7. 转发请求
func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 生成唯一请求追踪 ID，用于日志关联
	traceID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	slog.Debug("📥 [入口] 请求到达", "trace_id", traceID, "method", r.Method, "path", r.URL.Path, "user_agent", r.UserAgent())

	// 处理 CORS 预检请求
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 处理健康检查/根路径探测请求
	if r.Method == http.MethodGet && (r.URL.Path == "/v1" || r.URL.Path == "/v1/" || r.URL.Path == "/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "Polaris Gateway Universal Router Active"}`))
		return
	}

	// 步骤1：读取完整请求体，后续根据协议解析模型名
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()

	// 步骤2：从 URL 路径检测源协议类型
	sourceProtocol := getIncomingProtocol(r.URL.Path)

	// 严格校验：如果识别到的协议与路径中包含的业务端点特征不匹配，直接拦截报错，拒绝妥协适配
	if sourceProtocol == "google" && (strings.Contains(r.URL.Path, "chat/completions") || strings.Contains(r.URL.Path, "embeddings") || strings.Contains(r.URL.Path, "messages")) {
		slog.Warn("⚠️ [入口] 客户端请求协议与端点方法不匹配", "trace_id", traceID, "source_protocol", sourceProtocol, "path", r.URL.Path)
		http.Error(w, `{"error": {"message": "Protocol mismatch: The requested endpoint (e.g. chat/completions) is not supported by the Google protocol. Please check your client's BaseURL configuration.", "type": "invalid_request_error", "code": "protocol_mismatch"}}`, http.StatusBadRequest)
		return
	}
	if sourceProtocol == "openai" && (strings.Contains(r.URL.Path, "messages") || strings.Contains(r.URL.Path, "generateContent")) {
		slog.Warn("⚠️ [入口] 客户端请求协议与端点方法不匹配", "trace_id", traceID, "source_protocol", sourceProtocol, "path", r.URL.Path)
		http.Error(w, `{"error": {"message": "Protocol mismatch: The requested endpoint is not supported by the OpenAI protocol. Please check your client's BaseURL configuration.", "type": "invalid_request_error", "code": "protocol_mismatch"}}`, http.StatusBadRequest)
		return
	}
	if sourceProtocol == "anthropic" && (strings.Contains(r.URL.Path, "chat/completions") || strings.Contains(r.URL.Path, "generateContent")) {
		slog.Warn("⚠️ [入口] 客户端请求协议与端点方法不匹配", "trace_id", traceID, "source_protocol", sourceProtocol, "path", r.URL.Path)
		http.Error(w, `{"error": {"message": "Protocol mismatch: The requested endpoint is not supported by the Anthropic protocol. Please check your client's BaseURL configuration.", "type": "invalid_request_error", "code": "protocol_mismatch"}}`, http.StatusBadRequest)
		return
	}

	// 步骤3：清洗 URL 路径 — 移除协议前缀段 (/v1/vertex/models/... → /v1/models/...)
	// 这样下游转换器接收到的是干净的路径，无需关心协议前缀
	r.URL.Path = stripProtocolPrefix(r.URL.Path)
	slog.Debug("🔧 [入口] URL 路径清洗后", "trace_id", traceID, "clean_path", r.URL.Path, "source_protocol", sourceProtocol)
	slog.Debug("🔍 [入口] 协议检测结果", "trace_id", traceID, "source_protocol", sourceProtocol, "path", r.URL.Path, "body_size", len(bodyBytes))

	// 无法识别协议则返回 400
	if sourceProtocol == "unknown" {
		slog.Warn("⚠️ [入口] 无法识别协议", "trace_id", traceID, "path", r.URL.Path, "body_preview", string(bodyBytes[:min(len(bodyBytes), 200)]))
		http.Error(w, `{"error": "Unsupported protocol endpoint"}`, http.StatusBadRequest)
		return
	}

	// 拦截处理模型列表请求 (GET /models, /v1/models, /v1/v1/models 等)
	if r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/models") || strings.HasSuffix(r.URL.Path, "/models/")) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if sourceProtocol == "anthropic" {
			_, _ = w.Write([]byte(`{"data": [{"type": "model", "id": "claude-3-7-sonnet-20250219", "display_name": "Claude 3.7 Sonnet", "created_at": "2025-02-19T00:00:00Z"}, {"type": "model", "id": "claude-3-5-sonnet-20241022", "display_name": "Claude 3.5 Sonnet", "created_at": "2024-10-22T00:00:00Z"}, {"type": "model", "id": "claude-3-5-haiku-20241022", "display_name": "Claude 3.5 Haiku", "created_at": "2024-10-22T00:00:00Z"}], "has_more": false}`))
		} else if sourceProtocol == "google" {
			_, _ = w.Write([]byte(`{"models": [{"name": "models/gemini-2.5-pro", "version": "2.5", "displayName": "Gemini 2.5 Pro", "supportedGenerationMethods": ["generateContent", "countTokens"]}, {"name": "models/gemini-2.5-flash", "version": "2.5", "displayName": "Gemini 2.5 Flash", "supportedGenerationMethods": ["generateContent", "countTokens"]}, {"name": "models/gemini-3.1-flash-lite-preview", "version": "3.1", "displayName": "Gemini 3.1 Flash Lite Preview", "supportedGenerationMethods": ["generateContent", "countTokens"]}]}`))
		} else {
			_, _ = w.Write([]byte(`{"object": "list", "data": [{"id": "gpt-4o", "object": "model", "created": 1715368132, "owned_by": "system"}, {"id": "gpt-4o-mini", "object": "model", "created": 1715368132, "owned_by": "system"}, {"id": "o1", "object": "model", "created": 1715368132, "owned_by": "system"}, {"id": "o3-mini", "object": "model", "created": 1715368132, "owned_by": "system"}, {"id": "gemini-2.5-pro", "object": "model", "created": 1715368132, "owned_by": "system"}, {"id": "gemini-2.5-flash", "object": "model", "created": 1715368132, "owned_by": "system"}]}`))
		}
		return
	}

	// 拦截 GET 且 body 为空的请求——这类请求是客户端工具（如 /doctor、/plugins）的健康探测，
	// 不含模型信息，无法路由，直接返回 200 避免误报 WARN 日志
	if r.Method == http.MethodGet && len(bodyBytes) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
		return
	}

	// 拦截 Ollama 的模型探测请求 (/api/show)
	// 很多 AI Agent（如 Hermes）使用 Ollama 协议来探测模型是否存在
	// 返回伪造的成功响应以通过客户端的检查
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/api/show") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"modelfile": "","parameters": "","template": "","details": {"parent_model": "","format": "gguf","family": "polaris","families": ["polaris"],"parameter_size": "unknown","quantization_level": ""}}`))
		return
	}

	// 步骤4：从请求体或 URL 路径提取模型名
	// Google Agent Platform 原生协议：先从 body JSON 的 model 字段提取，失败则从 URL 路径提取
	modelName := extractModelName(bodyBytes, sourceProtocol)

	// 短路拦截：如果是 Anthropic 的 count_tokens 请求，直接在内存中计算并返回，不消耗任何上游节点锁
	if sourceProtocol == "anthropic" && strings.Contains(r.URL.Path, "/count_tokens") {
		if handler, ok := CountTokensHandlers["anthropic"]; ok {
			slog.Debug("🚀 [入口] 拦截 count_tokens 请求，执行本地纯内存计算", "trace_id", traceID, "model", modelName)
			handler(w, bodyBytes, traceID)
			return
		}
	}

	// Google Agent Platform 原生协议的特殊处理：body 中可能没有 model 字段，需要从 URL 路径提取
	if sourceProtocol == "google" && (modelName == "" || modelName == "_google_native_") {
		modelName = extractModelFromGooglePath(r.URL.Path)
		slog.Debug("🔍 [入口] Google Agent Platform 从 URL 路径提取模型名", "trace_id", traceID, "model", modelName, "path", r.URL.Path)
	}

	slog.Debug("📥 [入口] 模型提取结果", "trace_id", traceID, "source_protocol", sourceProtocol, "model", modelName)

	if modelName == "" || modelName == "_google_native_" {
		if len(bodyBytes) == 0 {
			// 空 body 请求是客户端连通性探测（如 Claude Code 启动时 POST /v1/anthropic），静默返回 200
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))
			return
		}
		slog.Warn("⚠️ [入口] 无法提取模型名", "trace_id", traceID, "source_protocol", sourceProtocol, "path", r.URL.Path, "body_preview", string(bodyBytes[:min(len(bodyBytes), 200)]))
		http.Error(w, `{"error": "Missing model field in request"}`, http.StatusBadRequest)
		return
	}

	// 步骤5：创建带超时的请求上下文（600秒），超时时自动取消下游请求
	ctx, cancel := context.WithTimeout(r.Context(), 600*time.Second)
	defer cancel()

	// 步骤6：路由分配与节点获取 — 在排队等待中轮询可用路由和节点
	const MaxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= MaxRetries; attempt++ {
		if ctx.Err() != nil {
			slog.Debug("🔌 [入口] 客户端断开，终止重试", "trace_id", traceID, "model", modelName)
			return
		}

		if attempt > 1 {
			slog.Warn("🔁 [重试机制] 节点不可用，发起自动重试 (跨节点/跨协议)", "trace_id", traceID, "attempt", attempt, "max_retries", MaxRetries, "last_error", lastErr)
			time.Sleep(100 * time.Millisecond) // 轻微退避防止风暴
		}

		atomic.AddInt32(&WaitingCount, 1)
		dest, err := MatchAndAcquireRoute(ctx, sourceProtocol, modelName)
		atomic.AddInt32(&WaitingCount, -1)

		// 路由匹配失败：无可用路由或无空闲节点 -> 无法继续重试，直接跳出
		if err != nil || dest == nil {
			if err != nil && ctx.Err() != nil {
				slog.Debug("🔌 [入口] 客户端断开，排队请求退出", "trace_id", traceID, "model", modelName)
				return
			}
			lastErr = err
			slog.Error("路由分发失败或队列超时，无法继续重试", "trace_id", traceID, "source_protocol", sourceProtocol, "model", modelName, "error", err)
			break
		}

		// 极端窗口保护：客户端可能在 tryAcquire 成功的瞬间断开
		if ctx.Err() != nil {
			ReleaseNode(dest)
			slog.Debug("🔌 [入口] 客户端在 acquire 节点的瞬间断开，立即归还", "trace_id", traceID, "node", dest.Node.Name)
			return
		}

		// 步骤7：节点已分配，增加活跃连接计数
		atomic.AddInt32(&ActiveCount, 1)

		// 步骤8：查找协议转换器
		translatorKey := fmt.Sprintf("%s_to_%s", sourceProtocol, dest.TargetProtocol)
		translator, exists := Translators[translatorKey]

		if !exists {
			slog.Error("未找到对应的协议转换器", "trace_id", traceID, "key", translatorKey)
			dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
			atomic.AddInt32(&ActiveCount, -1)
			ReleaseNode(dest)
			lastErr = fmt.Errorf("no translator available for %s", translatorKey)
			break // 配置缺失，重试也没有意义
		}

		slog.Info("🔗 [路由转发]", "trace_id", traceID, "source_protocol", sourceProtocol, "target_protocol", dest.TargetProtocol, "target_node", dest.Node.Name, "original_model", modelName, "target_model", dest.TargetModel, "attempt", attempt)

		slog.Debug("🔄 [协议转换] 开始执行翻译器", "trace_id", traceID, "translator_key", translatorKey, "target_node", dest.Node.Name, "is_probation", dest.IsProbationRun)

		// OpenAI 流式请求自动注入 stream_options.include_usage
		if sourceProtocol == "openai" {
			if bytes.Contains(bodyBytes, []byte(`"stream": true`)) || bytes.Contains(bodyBytes, []byte(`"stream":true`)) {
				if !bytes.Contains(bodyBytes, []byte(`"include_usage"`)) {
					bodyBytes = bytes.Replace(bodyBytes, []byte(`"stream": true`), []byte(`"stream": true, "stream_options": {"include_usage": true}`), 1)
					bodyBytes = bytes.Replace(bodyBytes, []byte(`"stream":true`), []byte(`"stream":true,"stream_options":{"include_usage":true}`), 1)
				}
			}
		}

		err = translator(ctx, w, r, bodyBytes, dest, traceID)

		atomic.AddInt32(&ActiveCount, -1)
		ReleaseNode(dest) // dest.finalized 会防止二次释放

		if err == nil {
			// 请求成功，直接退出
			return
		}

		// 发生可重试错误（如节点 429, 500, 或网络断开），记录错误并进入下一次重试循环
		lastErr = err
		slog.Warn("⚠️ [网关异常] 节点请求失败，准备尝试其他节点或降级协议", "trace_id", traceID, "node", dest.Node.Name, "error", err)
	}

	// 所有重试均失败，返回 502 Bad Gateway
	slog.Error("❌ [网关异常] 路由所有重试均失败", "trace_id", traceID, "last_error", lastErr)
	http.Error(w, fmt.Sprintf("Polaris Gateway: All retry attempts failed. Last error: %v", lastErr), http.StatusBadGateway)
}
