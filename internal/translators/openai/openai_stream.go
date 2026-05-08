// OpenAI 流式响应处理 + 用量结算
// 从上游后端读取 SSE 流，边写回客户端边在尾部 buffer 中缓存最近 8KB 数据
// 流结束后从尾部 buffer 中提取 usage 字段完成计费
package openai

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// streamAndSettleUsage 流式转发上游响应到客户端，同时在尾部窗口收集 usage 数据完成计费
// 尾部窗口: 保留最后 8KB 数据用于解析 usage 字段（位于 SSE 流的最后几条消息中）
func streamAndSettleUsage(w http.ResponseWriter, finalResp *http.Response, dest *router.MatchedDestination, modelName, clientType, methodName, traceID string, startTime time.Time) {
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