package router

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	modelRegexOpenAI    = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
	modelRegexAnthropic = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
	modelRegexVertexURL = regexp.MustCompile(`/models/([^:]+):`)
)

// getIncomingProtocol detects the protocol from the first path segment after /v1/
// URL: /v1/openai/chat/completions → "openai"
// URL: /v1/anthropic/messages → "anthropic"
// URL: /v1/vertex/models/gemini-1.5-pro:generateContent → "vertex"
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

// stripProtocolPrefix removes the protocol segment from the URL path
// /v1/openai/chat/completions → /v1/chat/completions
func stripProtocolPrefix(path string) string {
	trimmed := strings.TrimPrefix(path, "/v1/")
	idx := strings.Index(trimmed, "/")
	if idx > 0 {
		return "/v1/" + trimmed[idx+1:]
	}
	return path
}

// extractModelName extracts the model name from the request body or URL path
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

func extractModelFromVertexPath(path string) string {
	match := modelRegexVertexURL.FindStringSubmatch(path)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}
