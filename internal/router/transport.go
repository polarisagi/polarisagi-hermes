// HTTP 传输层：全局连接池 + 流式转发 + URL 构建
// 此文件集中管理网关与上游 LLM API 的所有 HTTP 通信基础设施
// 所有协议转换器通过 SharedHTTPClient 发出请求，禁止自建 http.Client
package router

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"polaris-gateway/internal/config"
)

// sharedTransport 全局共享的 HTTP Transport，避免高并发下 TCP 连接膨胀
// 默认 Transport 没有连接池限制，每个 LLM 上游请求都可能新建 socket，
// Claude Code/opencode 等客户端瞬时几十个并发时会触发 EADDRINUSE / 文件句柄耗尽
//
// 调参依据：
//   - MaxIdleConns=200 覆盖单网关 ~20 个上游节点 × 10 并发的常见规模
//   - MaxIdleConnsPerHost=50 单个上游 LLM host 的 idle 连接上限
//   - IdleConnTimeout=90s 比 LLM 平均响应时长长，复用率最大化
//   - DisableCompression=false 让上游 gzip 响应能正常解压
var sharedTransport = &http.Transport{
	MaxIdleConns:          200,
	MaxIdleConnsPerHost:   50,
	MaxConnsPerHost:       0, // 0 = 不限并发连接，仅 idle 池有上限
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 600 * time.Second, // Claude Code compress 大上下文首 token 延迟可达 5min+
	ForceAttemptHTTP2:     true,
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		return dialer.DialContext(ctx, network, addr)
	},
}

// SharedHTTPClient 全局统一的 HTTP 客户端，用于访问各大模型平台
// 使用统一的 Transport 共享 TCP 连接池，避免高并发下连接膨胀
var SharedHTTPClient = &http.Client{
	Timeout:   600 * time.Second,
	Transport: sharedTransport,
}

// ForwardStreamBody 将 body 流式转发到 w，同时维护尾部 8KB 缓冲窗口
// 返回尾部缓冲（用于从流末提取 usage 字段）和累计写入字节数（用于 token 估算兜底）
// 调用方负责在调用前后进行 header 复制、w.WriteHeader 和 body.Close
func ForwardStreamBody(w http.ResponseWriter, body io.Reader) (tailBuf []byte, totalWritten int64) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 8192)
	const tailWindowSize = 8192

	for {
		n, readErr := body.Read(buf)
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
		if readErr != nil {
			break
		}
	}
	return tailBuf, totalWritten
}

// BuildTargetURL 实现多态路由分发，原生支持 Vertex 端点的多子路径拼接
func BuildTargetURL(acc config.AccountDetail, incomingPath string) string {
	// 1. 提取业务子路径 (例如 chat/completions)
	subPath := strings.TrimPrefix(incomingPath, "/v1")
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}

	// 2. Vertex OpenAPI 节点路由渲染
	if acc.ProjectID != "" {
		template := acc.BaseURL
		if template == "" {
			template = "https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/endpoints/openapi"
		}

		location := acc.Location
		if location == "" {
			location = "global"
		}

		resURL := strings.ReplaceAll(template, "{project_id}", acc.ProjectID)
		resURL = strings.ReplaceAll(resURL, "{location}", location)

		// 完美咬合 Google 官方规范：在 openapi 之后直接拼接方法名
		return strings.TrimSuffix(resURL, "/") + subPath
	}

	// 3. 标准 OpenAI 节点处理 (如 DeepSeek)
	baseURL := strings.TrimSuffix(acc.BaseURL, "/")

	versionPrefix := "/v1"
	if strings.Contains(baseURL, "generativelanguage.googleapis") {
		if strings.Contains(subPath, "preview") || strings.Contains(subPath, "3.1") || strings.Contains(subPath, "2.5") || strings.Contains(subPath, "2.0") || strings.Contains(subPath, "lite") {
			versionPrefix = "/v1beta"
		}
	}
	return baseURL + versionPrefix + subPath
}

// ParseToInt 安全地将字节切片解析为 int64
func ParseToInt(b []byte) int64 {
	var n int64
	if _, err := fmt.Sscanf(string(b), "%d", &n); err != nil {
		return 0
	}
	return n
}
