package translators

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
)

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
		errMsg := err.Error()
		db.SaveUsage("vertex", dest.Node.Name, clientType, "anthropic_adapter", 0, 0, 0, http.StatusBadGateway)
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
		slog.Error("Anthropic(Vertex) 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		db.SaveUsage("vertex", dest.Node.Name, clientType, "anthropic_adapter", 0, 0, 0, statusCode)
		slog.Warn("Anthropic(Vertex) 节点异常/限流，记入熔断惩罚队列", "trace_id", traceID, "status", statusCode)
	} else if statusCode >= 400 {
		db.SaveUsage("vertex", dest.Node.Name, clientType, "anthropic_adapter", 0, 0, 0, statusCode)
		slog.Warn("Anthropic 客户端业务请求参数错误", "trace_id", traceID, "status", statusCode)
	}

	if req.Stream {
		streamAnthropicResponse(w, finalResp, req, traceID, dest, clientType, model)
	} else {
		handleAnthropicNonStreamResponse(w, finalResp, req, traceID, dest, clientType, model)
	}

	if isNodeFailure {
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
	} else {
		dest.Node.UpdateOnSuccess()
	}
}

func buildAnthropicVertexTargetURL(node *router.NodeState, model string, stream bool) string {
	baseURL := node.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1", node.Location)
	}
	endpoint := "generateContent"
	if stream {
		endpoint = "streamGenerateContent"
	}
	return fmt.Sprintf("%s/projects/%s/locations/%s/publishers/google/models/%s:%s",
		baseURL, node.ProjectID, node.Location, model, endpoint)
}

func init() {
	router.RegisterTranslator("anthropic", "vertex", AnthropicToVertex)
}
