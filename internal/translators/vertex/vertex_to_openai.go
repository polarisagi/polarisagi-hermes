// Vertex 原生协议 → OpenAI/Chat Completions 转换器
// 接收 Vertex 原生格式请求，去除 google/ 模型名前缀后转发到 OpenAI 兼容后端
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

var oaiHTTPClient = &http.Client{Timeout: 180 * time.Second}

// VertexToOpenAI 将 Vertex 原生请求转发到 OpenAI 兼容后端
// 自动去除模型名中的 "google/" 前缀，支持 Ge mini API Key 和标准 OpenAI 端点
func VertexToOpenAI(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := utils.IdentifyClient(r)
	methodName := utils.ExtractMethodName(r.URL.Path)

	targetURL := strings.TrimSuffix(dest.Node.BaseURL, "/")
	if targetURL == "" {
		targetURL = "https://api.openai.com/v1"
	}
	targetURL = targetURL + r.URL.Path

	// Strip google/ prefix from model name if present (Vertex→OpenAI passthrough)
	currentBody := bodyBytes
	if bytes.Contains(currentBody, []byte(`"model":"google/`)) {
		currentBody = bytes.ReplaceAll(currentBody, []byte(`"model":"google/`), []byte(`"model":"`))
		currentBody = bytes.ReplaceAll(currentBody, []byte(`"model": "google/`), []byte(`"model": "`))
	}
	// Apply target model mapping if set
	if dest.TargetModel != "" {
		currentBody = bytes.ReplaceAll(currentBody, []byte(fmt.Sprintf(`"model":"%s"`, utils.ExtractModelName(currentBody))), []byte(fmt.Sprintf(`"model":"%s"`, dest.TargetModel)))
		currentBody = bytes.ReplaceAll(currentBody, []byte(fmt.Sprintf(`"model": "%s"`, utils.ExtractModelName(currentBody))), []byte(fmt.Sprintf(`"model": "%s"`, dest.TargetModel)))
	}

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路 (Vertex→OpenAI)", "trace_id", traceID, "account", dest.Node.Name)
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
	proxyReq.Header.Set("Authorization", "Bearer "+dest.Node.Credentials)

	startTime := time.Now()
	finalResp, err := oaiHTTPClient.Do(proxyReq)
	if err != nil {
		errMsg := err.Error()
		db.SaveUsage("openai", dest.Node.Name, clientType, methodName, 0, 0, 0, http.StatusBadGateway)
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
		slog.Error("Vertex→OpenAI 物理网络断联", "trace_id", traceID, "error", errMsg)
		http.Error(w, fmt.Sprintf("Polaris Gateway Network Error: %s", errMsg), http.StatusBadGateway)
		return
	}

	statusCode := finalResp.StatusCode
	isNodeFailure := statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		db.SaveUsage("openai", dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("Vertex→OpenAI 节点异常/限流，记入熔断惩罚队列", "trace_id", traceID, "status", statusCode)
	} else if statusCode >= 400 {
		db.SaveUsage("openai", dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("Vertex→OpenAI 客户端业务请求参数错误", "trace_id", traceID, "status", statusCode)
	}

	streamAndSettleOpenAI(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime)

	if isNodeFailure {
		dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
	} else {
		dest.Node.UpdateOnSuccess()
	}
}

func streamAndSettleOpenAI(w http.ResponseWriter, finalResp *http.Response, dest *router.MatchedDestination, modelName, clientType, methodName, traceID string, startTime time.Time) {
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

	for {
		n, err := finalResp.Body.Read(buf)
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
		if err != nil {
			break
		}
	}

	if bytes.Contains(tailBuf, []byte("usage")) || bytes.Contains(tailBuf, []byte("promptTokenCount")) {
		prompt, completion, cached, found := utils.ParseUsageFromStreamTail(tailBuf)
		if found {
			cost := utils.CalculateCost(modelName, prompt, completion, cached)
			db.SaveUsage("openai", dest.Node.Name, clientType, methodName, prompt, completion, cost, finalResp.StatusCode)
			dest.Node.RecordCost(cost, traceID)
			slog.Info("💰 结算成功", "trace_id", traceID, "node", dest.Node.Name, "model", modelName, "cost", fmt.Sprintf("%.4f", cost))
		}
	}
}

func init() {
	router.RegisterTranslator("vertex", "openai", VertexToOpenAI)
	router.RegisterTranslator("vertex", "gemini", VertexToOpenAI)
}
