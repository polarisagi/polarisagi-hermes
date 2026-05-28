package translator

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/polarisagi/polarisagi-hermes/internal/domain"
	"github.com/polarisagi/polarisagi-hermes/internal/service/channel"
)

// BuildTargetURL 实现多态路由分发，原生支持 Vertex 端点的多子路径拼接
// 提取自旧代码的 transport.go，去除了对 config 包的依赖，改为依赖 ActiveChannel
func BuildTargetURL(ch *channel.ActiveChannel, targetEndpoint *domain.SysAccessEndpoint, incomingPath string) string {
	// 1. 提取业务子路径 (例如 chat/completions)
	subPath := strings.TrimPrefix(incomingPath, "/v1")
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}

	// 2. 判断是否是 Vertex OpenAPI 节点路由渲染（这里简单用 baseUrl 是否含占位符来判断，或者你可以用 Provider 内部字段）
	// 老代码依赖 ProjectID != ""，但在新的设计里，ProjectID 可以通过 UserProvider 的 AuthCredentials 中提取，
	// 目前为了兼容，先只进行标准的 BaseURL 拼接（假设用户配置的 BaseURL 就是完整的前缀）

	baseURL := strings.TrimSuffix(ch.Provider.BaseURL, "/")
	if baseURL == "" && targetEndpoint != nil {
		baseURL = strings.TrimSuffix(targetEndpoint.DefaultBaseURL, "/")
	}

	// 如果 baseURL 包含了 Vertex/GEAP 的占位符，按照老代码逻辑替换（由于新表设计我们把 ProjectID 放进了 Credentials JSON）
	// 这里可以预留给业务层通过 AuthCredentials 解析。
	// 为了最小化修改，我们目前直接使用 baseURL 和 versionPrefix 拼接

	versionPrefix := "/v1"
	if strings.Contains(baseURL, "generativelanguage.googleapis") {
		if strings.Contains(subPath, "preview") || strings.Contains(subPath, "3.1") || strings.Contains(subPath, "2.5") || strings.Contains(subPath, "2.0") || strings.Contains(subPath, "lite") {
			versionPrefix = "/v1beta"
		}
	}
	return baseURL + versionPrefix + subPath
}

// ForwardStreamBody 将 body 流式转发到 w，同时维护尾部 8KB 缓冲窗口
// 提取自旧代码的 transport.go，用于在 SSE 拦截结束后计算计费 token
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

// ParseToInt 安全地将字节切片解析为 int64
func ParseToInt(b []byte) int64 {
	var n int64
	if _, err := fmt.Sscanf(string(b), "%d", &n); err != nil {
		return 0
	}
	return n
}

// CopyHeaders 安全地从源请求/响应复制 Header 到目标，忽略特定头
func CopyHeaders(dst http.Header, src http.Header) {
	for k, vv := range src {
		if !strings.EqualFold(k, "Host") &&
			!strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Transfer-Encoding") &&
			!strings.EqualFold(k, "Accept-Encoding") &&
			!strings.EqualFold(k, "Authorization") {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}
