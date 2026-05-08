// 路由工具函数：协议检测、URL 清洗、模型名提取
package router

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	// 从请求体 JSON 中提取模型名的正则表达式（OpenAI/Anthropic 格式）
	modelRegexOpenAI    = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
	modelRegexAnthropic = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
	// 从 Vertex URL 路径提取模型名的正则
	// /v1/models/gemini-1.5-pro:generateContent → gemini-1.5-pro
	modelRegexVertexURL        = regexp.MustCompile(`/models/([^:]+):`)
	// /v1/gemini-1.5-pro:streamGenerateContent → gemini-1.5-pro (清除协议前缀后的路径)
	modelRegexVertexGatewayURL = regexp.MustCompile(`^/v1/([^:]+):`)
)

// getIncomingProtocol 从 URL 路径的第一个路径段检测客户端使用的协议
//
// 示例:
//
//	/v1/openai/chat/completions    → "openai"
//	/v1/anthropic/messages         → "anthropic"
//	/v1/vertex/models/...          → "vertex"
//	/v1/chat/completions (旧格式)  → 自动识别为 "openai"
func getIncomingProtocol(path string) string {
	trimmed := strings.TrimPrefix(path, "/v1/")
	idx := strings.Index(trimmed, "/")
	segment := trimmed
	if idx > 0 {
		segment = trimmed[:idx]
	}

	switch segment {
	case "openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	case "vertex":
		return "vertex"
	case "gemini":
		return "gemini"
	default:
		// Legacy fallback: auto-detect from path content (backward compatibility)
		if strings.Contains(path, "chat/completions") || strings.Contains(path, "embeddings") {
			return "openai"
		}
		if strings.Contains(path, "messages") {
			return "anthropic"
		}
		if strings.Contains(path, "generateContent") || strings.Contains(path, "streamGenerateContent") {
			return "vertex"
		}
		return "unknown"
	}
}

// stripProtocolPrefix 移除 URL 路径中的协议前缀段
// 转换后下游转换器接收的是统一格式的干净路径
// 例如: /v1/vertex/models/gemini-1.5-pro:generateContent → /v1/models/gemini-1.5-pro:generateContent
func stripProtocolPrefix(path string) string {
	trimmed := strings.TrimPrefix(path, "/v1/")
	idx := strings.Index(trimmed, "/")
	if idx > 0 {
		return "/v1/" + trimmed[idx+1:]
	}
	return path
}

// extractModelName 从请求体中提取模型名
// OpenAI/Anthropic: 从 body JSON 的 "model" 字段提取
// Vertex: 先尝试从 body JSON 的 "model" 字段提取，失败返回占位符 "_vertex_native_"
//         (后续会从 URL 路径再次提取)
func extractModelName(body []byte, protocol string) string {
	if protocol == "openai" || protocol == "gemini" {
		match := modelRegexOpenAI.FindSubmatch(body)
		if len(match) > 1 {
			return string(match[1])
		}
	} else if protocol == "anthropic" {
		match := modelRegexAnthropic.FindSubmatch(body)
		if len(match) > 1 {
			return string(match[1])
		}
	} else if protocol == "vertex" {
		var vertexReq struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &vertexReq) == nil && vertexReq.Model != "" {
			return vertexReq.Model
		}
		return "_vertex_native_"
	}
	return ""
}

// extractModelFromVertexPath 从 Vertex 原生的 URL 路径中提取模型名
// 支持两种路径格式:
//
//	/v1/models/gemini-1.5-pro:generateContent         → gemini-1.5-pro
//	/v1/gemini-1.5-pro:streamGenerateContent          → gemini-1.5-pro (协议前缀已清除后)
func extractModelFromVertexPath(path string) string {
	match := modelRegexVertexURL.FindStringSubmatch(path)
	if len(match) > 1 {
		return match[1]
	}
	match = modelRegexVertexGatewayURL.FindStringSubmatch(path)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}
