// OpenAI 直通处理器
// OpenAI → OpenAI 同协议直通：仅做模型名替换和认证头注入，请求体格式保持不变
package openai

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"polaris-gateway/internal/router"
)

// OpenAIToOpenAI 处理 OpenAI 协议到 OpenAI 兼容后端的直通转发
// 仅做: 模型名替换 + API Key 注入 + 头透传，不做协议格式转换
func OpenAIToOpenAI(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := router.IdentifyClient(r)
	methodName := router.ExtractMethodName(r.URL.Path)

	targetURL := strings.TrimSuffix(dest.Node.BaseURL, "/")
	if targetURL == "" {
		targetURL = "https://api.openai.com/v1"
	}
	subPath := strings.TrimPrefix(r.URL.Path, "/v1")
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}
	targetURL = targetURL + subPath

	// 缓存原始模型名：避免在第二次 ReplaceAll 时 ExtractModelName 读到已被替换后的值
	originalModel := router.ExtractModelNameFromBody(bodyBytes)
	if dest.TargetModel != "" && dest.TargetModel != originalModel {
		bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(fmt.Sprintf(`"model":"%s"`, originalModel)), []byte(fmt.Sprintf(`"model":"%s"`, dest.TargetModel)))
		bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(fmt.Sprintf(`"model": "%s"`, originalModel)), []byte(fmt.Sprintf(`"model": "%s"`, dest.TargetModel)))
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	
	// Query transfer
	proxyReq.URL.RawQuery = r.URL.RawQuery

	for k, vv := range r.Header {
		if !strings.EqualFold(k, "Host") && !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Accept-Encoding") && !strings.EqualFold(k, "Authorization") {
			for _, v := range vv {
				proxyReq.Header.Add(k, v)
			}
		}
	}
	proxyReq.Header.Set("Authorization", "Bearer "+dest.Node.Credentials)

	// OpenAIToOpenAI 本身原本没有输出 Probation 探路日志，这里交给 ExecuteAndStream 统一处理
	router.ExecuteAndStream(w, proxyReq, dest, "openai", clientType, methodName, traceID, "OAI",
		func(finalResp *http.Response, startTime time.Time) {
			streamAndSettleUsage(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime, bodyBytes)
		})
}

func init() {
	router.RegisterTranslator("openai", "openai", OpenAIToOpenAI)
}