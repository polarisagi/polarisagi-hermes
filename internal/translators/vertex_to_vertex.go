package translators

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
)

func VertexToVertex(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := identifyClient(r)
	methodName := extractMethodName(r.URL.Path)

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

	if bytes.Contains(tailBuf, []byte("usageMetadata")) {
		pMatch := promptRegex.FindSubmatch(tailBuf)
		cMatch := candidateRegex.FindSubmatch(tailBuf)
		cacheMatch := cachedContentRegex.FindSubmatch(tailBuf)

		if len(pMatch) > 1 && len(cMatch) > 1 {
			p := parseToInt(pMatch[1])
			c := parseToInt(cMatch[1])
			var cached int64
			if len(cacheMatch) > 1 {
				cached = parseToInt(cacheMatch[1])
			}

			cost := calculateCost(modelName, p, c, cached)

			db.SaveUsage("vertex", dest.Node.Name, clientType, methodName, p, c, cost, finalResp.StatusCode)
			dest.Node.RecordCost(cost, traceID)

			if cached > 0 {
				slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", p, "cached", cached, "completion", c, "cost", fmt.Sprintf("%.4f", cost))
			} else {
				slog.Info("💰 结算完成", "trace_id", traceID, "account", dest.Node.Name, "model", modelName, "prompt", p, "completion", c, "cost", fmt.Sprintf("%.4f", cost))
			}
		}
	}
}

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
