package openai

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// OpenAIToVertex 将 OpenAI Chat Completions 请求转发到 Vertex AI 端点
// 自动在模型名前添加 "google/" 前缀（满足 Vertex OpenAI 兼容端点的要求）
// 带有 ProjectID 的 Vertex 节点使用查询参数 ?key= 认证，否则使用 Bearer Token
func OpenAIToVertex(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := utils.IdentifyClient(r)
	methodName := utils.ExtractMethodName(r.URL.Path)

	targetURL := utils.BuildTargetURL(dest.Node.AccountDetail, r.URL.Path)
	currentBody := bodyBytes

	if dest.Node.ProjectID != "" {
		if !bytes.Contains(currentBody, []byte(`"model":"google/`)) && !bytes.Contains(currentBody, []byte(`"model": "google/`)) {
			currentBody = bytes.ReplaceAll(currentBody, []byte(`"model":"`), []byte(`"model":"google/`))
			currentBody = bytes.ReplaceAll(currentBody, []byte(`"model": "`), []byte(`"model": "google/`))
		}
	}

	if dest.TargetModel != "" {
		currentBody = bytes.ReplaceAll(currentBody, []byte(fmt.Sprintf(`"model":"%s"`, utils.ExtractModelName(currentBody))), []byte(fmt.Sprintf(`"model":"google/%s"`, dest.TargetModel)))
		currentBody = bytes.ReplaceAll(currentBody, []byte(fmt.Sprintf(`"model": "%s"`, utils.ExtractModelName(currentBody))), []byte(fmt.Sprintf(`"model": "google/%s"`, dest.TargetModel)))
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

	// Probation 探路日志由 ExecuteAndStream 统一处理
	utils.ExecuteAndStream(w, proxyReq, dest, "vertex", clientType, methodName, traceID, "OAI→Vertex",
		func(finalResp *http.Response, startTime time.Time) {
			streamAndSettleUsage(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime)
		})
}

func init() {
	router.RegisterTranslator("openai", "vertex", OpenAIToVertex)
}