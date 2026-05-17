package openai

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// OpenAIToGoogle 将 OpenAI Chat Completions 请求转发到 Google Agent Platform 端点
// 官方 REST API 参考：https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest
// 自动在模型名前添加 "google/" 前缀（满足 GEAP OpenAI 兼容端点的要求）
// 带有 ProjectID 的节点使用查询参数 ?key= 认证，否则使用 Bearer Token
func OpenAIToGoogle(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
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
		// 缓存原始模型名：第二次 ReplaceAll 读取必须用替换前的值
		originalModel := utils.ExtractModelName(currentBody)
		currentBody = bytes.ReplaceAll(currentBody, []byte(fmt.Sprintf(`"model":"%s"`, originalModel)), []byte(fmt.Sprintf(`"model":"google/%s"`, dest.TargetModel)))
		currentBody = bytes.ReplaceAll(currentBody, []byte(fmt.Sprintf(`"model": "%s"`, originalModel)), []byte(fmt.Sprintf(`"model": "google/%s"`, dest.TargetModel)))
	}

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation OAI 账号探路", "trace_id", traceID, "account", dest.Node.Name)
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

	// 统一注入 Google 认证信息 (自动处理 ADC Token 或 API Key)
	if err := dest.Node.InjectGoogleAuth(proxyReq); err != nil {
		slog.Error("❌ [OpenAIToGoogle] 注入认证信息失败", "node", dest.Node.Name, "err", err)
		http.Error(w, "Failed to generate ADC Token", http.StatusInternalServerError)
		return
	}

	// 对于 OpenAI 兼容协议（特别是 Gemini 的），有时需要以 Bearer 形式传入原始 API Key
	if dest.Node.TokenSource == nil && dest.Node.ProjectID == "" {
		proxyReq.Header.Set("Authorization", "Bearer "+dest.Node.Credentials)
	}

	// Probation 探路日志由 ExecuteAndStream 统一处理
	utils.ExecuteAndStream(w, proxyReq, dest, "google", clientType, methodName, traceID, "OAI→Google Agent Platform",
		func(finalResp *http.Response, startTime time.Time) {
			streamAndSettleUsage(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime, currentBody)
		})
}

func init() {
	router.RegisterTranslator("openai", "google", OpenAIToGoogle)
}