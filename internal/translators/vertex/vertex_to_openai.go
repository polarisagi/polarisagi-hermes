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

// VertexToOpenAI 将 Vertex 原生请求转发到 OpenAI 兼容后端
// 自动去除模型名中的 "google/" 前缀，支持 Ge mini API Key 和标准 OpenAI 端点
func VertexToOpenAI(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	clientType := utils.IdentifyClient(r)
	methodName := utils.ExtractMethodName(r.URL.Path)

	targetURL := strings.TrimSuffix(dest.Node.BaseURL, "/")
	if targetURL == "" {
		targetURL = "https://api.openai.com/v1"
	}
	subPath := strings.TrimPrefix(r.URL.Path, "/v1")
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}
	targetURL = targetURL + subPath

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

	utils.ExecuteAndStream(w, proxyReq, dest, "openai", clientType, methodName, traceID, "Vertex→OpenAI",
		func(finalResp *http.Response, startTime time.Time) {
			streamAndSettleOpenAI(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime, currentBody)
		})
}

func streamAndSettleOpenAI(w http.ResponseWriter, finalResp *http.Response, dest *router.MatchedDestination, modelName, clientType, methodName, traceID string, startTime time.Time, reqBody []byte) {
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
		db.SaveUsage("openai", dest.Node.Name, clientType, methodName, prompt, completion, cost, finalResp.StatusCode)
		dest.Node.RecordCost(cost, traceID)
		slog.Info("💰 结算成功", "trace_id", traceID, "node", dest.Node.Name, "model", modelName, "cost", fmt.Sprintf("%.4f", cost))
	}
}

func init() {
	router.RegisterTranslator("vertex", "openai", VertexToOpenAI)
	router.RegisterTranslator("vertex", "gemini", VertexToOpenAI)
}
