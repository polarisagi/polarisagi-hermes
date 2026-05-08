package translators

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
)

func OpenAIToVertex(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := identifyClient(r)
	methodName := extractMethodName(r.URL.Path)

	targetURL := buildTargetURL(dest.Node.AccountDetail, r.URL.Path)
	currentBody := bodyBytes

	if dest.Node.ProjectID != "" {
		if !bytes.Contains(currentBody, []byte(`"model":"google/`)) && !bytes.Contains(currentBody, []byte(`"model": "google/`)) {
			currentBody = bytes.ReplaceAll(currentBody, []byte(`"model":"`), []byte(`"model":"google/`))
			currentBody = bytes.ReplaceAll(currentBody, []byte(`"model": "`), []byte(`"model": "google/`))
		}
	}

	if dest.TargetModel != "" {
		currentBody = bytes.ReplaceAll(currentBody, []byte(fmt.Sprintf(`"model":"%s"`, extractModelName(currentBody))), []byte(fmt.Sprintf(`"model":"google/%s"`, dest.TargetModel)))
		currentBody = bytes.ReplaceAll(currentBody, []byte(fmt.Sprintf(`"model": "%s"`, extractModelName(currentBody))), []byte(fmt.Sprintf(`"model": "google/%s"`, dest.TargetModel)))
	}

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation OAI 账号探路", "trace_id", traceID, "account", dest.Node.Name)
	}

	parsedURL, err := url.Parse(targetURL)
	if err == nil && dest.Node.ProjectID != "" {
		q := parsedURL.Query()
		q.Set("key", dest.Node.Credentials)
		parsedURL.RawQuery = q.Encode()
		targetURL = parsedURL.String()
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(currentBody))

	for k, vv := range r.Header {
		if !strings.EqualFold(k, "Host") && !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Accept-Encoding") && !strings.EqualFold(k, "Authorization") {
			for _, v := range vv {
				proxyReq.Header.Add(k, v)
			}
		}
	}

	if dest.Node.ProjectID == "" {
		proxyReq.Header.Set("Authorization", "Bearer "+dest.Node.Credentials)
	}

	startTime := time.Now()
	finalResp, err := httpClient.Do(proxyReq)

	if err != nil {
		errMsg := err.Error()
		db.SaveUsage("vertex", dest.Node.Name, clientType, methodName, 0, 0, 0, http.StatusBadGateway)
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
		slog.Error("OAI_To_Vertex 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Polaris Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		db.SaveUsage("vertex", dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("OAI_To_Vertex 节点异常/限流，记入熔断惩罚队列", "trace_id", traceID, "status", statusCode)
	} else if statusCode >= 400 {
		db.SaveUsage("vertex", dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("OAI_To_Vertex 客户端业务请求参数错误", "trace_id", traceID, "status", statusCode)
	}

	streamAndSettleUsage(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime)

	if isNodeFailure {
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
	} else {
		dest.Node.UpdateOnSuccess()
	}
}

func init() {
	router.RegisterTranslator("openai", "vertex", OpenAIToVertex)
}
