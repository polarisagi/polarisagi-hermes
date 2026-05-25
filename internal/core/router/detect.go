// 请求检测与统计：协议识别 + 模型提取 + 客户端识别 + 并发计数器
// 此文件扩展了原 utils.go 的能力，将分散在 webapi 和 translators/utils 中的检测逻辑集中到核心引擎
package router

import (
	"net/http"
	"strings"
)

var (
	// ActiveCount 正在处理的请求计数（原子操作），由 engine 层维护，webapi 读取
	ActiveCount int32
	// WaitingCount 排队等待的请求计数（原子操作），由 engine 层维护，webapi 读取
	WaitingCount int32
	// MaxConcurrency 最大并发数（=活跃节点数），启动时由 main 设置
	MaxConcurrency int
)

// IdentifyClient 从 User-Agent 请求头识别客户端类型，用于统计面板按客户端分组
func IdentifyClient(r *http.Request) string {
	userAgent := strings.ToLower(r.UserAgent())
	if strings.Contains(userAgent, "aider") {
		return "Aider"
	}
	if strings.Contains(userAgent, "curl") {
		return "cURL"
	}
	if strings.Contains(userAgent, "opencode") || strings.Contains(userAgent, "vscode") {
		return "OpenCode"
	}
	if userAgent == "" {
		return "Unknown"
	}
	if len(userAgent) > 20 {
		return userAgent[:20] + "..."
	}
	return r.UserAgent()
}

// ExtractMethodName 从 URL 路径中动态推导 OpenAPI 标准接口 (如 chat/completions, embeddings)
func ExtractMethodName(incomingPath string) string {
	sub := strings.TrimPrefix(incomingPath, "/v1/")
	sub = strings.TrimPrefix(sub, "/")
	if sub == "" {
		return "unknown"
	}
	return sub
}
