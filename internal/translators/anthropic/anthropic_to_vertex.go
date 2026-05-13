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
	"polaris-gateway/internal/translators/utils"
)

// httpClient 复用 utils.SharedHTTPClient 共享 TCP 连接池
// 取本地别名以保留原有调用语法；如需修改超时/Transport 请改 utils.SharedHTTPClient
var httpClient = utils.SharedHTTPClient

// AnthropicToVertex 将 Anthropic Messages API 请求转换为 Vertex GenerateContent API 格式
// 转换流程: 解析 Anthropic 消息 → mapToVertexRequest 转换格式 → 发送到 Vertex 端点 → 流式/非流式回写 Anthropic 格式
func AnthropicToVertex(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	// 解析请求体以获取 Stream/Model 等字段
	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type": "error", "error": {"type": "invalid_request_error", "message": "invalid json"}}`, 400)
		return
	}

	// 路由解析后的最终模型 = TargetModel（路由 target 字段，非空时覆盖）|| req.Model（透传客户端）
	// 路由 match 字段对客户端模型名支持通配（*、prefix-*），target 字段为空时表示透传 req.Model
	// 分流策略：最终模型以 claude- 开头 → 走 GEAP Claude 直通；其他 → 走 Gemini 转换
	// 覆盖场景：
	//   match=claude-*, target=claude-sonnet-4-6  → 强制走 GEAP（Google 官方 Claude）
	//   match=claude-*, target=gemini-2.5-pro     → 强制走 Gemini 转换
	//   match=claude-*, target=""                 → 透传 req.Model 到 GEAP Claude（同名 Google 官方模型）
	//   match=*, target=""                        → 透传客户端模型名，按是否 claude- 自动分流
	finalModel := dest.TargetModel
	if finalModel == "" {
		finalModel = req.Model
	}
	useGEAPClaude := isClaudeModel(finalModel)

	// count_tokens 端点分流：Claude Code 的 /context 命令与自动 compact 触发依赖此端点
	if isCountTokensPath(r.URL.Path) {
		if useGEAPClaude {
			handleGEAPClaudeCountTokens(ctx, w, bodyBytes, dest, traceID, finalModel)
		} else {
			handleVertexCountTokens(ctx, w, bodyBytes, dest, traceID)
		}
		return
	}

	if useGEAPClaude {
		passthroughToGEAPClaude(ctx, w, r, bodyBytes, dest, traceID, finalModel)
		return
	}

	clientType := "Anthropic-Adapter"

	// Gemini 路径：把 Anthropic 请求体映射为 Vertex GenerateContent 格式
	vReq, _ := mapToVertexRequest(req)
	vReqBytes, _ := json.Marshal(vReq)

	// 此处 finalModel 必然不是 claude-*（已被上方分流截获）
	model := finalModel
	if model == "" {
		model = "gemini-1.5-pro" // 兜底：路由透传但客户端也未指定模型
	}

	targetURL := buildAnthropicVertexTargetURL(dest.Node, model, req.Stream)

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路 (Anthropic Adapter)", "trace_id", traceID, "account", dest.Node.Name)
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(vReqBytes))
	proxyReq.Header.Set("Content-Type", "application/json")

	q := proxyReq.URL.Query()
	q.Set("key", dest.Node.Credentials)
	if req.Stream {
		q.Set("alt", "sse")
	}
	proxyReq.URL.RawQuery = q.Encode()

	finalResp, err := httpClient.Do(proxyReq)
	if err != nil {
		utils.HandleNetworkError(w, err, dest, "vertex", clientType, "anthropic_adapter", traceID, "Anthropic(Vertex)")
		return
	}

	isNodeFailure, isQuotaExhausted := utils.CheckResponseStatus(finalResp, dest, "vertex", clientType, "anthropic_adapter", traceID, "Anthropic(Vertex)")

	if finalResp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(finalResp.Body)
		finalResp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(finalResp.StatusCode)
		errResp := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "api_error",
				"message": fmt.Sprintf("Vertex API returned status %d: %s", finalResp.StatusCode, string(errBody)),
			},
		}
		json.NewEncoder(w).Encode(errResp)
		utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
		return
	}

	if req.Stream {
		streamOK := streamAnthropicResponse(ctx, w, finalResp, req, traceID, dest, clientType, model, bodyBytes)
		if !streamOK {
			isNodeFailure = true
		}
	} else {
		handleAnthropicNonStreamResponse(w, finalResp, req, traceID, dest, clientType, model, bodyBytes)
	}

	utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
}

// buildAnthropicVertexTargetURL 构建 Anthropic→Vertex 转发的目标 URL
// 支持模板变量 {project_id}, {location}, {subpath}，与 Vertex 原生转发保持一致
func buildAnthropicVertexTargetURL(node *router.NodeState, model string, stream bool) string {
	template := node.BaseURL
	if template == "" {
		template = "https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/google/{subpath}"
	}

	location := node.Location
	if location == "" {
		location = "global"
	}

	endpoint := "generateContent"
	if stream {
		endpoint = "streamGenerateContent"
	}

	subpath := fmt.Sprintf("models/%s:%s", model, endpoint)

	resURL := strings.ReplaceAll(template, "{project_id}", node.ProjectID)
	resURL = strings.ReplaceAll(resURL, "{location}", location)
	resURL = strings.ReplaceAll(resURL, "{subpath}", subpath)

	return resURL
}

func init() {
	router.RegisterTranslator("anthropic", "vertex", AnthropicToVertex)
}