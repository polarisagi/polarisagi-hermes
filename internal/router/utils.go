// 路由工具函数：协议检测、URL 清洗、模型名提取
package router

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	// 从请求体 JSON 中提取模型名（OpenAI/Anthropic 格式均为 "model":"xxx"）
	modelRegexOpenAI = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
	// 从 Google 原生 URL 路径提取模型名
	// /v1/models/gemini-1.5-pro:generateContent        → gemini-1.5-pro
	modelRegexGoogleURL = regexp.MustCompile(`/models/([^:]+):`)
	// /v1/gemini-1.5-pro:streamGenerateContent（协议前缀已清除后）→ gemini-1.5-pro
	modelRegexGoogleGatewayURL = regexp.MustCompile(`^/v1/([^:]+):`)
)

// getIncomingProtocol 从 URL 路径的第一个路径段检测客户端使用的源协议
// 合法的源协议: anthropic | openai | google
// 旧路径 "vertex" 和 "gemini" 向后兼容，统一映射到 "google"
//
// 示例:
//
//	/v1/anthropic/messages         → "anthropic"
//	/v1/openai/chat/completions    → "openai"
//	/v1/google/models/...          → "google"
//	/v1/vertex/models/...          → "google" (旧路径，向后兼容)
//	/v1/gemini/models/...          → "google" (旧路径，向后兼容)
//	/v1/chat/completions (旧格式)  → 自动识别为 "openai"
func getIncomingProtocol(path string) string {
	parts := strings.SplitN(path, "/", 4)
	if len(parts) >= 3 && strings.HasPrefix(parts[1], "v") {
		segment := parts[2]
		switch segment {
		case "openai":
			return "openai"
		case "anthropic":
			return "anthropic"
		case "google", "vertex", "gemini": // "vertex"/"gemini" 向后兼容旧客户端配置
			return "google"
		}
	}

	// 旧式路径：从路径内容自动推断（向后兼容）
	if strings.Contains(path, "chat/completions") || strings.Contains(path, "embeddings") {
		return "openai"
	}
	if strings.Contains(path, "messages") {
		return "anthropic"
	}
	if strings.Contains(path, "generateContent") || strings.Contains(path, "streamGenerateContent") || strings.Contains(path, "publishers/google/") {
		return "google"
	}
	return "unknown"
}

// stripProtocolPrefix 移除 URL 路径中的协议前缀段
// 转换后下游转换器接收的是统一格式的干净路径
// 例如: /v1/google/models/gemini-1.5-pro:generateContent → /v1/models/gemini-1.5-pro:generateContent
//      /v1/vertex/models/gemini-1.5-pro:generateContent → /v1/models/gemini-1.5-pro:generateContent（向后兼容）
// 支持 /v1/, /v1beta/, /v1alpha/ 等版本前缀
func stripProtocolPrefix(path string) string {
	parts := strings.SplitN(path, "/", 4)
	if len(parts) >= 4 && strings.HasPrefix(parts[1], "v") {
		protocol := parts[2]
		if protocol == "openai" || protocol == "anthropic" || protocol == "google" || protocol == "vertex" || protocol == "gemini" {
			return "/" + parts[1] + "/" + parts[3]
		}
	}
	return path
}

// extractModelName 从请求体中提取模型名
// openai/anthropic: 从 body JSON 的 "model" 字段提取
// google: 先尝试从 body JSON 的 "model" 字段提取，失败返回占位符 "_google_native_"
//         (后续由调用方从 URL 路径再次提取)
func extractModelName(body []byte, protocol string) string {
	switch protocol {
	case "openai", "anthropic":
		match := modelRegexOpenAI.FindSubmatch(body)
		if len(match) > 1 {
			return string(match[1])
		}
	case "google":
		var req struct {
			Model string `json:"model"`
			Name  string `json:"name"`
		}
		if json.Unmarshal(body, &req) == nil {
			if req.Model != "" {
				return strings.TrimPrefix(req.Model, "models/")
			}
			if req.Name != "" {
				return strings.TrimPrefix(req.Name, "models/")
			}
		}
		return "_google_native_"
	}
	return ""
}

// extractModelFromGooglePath 从 Google Agent Platform 原生 URL 路径中提取模型名
// 支持两种路径格式:
//
//	/v1/models/gemini-1.5-pro:generateContent         → gemini-1.5-pro
//	/v1/gemini-1.5-pro:streamGenerateContent          → gemini-1.5-pro (协议前缀已清除后)
func extractModelFromGooglePath(path string) string {
	match := modelRegexGoogleURL.FindStringSubmatch(path)
	if len(match) > 1 {
		return match[1]
	}
	match = modelRegexGoogleGatewayURL.FindStringSubmatch(path)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// ReplaceGoogleModelInPath 替换 Google 原生 URL 路径中的模型名
func ReplaceGoogleModelInPath(path, targetModel string) string {
	if match := modelRegexGoogleURL.FindStringSubmatch(path); len(match) > 1 {
		return strings.Replace(path, match[1], targetModel, 1)
	}
	if match := modelRegexGoogleGatewayURL.FindStringSubmatch(path); len(match) > 1 {
		return strings.Replace(path, match[1], targetModel, 1)
	}
	return path
}
