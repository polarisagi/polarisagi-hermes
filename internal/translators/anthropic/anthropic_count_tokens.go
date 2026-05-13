// Anthropic /v1/messages/count_tokens 端点支持
// Claude Code 客户端使用此端点驱动 /context 命令显示 token 占用、
// 以及自动判断是否触发 /compact 上下文压缩。
//
// 协议层面：与 /v1/messages 共享同一请求体 schema，响应为 {"input_tokens": N}。
//
// 本网关实现策略：
//   - anthropic→anthropic 透传：转发到上游真实的 count_tokens 端点，最高精度
//   - anthropic→google (GEAP Claude)：转发到 GEAP rawPredict count-tokens 端点，精确计数
//   - anthropic→google (Gemini)：调用 Gemini countTokens 端点，精确计数
//   - anthropic→openai：本地估算（OpenAI 无对等端点）
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"polaris-gateway/internal/router"
)

// isCountTokensPath 判断请求路径是否为 count_tokens 端点
// 路由层已剥离协议前缀，此处看到的路径形如 /v1/messages/count_tokens
func isCountTokensPath(path string) bool {
	return strings.Contains(path, "/count_tokens")
}

// estimateAnthropicTokens 本地估算 Anthropic Messages 请求的 input token 数
// 经验法则：英文文本约 4 字节/token、图片基线 1500、工具 schema 序列化后按文本估算
// 估算误差对 Claude Code 的 /context 显示与 /compact 触发判断完全可接受
func estimateAnthropicTokens(req MessageRequest) int {
	total := 0

	// System prompt：支持字符串或内容块数组
	switch sys := req.System.(type) {
	case string:
		total += len(sys) / 4
	case []interface{}:
		for _, item := range sys {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					total += len(t) / 4
				}
			}
		}
	}

	// Messages
	for _, msg := range req.Messages {
		switch v := msg.Content.(type) {
		case string:
			total += len(v) / 4
		case []interface{}:
			for _, item := range v {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				switch m["type"] {
				case "text":
					if t, ok := m["text"].(string); ok {
						total += len(t) / 4
					}
				case "image", "document":
					// 图片/PDF 按 Anthropic 官方经验值近似
					total += 1500
				case "tool_use":
					if input, ok := m["input"]; ok {
						b, _ := json.Marshal(input)
						total += len(b) / 4
					}
					if name, ok := m["name"].(string); ok {
						total += len(name) / 4
					}
				case "tool_result":
					if c, ok := m["content"].(string); ok {
						total += len(c) / 4
					} else if arr, ok := m["content"].([]interface{}); ok {
						for _, ci := range arr {
							if cm, ok := ci.(map[string]interface{}); ok {
								if t, ok := cm["text"].(string); ok {
									total += len(t) / 4
								}
							}
						}
					}
				case "thinking":
					// Claude Code 历史中保留的思考块仍计入上下文
					if t, ok := m["thinking"].(string); ok {
						total += len(t) / 4
					}
				}
			}
		}
		total += 4 // role 等结构开销
	}

	// Tools：name + description + input_schema
	for _, tool := range req.Tools {
		total += len(tool.Name) / 4
		total += len(tool.Description) / 4
		if tool.InputSchema != nil {
			b, _ := json.Marshal(tool.InputSchema)
			total += len(b) / 4
		}
	}

	return total
}

