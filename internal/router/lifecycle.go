// 节点生命周期管理：网络错误处理 + HTTP 状态检查 + 节点状态结算 + 统一执行封装
// 此文件是负载均衡核心的关键组件，控制节点的健康状态转换（Idle/Busy/Cooldown/Exhausted）
// 所有协议转换器的 HTTP 执行最终都通过此文件与节点状态机交互
package router

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"polaris-gateway/internal/db"
)

// HandleNetworkError 处理向上游代理时的网络级错误，触发节点惩罚
func HandleNetworkError(err error, dest *MatchedDestination, platform, clientType, methodName, traceID, logPrefix string) {
	errMsg := err.Error()
	dest.MarkFinalized()
	db.SaveUsage(platform, dest.Node.Name, clientType, methodName, 0, 0, 0, http.StatusBadGateway)
	dest.Node.UpdateOnFailure(dest.IsProbationRun, traceID)
	slog.Error("物理网络断联", "prefix", logPrefix, "trace_id", traceID, "account", dest.Node.Name, "error", errMsg)
}

// CheckResponseStatus 统一验证上游 HTTP 状态码，读取 Body 判断是否触达 Quota Exceeded 额度耗尽，记录节点错误或警告并保存到账单日志
func CheckResponseStatus(finalResp *http.Response, dest *MatchedDestination, platform, clientType, methodName, traceID, logPrefix string) (isNodeFailure, isQuotaExhausted bool) {
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
		slog.Warn("节点异常/限流，记入熔断惩罚队列", "prefix", logPrefix, "trace_id", traceID, "account", dest.Node.Name, "status", statusCode)
	} else if statusCode >= 400 {
		errBody, _ := io.ReadAll(finalResp.Body)
		finalResp.Body.Close()
		finalResp.Body = io.NopCloser(bytes.NewReader(errBody))

		db.SaveUsage(platform, dest.Node.Name, clientType, methodName, 0, 0, 0, statusCode)
		slog.Warn("客户端业务请求参数错误", "prefix", logPrefix, "trace_id", traceID, "account", dest.Node.Name, "status", statusCode, "body", string(errBody))
	}

	return isNodeFailure, isQuotaExhausted
}

// FinalizeNodeState 在流式输出结束后，根据探测到的错误情况更新节点的状态（成功、失败或永久封禁）
func FinalizeNodeState(dest *MatchedDestination, isNodeFailure, isQuotaExhausted bool, traceID string) {
	dest.MarkFinalized()
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

// StreamHandler 定义了流式/非流式响应的处理回调
// 返回值 streamFailed: true 表示在流式传输过程中发生不可恢复错误（如 Vertex SSE 中途报错），
// 调用方（ExecuteAndStream）会将此信息合并到 isNodeFailure，触发节点惩罚
type StreamHandler func(finalResp *http.Response, startTime time.Time) (streamFailed bool)

// ExecuteAndStream 包装了调用大模型 API 的完整生命周期：
// 包括探路日志、请求执行、异常捕获、额度判断、执行协议回写回调、状态刷新
//
// 所有协议转换器必须通过此函数发起 HTTP 请求，禁止直接调用 httpClient.Do()
// 转换器只需要负责：
//  1. 构造 proxyReq（URL + Headers + Body 格式转换）
//  2. 提供 streamHandler 回调处理响应内容（格式转换 + 计费）
//
// 外层 ExecuteAndStream 负责：
//  1. 发起 HTTP 请求（SharedHTTPClient.Do）
//  2. 网络错误处理（HandleNetworkError）
//  3. HTTP 状态码检查（CheckResponseStatus）
//  4. 错误响应透传给客户端
//  5. 节点状态结算（FinalizeNodeState）
func ExecuteAndStream(
	w http.ResponseWriter,
	proxyReq *http.Request,
	dest *MatchedDestination,
	platform string,
	clientType string,
	methodName string,
	traceID string,
	logPrefix string,
	streamHandler StreamHandler,
) error {
	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路", "prefix", logPrefix, "trace_id", traceID, "account", dest.Node.Name)
	}

	startTime := time.Now()
	finalResp, err := SharedHTTPClient.Do(proxyReq)

	if err != nil {
		HandleNetworkError(err, dest, platform, clientType, methodName, traceID, logPrefix)
		return fmt.Errorf("network error: %w", err)
	}

	isNodeFailure, isQuotaExhausted := CheckResponseStatus(finalResp, dest, platform, clientType, methodName, traceID, logPrefix)

	if isNodeFailure {
		// 上游节点级错误 (如 429, 500)，属于可重试错误。不调用 streamHandler，也不向 w 写入
		FinalizeNodeState(dest, true, isQuotaExhausted, traceID)

		errBody, _ := io.ReadAll(finalResp.Body)
		finalResp.Body.Close()

		return fmt.Errorf("upstream node failure: %d, body: %s", finalResp.StatusCode, string(errBody))
	}

	// 此时属于正常的业务响应（200 OK，或 400 客户端错误），将响应体交给转换器流式返回
	if streamHandler != nil {
		streamFailed := streamHandler(finalResp, startTime)
		if streamFailed {
			isNodeFailure = true
		}
	}

	FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
	return nil
}
