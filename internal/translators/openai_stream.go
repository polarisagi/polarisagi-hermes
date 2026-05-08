package translators

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
)

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

	if bytes.Contains(tailBuf, []byte("usage")) || bytes.Contains(tailBuf, []byte("prompt_tokens")) {
		pMatch := openAIPromptRegex.FindSubmatch(tailBuf)
		cMatch := openAICompletionRegex.FindSubmatch(tailBuf)
		cacheMatch := openAICachedRegex.FindSubmatch(tailBuf)

		var p, c, cached int64
		if len(pMatch) > 1 {
			p = parseToInt(pMatch[1])
		}
		if len(cMatch) > 1 {
			c = parseToInt(cMatch[1])
		}
		if len(cacheMatch) > 1 {
			cached = parseToInt(cacheMatch[1])
		}

		if p > 0 || c > 0 {
			cost := calculateCost(modelName, p, c, cached)

			db.SaveUsage("openai", dest.Node.Name, clientType, methodName, p, c, cost, finalResp.StatusCode)
			dest.Node.RecordCost(cost, traceID)

			slog.Info("💰 结算成功", "trace_id", traceID, "node", dest.Node.Name, "model", modelName, "cost", fmt.Sprintf("%.4f", cost))
		}
	}
}
