// Vertex 原生直通处理器
// 支持 Vertex → Vertex（同协议直通），使用 Vertex 原生的 generateContent/streamGenerateContent 端点
// 自动构建包含 project_id/location/model 的 GCP 端点 URL，并注入 API Key 认证
package vertex

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

var httpClient = &http.Client{Timeout: 180 * time.Second}

// VertexToVertex Vertex 原生协议直通：不做协议转换，仅替换端点 URL 和认证方式
func VertexToVertex(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := utils.IdentifyClient(r)
	methodName := utils.ExtractMethodName(r.URL.Path)

	targetURL := buildVertexTargetURL(dest.Node, r.URL.Path)

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路", "trace_id", traceID, "account", dest.Node.Name)
	}

	bodyReader := bytes.NewReader(bodyBytes)
	proxyReq, _ := http.NewRequestWithContext(ctx, r.Method, targetURL, bodyReader)
	proxyReq.Header.Del("Authorization")

	q := proxyReq.URL.Query()
	for k, vv := range r.URL.Query() {
		for _, v := range vv {
			q.Add(k, v)
		}
	}
	q.Set("key", dest.Node.Credentials)
	proxyReq.URL.RawQuery = q.Encode()

	for k, vv := range r.Header {
		if !strings.EqualFold(k, "Host") && !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Accept-Encoding") && !strings.EqualFold(k, "Authorization") {
			for _, v := range vv {
				proxyReq.Header.Add(k, v)
			}
		}
	}

	startTime := time.Now()
	finalResp, err := httpClient.Do(proxyReq)
	if err != nil {
		errMsg := err.Error()
		db.SaveUsage("vertex", dest.Node.Name, clientType, methodName, 0, 0, 0, http.StatusBadGateway)
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
		slog.Error("Vertex 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Vertex Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		db.SaveUsage("vertex", dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("Vertex 节点异常/限流，记入熔断惩罚队列", "trace_id", traceID, "status", statusCode)
	} else if statusCode >= 400 {
		db.SaveUsage("vertex", dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("Vertex 客户端业务请求参数错误", "trace_id", traceID, "status", statusCode)
	}

	streamVertexResponse(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime)

	if isNodeFailure {
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
	} else {
		dest.Node.UpdateOnSuccess()
	}
}

// streamVertexResponse 流式转发 Vertex 上游响应到客户端，同步从尾部提取 usageMetadata 完成计费
func streamVertexResponse(w http.ResponseWriter, finalResp *http.Response, dest *router.MatchedDestination, modelName, clientType, methodName, traceID string, startTime time.Time) {
	defer finalResp.Body.Close()

	for k, vv := range finalResp.Header {
		if !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Content-Encoding") &&
			!strings.EqualFold(k, "Transfer-Encoding") &&
			!strings.EqualFold(k, "Connection") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(finalResp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	var tailBuf []byte
	const tailWindowSize = 8192

	for {
		n, readErr := finalResp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				break
			}
			if flusher != nil {
				flusher.Flush()
			}

			tailBuf = append(tailBuf, buf[:n]...)
			if len(tailBuf) > tailWindowSize {
				tailBuf = tailBuf[len(tailBuf)-tailWindowSize:]
			}
		}
		if readErr != nil {
			break
		}
	}

	if bytes.Contains(tailBuf, []byte("usageMetadata")) || bytes.Contains(tailBuf, []byte("usage")) {
		prompt, completion, cached, found := utils.ParseUsageFromStreamTail(tailBuf)
		if found {
			cost := utils.CalculateCost(modelName, prompt, completion, cached)

			db.SaveUsage("vertex", dest.Node.Name, clientType, methodName, prompt, completion, cost, finalResp.StatusCode)
			dest.Node.RecordCost(cost, traceID)

			if cached > 0 {
				slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", prompt, "cached", cached, "completion", completion, "cost", fmt.Sprintf("%.4f", cost))
			} else {
				slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", prompt, "completion", completion, "cost", fmt.Sprintf("%.4f", cost))
			}
		}
	}
}

// buildVertexTargetURL 构建 Vertex 原生端点 URL
// 支持两种模式:
//  1. 有 ProjectID: 使用 GCP Agent Platform 路由格式，支持 {project_id}/{location}/{subpath} 模板
//  2. 无 ProjectID: 使用标准 BaseURL 拼接路径
func buildVertexTargetURL(node *router.NodeState, incomingPath string) string {
	subPath := strings.TrimPrefix(incomingPath, "/v1")
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}

	if node.ProjectID != "" {
		template := node.BaseURL
		if template == "" {
			template = "https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/google/{subpath}"
		}

		location := node.Location
		if location == "" {
			location = "global"
		}

		resURL := strings.ReplaceAll(template, "{project_id}", node.ProjectID)
		resURL = strings.ReplaceAll(resURL, "{location}", location)
		resURL = strings.ReplaceAll(resURL, "{subpath}", strings.TrimPrefix(subPath, "/"))

		return resURL
	}

	baseURL := strings.TrimSuffix(node.BaseURL, "/")
	return baseURL + "/v1" + subPath
}

func init() {
	router.RegisterTranslator("vertex", "vertex", VertexToVertex)
}