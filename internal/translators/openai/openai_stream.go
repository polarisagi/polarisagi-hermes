// OpenAI 流式响应处理 + 用量结算
// 从上游后端读取 SSE 流，边写回客户端边在尾部 buffer 中缓存最近 8KB 数据
// 流结束后从尾部 buffer 中提取 usage 字段完成计费
package openai

import (
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
func streamAndSettleUsage(w http.ResponseWriter, finalResp *http.Response, dest *router.MatchedDestination, modelName, clientType, methodName, traceID string, startTime time.Time, reqBody []byte) {
	defer finalResp.Body.Close()
	for k, vv := range finalResp.Header {
		if !strings.EqualFold(k, "Content-Length") && !strings.EqualFold(k, "Content-Encoding") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(finalResp.StatusCode)

	tailBuf, totalWritten := utils.ForwardStreamBody(w, finalResp.Body)

	prompt, completion, cached, found := utils.ParseUsageFromStreamTail(tailBuf)
	if !found {
		// 客户端中断导致 stream 提前结束，未能获取最后一块的 usageMetadata
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