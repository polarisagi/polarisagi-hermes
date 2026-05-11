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

// VertexToVertex 将 Vertex 原生请求转发到 Vertex 后端协议直通：不做协议转换，仅替换端点 URL 和认证方式
func VertexToVertex(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := utils.IdentifyClient(r)
	methodName := utils.ExtractMethodName(r.URL.Path)

	targetURL := buildVertexTargetURL(dest.Node, r.URL.Path)

	proxyReq, _ := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	for k, vv := range r.Header {
		if !strings.EqualFold(k, "Host") && !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Accept-Encoding") && !strings.EqualFold(k, "Authorization") {
			for _, v := range vv {
				proxyReq.Header.Add(k, v)
			}
		}
	}

	q := proxyReq.URL.Query()
	for k, vv := range r.URL.Query() {
		for _, v := range vv {
			q.Add(k, v)
		}
	}
	q.Set("key", dest.Node.Credentials)
	proxyReq.URL.RawQuery = q.Encode()

	utils.ExecuteAndStream(w, proxyReq, dest, "vertex", clientType, methodName, traceID, "Vertex",
		func(finalResp *http.Response, startTime time.Time) {
			streamVertexResponse(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime, bodyBytes)
		})
}

// streamVertexResponse 流式转发 Vertex 上游响应到客户端，同步从尾部提取 usageMetadata 完成计费
func streamVertexResponse(w http.ResponseWriter, finalResp *http.Response, dest *router.MatchedDestination, modelName, clientType, methodName, traceID string, startTime time.Time, reqBody []byte) {
	defer finalResp.Body.Close()

	for k, vv := range finalResp.Header {
		if !strings.EqualFold(k, "Content-Length") && !strings.EqualFold(k, "Content-Encoding") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(finalResp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 8192)
	var tailBuf []byte
	const tailWindowSize = 8192
	var totalWritten int64

	for {
		n, err := finalResp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
			totalWritten += int64(n)
			tailBuf = append(tailBuf, buf[:n]...)
			if len(tailBuf) > tailWindowSize {
				tailBuf = tailBuf[len(tailBuf)-tailWindowSize:]
			}
		}
		if err != nil {
			break
		}
	}

	prompt, completion, cached, found := utils.ParseUsageFromStreamTail(tailBuf)
	if !found {
		prompt = utils.EstimatePromptTokens(reqBody)
		completion = utils.EstimateCompletionTokens(totalWritten)
		slog.Warn("⚠️ 响应流中断，启用 token 估算补偿", "trace_id", traceID, "node", dest.Node.Name, "prompt", prompt, "completion", completion)
	}

	if prompt > 0 || completion > 0 {
		cost := utils.CalculateCost(dest.Node.Provider, modelName, prompt, completion, cached, reqBody)
		db.SaveUsage("vertex", dest.Node.Name, clientType, methodName, prompt, completion, cost, finalResp.StatusCode)
		dest.Node.RecordCost(cost, traceID)
		slog.Info("💰 结算成功", "trace_id", traceID, "node", dest.Node.Name, "model", modelName, "cost", fmt.Sprintf("%.4f", cost))
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