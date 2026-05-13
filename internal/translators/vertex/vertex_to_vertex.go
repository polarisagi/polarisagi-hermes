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
	// 认证策略：
	//   - GEAP 合作伙伴模型 (publishers/anthropic/...) 强制 OAuth Bearer Token
	//   - Gemini 短路径或 publishers/google 兼容 API Key 查询参数（向后兼容）
	if strings.Contains(targetURL, "publishers/anthropic/") {
		proxyReq.Header.Set("Authorization", "Bearer "+dest.Node.Credentials)
	} else {
		q.Set("key", dest.Node.Credentials)
	}
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

// buildVertexTargetURL 构建 Gemini Enterprise Agent Platform (原 Vertex AI) 端点 URL
// 支持三种入站路径：
//  1. 短路径 `/models/X:method`              → 自动套用 publishers/google 前缀（向后兼容）
//  2. 完整路径 `/publishers/{pub}/models/X:method` → 保留客户端指定的发布者（google/anthropic 等）
//  3. 完整路径 `/projects/.../locations/.../publishers/...` → 视为绝对 GEAP 路径，直接拼接到 host
//
// 当 ProjectID 为空时退化为旧式 BaseURL + /v1 + path 拼接（用于非 GEAP 的 Gemini API 兼容端点）
func buildVertexTargetURL(node *router.NodeState, incomingPath string) string {
	subPath := strings.TrimPrefix(incomingPath, "/v1")
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}
	trimmedSub := strings.TrimPrefix(subPath, "/")

	if node.ProjectID != "" {
		location := node.Location
		if location == "" {
			location = "global"
		}

		// 客户端已写完整 /projects/.../locations/.../publishers/... 路径：直接套到 GEAP host 上
		if strings.HasPrefix(trimmedSub, "projects/") {
			host := node.BaseURL
			if host == "" {
				host = inferGEAPHost(location)
			} else {
				// 用户在 BaseURL 配置了 host 占位的模板时，提取 host 部分
				host = stripTemplatePath(host)
			}
			return strings.TrimSuffix(host, "/") + "/v1/" + trimmedSub
		}

		template := node.BaseURL
		if template == "" {
			template = inferGEAPHost(location) + "/v1/projects/{project_id}/locations/{location}/{publisher_subpath}"
		}

		// 客户端路径已包含 publishers/{pub}/... → 整体作为 publisher_subpath
		// 否则默认套用 publishers/google/ 前缀（向后兼容 Gemini 模型短路径）
		var publisherSubpath string
		if strings.HasPrefix(trimmedSub, "publishers/") {
			publisherSubpath = trimmedSub
		} else {
			publisherSubpath = "publishers/google/" + trimmedSub
		}

		resURL := strings.ReplaceAll(template, "{project_id}", node.ProjectID)
		resURL = strings.ReplaceAll(resURL, "{location}", location)
		// 同时兼容旧的 {subpath} 占位（自动加 publishers/google/ 前缀）
		resURL = strings.ReplaceAll(resURL, "{subpath}", trimmedSub)
		resURL = strings.ReplaceAll(resURL, "{publisher_subpath}", publisherSubpath)

		return resURL
	}

	baseURL := strings.TrimSuffix(node.BaseURL, "/")
	return baseURL + "/v1" + subPath
}

// inferGEAPHost 根据 location 推断 GEAP API host
// global 端点用 aiplatform.googleapis.com，区域端点用 {region}-aiplatform.googleapis.com
func inferGEAPHost(location string) string {
	if location == "" || location == "global" {
		return "https://aiplatform.googleapis.com"
	}
	return "https://" + location + "-aiplatform.googleapis.com"
}

// stripTemplatePath 从 BaseURL 模板中提取 scheme://host 部分，丢弃 /v1/... 等模板路径段
func stripTemplatePath(template string) string {
	idx := strings.Index(template, "://")
	if idx < 0 {
		return template
	}
	rest := template[idx+3:]
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return template
	}
	return template[:idx+3+slash]
}

func init() {
	router.RegisterTranslator("vertex", "vertex", VertexToVertex)
}