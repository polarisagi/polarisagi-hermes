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

func getIncomingProtocol(path string) string {
	// Vertex native: path contains generateContent, streamGenerateContent, or publishers/google/models
	if strings.Contains(path, "generateContent") || strings.Contains(path, "streamGenerateContent") || strings.Contains(path, "publishers/google/models") {
		return "vertex"
	}
	// OpenAI: chat/completions, embeddings, or plain models listing
	if strings.Contains(path, "chat/completions") || strings.Contains(path, "embeddings") {
		return "openai"
	}
	// Anthropic: messages endpoint
	if strings.Contains(path, "messages") {
		return "anthropic"
	}
	// Models endpoint (non-Vertex): OpenAI-style models listing
	if strings.Contains(path, "models") {
		return "openai"
	}
	return "unknown"
}

// extractModelName extracts the model name from the request body or URL path
func extractModelName(body []byte, protocol string) string {
	if protocol == "openai" {
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
		// Try body first (some Vertex clients send model in body)
		var vertexReq struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &vertexReq) == nil && vertexReq.Model != "" {
			return vertexReq.Model
		}
		// Fallback: use empty string — model will be extracted from URL path at routing time
		// The route's model_mappings will handle this via wildcard match
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