// handleCountTokensLocal 在网关本地估算后直接返回 Anthropic 格式响应
// 适用于上游协议不是 Anthropic 的场景（Vertex、OpenAI），因为这些协议
// 没有可对等的 count_tokens 端点。
func handleCountTokensLocal(w http.ResponseWriter, bodyBytes []byte, traceID string) {
	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"invalid json"}}`, http.StatusBadRequest)
		return
	}

	tokens := estimateAnthropicTokens(req)

	slog.Debug("📏 [CountTokens] 本地估算返回", "trace_id", traceID, "input_tokens", tokens, "model", req.Model)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]int{
		"input_tokens": tokens,
	})
}

// handleCountTokensPassthrough 把 count_tokens 请求透传到上游 Anthropic 节点
// 适用于 anthropic→anthropic 透传场景，可获得最精确的 token 数
func handleCountTokensPassthrough(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	targetURL := strings.TrimSuffix(dest.Node.BaseURL, "/")
	if targetURL == "" {
		targetURL = "https://api.anthropic.com/v1"
	}
	subPath := strings.TrimPrefix(r.URL.Path, "/v1")
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}
	targetURL = targetURL + subPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		slog.Error("❌ [CountTokens] 构建上游请求失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}
	for k, vv := range r.Header {
		if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Content-Length") ||
			strings.EqualFold(k, "Accept-Encoding") || strings.EqualFold(k, "Authorization") ||
			strings.EqualFold(k, "X-Api-Key") {
			continue
		}
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}
	proxyReq.Header.Set("x-api-key", dest.Node.Credentials)
	proxyReq.Header.Set("anthropic-version", "2023-06-01")
	proxyReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(proxyReq)
	if err != nil {
		slog.Warn("⚠️ [CountTokens] 上游请求失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		slog.Warn("⚠️ [CountTokens] 上游返回非 200，降级本地估算",
			"trace_id", traceID, "status", resp.StatusCode,
			"body_preview", string(respBody[:min(len(respBody), 200)]))
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBody)
}

// handleVertexCountTokens 调用 Vertex 的 :countTokens 端点获取精确 token 数
// 把 Anthropic 请求映射成 Vertex GenerateContent 体后调用 countTokens 端点
// 失败时降级到本地估算
func handleVertexCountTokens(ctx context.Context, w http.ResponseWriter, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"invalid json"}}`, http.StatusBadRequest)
		return
	}

	model := req.Model
	if dest.TargetModel != "" {
		model = dest.TargetModel
	} else if model == "" || strings.Contains(model, "claude") {
		model = "gemini-1.5-pro"
	}

	vReq, _ := mapToVertexRequest(req)
	// countTokens 端点只接受 contents/systemInstruction/tools，剥离 generationConfig
	delete(vReq, "generationConfig")
	vReqBytes, _ := json.Marshal(vReq)

	targetURL := buildGEAPURL(dest.Node, "google", fmt.Sprintf("models/%s:countTokens", model), "global")

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(vReqBytes))
	if err != nil {
		slog.Warn("⚠️ [CountTokens] Vertex 请求构建失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	q := proxyReq.URL.Query()
	q.Set("key", dest.Node.Credentials)
	proxyReq.URL.RawQuery = q.Encode()

	resp, err := httpClient.Do(proxyReq)
	if err != nil {
		slog.Warn("⚠️ [CountTokens] Vertex 调用失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		slog.Warn("⚠️ [CountTokens] Vertex 返回非 200，降级本地估算", "trace_id", traceID, "status", resp.StatusCode, "resp_body", string(respBody))
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}

	// 打印 Vertex 原始响应以便排查
	slog.Debug("📏 [CountTokens] Vertex 原生响应内容", "trace_id", traceID, "body", string(respBody))

	var vertexResp map[string]interface{}
	if err := json.Unmarshal(respBody, &vertexResp); err != nil {
		slog.Warn("⚠️ [CountTokens] Vertex 响应解析失败，降级本地估算", "trace_id", traceID, "error", err)
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}

	var totalTokens int
	if val, ok := vertexResp["totalTokens"].(float64); ok {
		totalTokens = int(val)
	} else if val, ok := vertexResp["totalTokenCount"].(float64); ok {
		totalTokens = int(val)
	}

	slog.Debug("📏 [CountTokens] Vertex 精确计数提取",
		"trace_id", traceID, "input_tokens", totalTokens, "model", model)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]int{
		"input_tokens": totalTokens,
	})
}
