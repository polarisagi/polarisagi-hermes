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
	"time"

	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

var httpClient = &http.Client{Timeout: 180 * time.Second}

// AnthropicToVertex 将 Anthropic Messages API 请求转换为 Vertex GenerateContent API 格式
// 转换流程: 解析 Anthropic 消息 → mapToVertexRequest 转换格式 → 发送到 Vertex 端点 → 流式/非流式回写 Anthropic 格式
func AnthropicToVertex(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := "Anthropic-Adapter"
	
	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type": "error", "error": {"type": "invalid_request_error", "message": "invalid json"}}`, 400)
		return
	}

	vReq, _ := mapToVertexRequest(req)
	vReqBytes, _ := json.Marshal(vReq)

	model := req.Model
	if dest.TargetModel != "" {
		model = dest.TargetModel
	} else if model == "" || strings.Contains(model, "claude") {
		model = "gemini-1.5-pro" // fallback
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(finalResp.StatusCode)
		errBody, _ := io.ReadAll(finalResp.Body)
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
		if !streamAnthropicResponse(w, finalResp, req, traceID, dest, clientType, model) {
			isNodeFailure = true
		}
	} else {
		handleAnthropicNonStreamResponse(w, finalResp, req, traceID, dest, clientType, model)
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