package utils

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"polaris-gateway/internal/db"
	"polaris-gateway/internal/router"
)

// HandleNetworkError 处理向上游代理时的网络级错误，直接向客户端返回 502 并触发节点惩罚
func HandleNetworkError(w http.ResponseWriter, err error, dest *router.MatchedDestination, platform, clientType, methodName, traceID, logPrefix string) {
	errMsg := err.Error()
	db.SaveUsage(platform, dest.Node.Name, clientType, methodName, 0, 0, 0, http.StatusBadGateway)
	dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
	slog.Error(fmt.Sprintf("%s 物理网络断联", logPrefix), "trace_id", traceID, "account", dest.Node.Name, "error", errMsg)
	http.Error(w, fmt.Sprintf("Polaris Gateway Network Error: %s", errMsg), http.StatusBadGateway)
}

// CheckResponseStatus 统一验证上游 HTTP 状态码，读取 Body 判断是否触达 Quota Exceeded 额度耗尽，记录节点错误或警告并保存到账单日志
func CheckResponseStatus(finalResp *http.Response, dest *router.MatchedDestination, platform, clientType, methodName, traceID, logPrefix string) (isNodeFailure, isQuotaExhausted bool) {
	statusCode := finalResp.StatusCode
	isNodeFailure = statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden

	if isNodeFailure {
		errBody, _ := io.ReadAll(finalResp.Body)
		finalResp.Body.Close()
		// 放回 Body 以便后续读取或透传
		finalResp.Body = io.NopCloser(bytes.NewReader(errBody))

		if statusCode == http.StatusTooManyRequests && (bytes.Contains(errBody, []byte("Quota exceeded")) || bytes.Contains(errBody, []byte("quota")) || bytes.Contains(errBody, []byte("insufficient_quota"))) {
			isQuotaExhausted = true
		}

		db.SaveUsage(platform, dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn(fmt.Sprintf("%s 节点异常/限流，记入熔断惩罚队列", logPrefix), "trace_id", traceID, "account", dest.Node.Name, "status", statusCode)
	} else if statusCode >= 400 {
		errBody, _ := io.ReadAll(finalResp.Body)
		finalResp.Body.Close()
		finalResp.Body = io.NopCloser(bytes.NewReader(errBody))

		db.SaveUsage(platform, dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn(fmt.Sprintf("%s 客户端业务请求参数错误", logPrefix), "trace_id", traceID, "account", dest.Node.Name, "status", statusCode, "body", string(errBody))
	}

	return isNodeFailure, isQuotaExhausted
}

// FinalizeNodeState 在流式输出结束后，根据探测到的错误情况更新节点的状态（成功、失败或永久封禁）
func FinalizeNodeState(dest *router.MatchedDestination, isNodeFailure, isQuotaExhausted bool, traceID string) {
	if isNodeFailure {
		if isQuotaExhausted {
			dest.Node.MarkAsExhausted("Quota Exceeded", traceID)
		} else {
			dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
		}
	} else {
		dest.Node.UpdateOnSuccess()
	}
}

// SharedHTTPClient 全局统一的 HTTP 客户端，用于访问各大模型平台
// 使用统一的 Transport 共享 TCP 连接池，避免高并发下连接膨胀
var SharedHTTPClient = &http.Client{
	Timeout:   180 * time.Second,
	Transport: sharedTransport,
}

// StreamHandler 定义了流式/非流式响应的处理回调
type StreamHandler func(finalResp *http.Response, startTime time.Time)

// ExecuteAndStream 包装了调用大模型 API 的完整生命周期：
// 包括探路日志、请求执行、异常捕获、额度判断、执行协议回写回调、状态刷新
func ExecuteAndStream(
	w http.ResponseWriter,
	proxyReq *http.Request,
	dest *router.MatchedDestination,
	platform string,
	clientType string,
	methodName string,
	traceID string,
	logPrefix string,
	streamHandler StreamHandler,
) {
	if dest.IsProbationRun {
		slog.Warn(fmt.Sprintf("⚠️ 启用 🟠 Probation 账号执行流量探路 (%s)", logPrefix), "trace_id", traceID, "account", dest.Node.Name)
	}

	startTime := time.Now()
	finalResp, err := SharedHTTPClient.Do(proxyReq)

	if err != nil {
		HandleNetworkError(w, err, dest, platform, clientType, methodName, traceID, logPrefix)
		return
	}

	isNodeFailure, isQuotaExhausted := CheckResponseStatus(finalResp, dest, platform, clientType, methodName, traceID, logPrefix)

	if streamHandler != nil {
		streamHandler(finalResp, startTime)
	}

	FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
}
