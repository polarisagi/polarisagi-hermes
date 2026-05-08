// Package router 提供多协议路由分发引擎
// 核心职责：
// 1. 接收客户端请求，检测源协议类型（OpenAI/Anthropic/Vertex）
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
	"sync/atomic"
	"time"

	"polaris-gateway/internal/webapi"
)

// TranslatorFunc 协议转换器函数签名
// ctx: 请求上下文（含 180s 超时）
// w: HTTP 响应写入器
// r: 原始 HTTP 请求（路径已清洗）
// bodyBytes: 请求体字节
// dest: 匹配到的目标路由（含节点、目标模型、目标协议）
// traceID: 请求追踪 ID
type TranslatorFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *MatchedDestination, traceID string)

// Translators 全局协议转换器注册表
// key 格式: "{源协议}_to_{目标协议}", 例如 "vertex_to_openai"
var Translators = make(map[string]TranslatorFunc)

// RegisterTranslator 注册一个协议转换器
// incomingProtocol: 客户端使用的协议 (openai/anthropic/vertex)
// targetProvider: 上游节点的协议类型 (openai/vertex/gemini)
func RegisterTranslator(incomingProtocol, targetProvider string, f TranslatorFunc) {
	key := fmt.Sprintf("%s_to_%s", incomingProtocol, targetProvider)
	Translators[key] = f
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
		w.Write([]byte(`{"status": "Polaris Gateway Universal Router Active"}`))
		return
	}

	// 步骤1：读取完整请求体，后续根据协议解析模型名
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()

	// 步骤2：从 URL 路径检测源协议类型
	sourceProtocol := getIncomingProtocol(r.URL.Path)

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

	// 步骤4：从请求体或 URL 路径提取模型名
	// Vertex 原生协议：先从 body JSON 的 model 字段提取，失败则从 URL 路径提取
	modelName := extractModelName(bodyBytes, sourceProtocol)

	// Vertex 原生协议的特殊处理：body 中可能没有 model 字段，需要从 URL 路径提取
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

	// 步骤5：创建带超时的请求上下文（180秒），超时时自动取消下游请求
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()

	// 步骤6：路由分配与节点获取 — 在排队等待中轮询可用路由和节点
	atomic.AddInt32(&webapi.WaitingCount, 1)
	dest, err := MatchAndAcquireRoute(ctx, sourceProtocol, modelName)
	atomic.AddInt32(&webapi.WaitingCount, -1)

	// 路由匹配失败：无可用路由或无空闲节点 → 返回 503
	if err != nil || dest == nil {
		slog.Error("路由分发失败或队列超时", "trace_id", traceID, "source_protocol", sourceProtocol, "model", modelName, "error", err)
		http.Error(w, fmt.Sprintf("Polaris Gateway: No active routes available for model %s (%s) or queue timeout", modelName, sourceProtocol), http.StatusServiceUnavailable)
		return
	}

	// 步骤7：节点已分配，增加活跃连接计数
	atomic.AddInt32(&webapi.ActiveCount, 1)
	defer atomic.AddInt32(&webapi.ActiveCount, -1)
	defer ReleaseNode(dest.Node.ID)

	// 步骤8：查找协议转换器 (如 "vertex_to_openai")
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

	// OpenAI 流式请求自动注入 stream_options.include_usage，确保能提取 token 用量
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
