// OpenAI 直通/转发处理器
// 支持 OpenAI → OpenAI（同协议直通）和 OpenAI → Gemini API Key（同为 OpenAI 兼容协议）
// 仅做模型名替换和认证头注入，请求体格式保持不变
package openai

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

var httpClient = &http.Client{Timeout: 180 * time.Second} // 全局 HTTP 客户端，180 秒超时

// OpenAIToOpenAI 处理 OpenAI 协议到 OpenAI/Gemini 兼容后端的转发
// 仅做: 模型名替换 + API Key 注入 + 头透传，不做协议格式转换
func OpenAIToOpenAI(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := utils.IdentifyClient(r)
	methodName := utils.ExtractMethodName(r.URL.Path)

	targetURL := dest.Node.BaseURL
	if targetURL == "" {
		targetURL = "https://api.openai.com/v1"
	}
	targetURL = strings.TrimSuffix(targetURL, "/") + r.URL.Path

	if dest.TargetModel != "" && dest.TargetModel != utils.ExtractModelName(bodyBytes) {
		bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(fmt.Sprintf(`"model":"%s"`, utils.ExtractModelName(bodyBytes))), []byte(fmt.Sprintf(`"model":"%s"`, dest.TargetModel)))
		bodyBytes = bytes.ReplaceAll(bodyBytes, []byte(fmt.Sprintf(`"model": "%s"`, utils.ExtractModelName(bodyBytes))), []byte(fmt.Sprintf(`"model": "%s"`, dest.TargetModel)))
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

	startTime := time.Now()
	finalResp, err := httpClient.Do(proxyReq)

	if err != nil {
		errMsg := err.Error()
		db.SaveUsage("openai", dest.Node.Name, clientType, methodName, 0, 0, 0, http.StatusBadGateway)
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
		slog.Error("OAI 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Polaris Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		db.SaveUsage("openai", dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("OAI 节点异常/限流，记入熔断惩罚队列", "trace_id", traceID, "status", statusCode)
	} else if statusCode >= 400 {
		db.SaveUsage("openai", dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("OAI 客户端业务请求参数错误", "trace_id", traceID, "status", statusCode)
	}

	streamAndSettleUsage(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime)

	if isNodeFailure {
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
	} else {
		dest.Node.UpdateOnSuccess()
	}
}

func init() {
	router.RegisterTranslator("openai", "openai", OpenAIToOpenAI)
	router.RegisterTranslator("openai", "gemini", OpenAIToOpenAI)
}