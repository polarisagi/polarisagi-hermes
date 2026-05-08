package utils

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"

	"polaris-gateway/internal/config"
)

// Project Atlas: Polaris Gateway (OpenAI Protocol Module)
// Author: mrlaoliai

type ModelPrice struct {
	Prompt1M    float64
	Candidate1M float64
}

var openAIPriceDict = map[string]ModelPrice{
	// DeepSeek 系列
	"deepseek-chat":  {Prompt1M: 0.14, Candidate1M: 0.28},
	"deepseek-coder": {Prompt1M: 0.14, Candidate1M: 0.28},
	// Google Gemini 3.1 系列
	"google/gemini-3.1-pro-preview-customtools": {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-3.1-pro-preview":             {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-3.1-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-3.1-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"google/gemini-3.1-ultra":                   {Prompt1M: 3.50, Candidate1M: 10.50},
	"google/gemini-3.0-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-3.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"google/gemini-3-flash-preview":             {Prompt1M: 0.10, Candidate1M: 0.40},
	"google/gemini-2.5-flash":                   {Prompt1M: 0.075, Candidate1M: 0.30},
	"google/gemini-2.0-pro-exp":                 {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-2.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	// 兜底基准
	"default": {Prompt1M: 1.0, Candidate1M: 2.0},
}

var (
	OpenAIPromptRegex     = regexp.MustCompile(`"prompt_tokens"\s*:\s*(\d+)`)
	OpenAICompletionRegex = regexp.MustCompile(`"completion_tokens"\s*:\s*(\d+)`)
	OpenAICachedRegex     = regexp.MustCompile(`"cached_tokens"\s*:\s*(\d+)`)
	ModelRegex            = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
	
	PromptRegex        = regexp.MustCompile(`"promptTokenCount":\s*(\d+)`)
	CandidateRegex     = regexp.MustCompile(`"candidatesTokenCount":\s*(\d+)`)
	CachedContentRegex = regexp.MustCompile(`"cachedContentTokenCount":\s*(\d+)`)
)

func ExtractModelName(body []byte) string {
	match := ModelRegex.FindSubmatch(body)
	if len(match) > 1 {
		return string(match[1])
	}
	return "unknown"
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

func CalculateCost(modelName string, promptTokens, candidateTokens, cachedTokens int64) float64 {
	price, exists := openAIPriceDict[modelName]
	if !exists {
		price = openAIPriceDict["default"]
	}

	promptRate := price.Prompt1M
	candidateRate := price.Candidate1M

	if strings.Contains(modelName, "gemini-") && promptTokens > 128000 {
		promptRate *= 2.0
		candidateRate *= 2.0
	}

	uncachedTokens := promptTokens - cachedTokens
	if uncachedTokens < 0 {
		uncachedTokens = 0
	}

	cachedRate := promptRate * 0.50 // Default 50% discount for cached tokens

	if strings.Contains(modelName, "deepseek-") {
		// DeepSeek cached tokens are typically 10% of standard rate
		cachedRate = promptRate * 0.10
	} else if strings.Contains(modelName, "gemini-") {
		// Gemini cached context discount is ~25% of standard rate
		cachedRate = promptRate * 0.25
	}

	cost := (float64(uncachedTokens)/1000000.0*promptRate) +
		(float64(cachedTokens)/1000000.0*cachedRate) +
		(float64(candidateTokens)/1000000.0*candidateRate)
	return math.Ceil(cost*10000) / 10000
}

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
	return baseURL + "/v1" + subPath
}

func ParseToInt(b []byte) int64 {
	var n int64
	if _, err := fmt.Sscanf(string(b), "%d", &n); err != nil {
		return 0
	}
	return n
}

// ParseUsageFromStreamTail parses OpenAI, Vertex, and Gemini stream response tails for token usage info.
func ParseUsageFromStreamTail(tailBuf []byte) (prompt, completion, cached int64, found bool) {
	// Try OpenAI format: "prompt_tokens", "completion_tokens"
	if bytes.Contains(tailBuf, []byte("prompt_tokens")) || bytes.Contains(tailBuf, []byte("completion_tokens")) {
		pMatch := OpenAIPromptRegex.FindSubmatch(tailBuf)
		cMatch := OpenAICompletionRegex.FindSubmatch(tailBuf)
		cacheMatch := OpenAICachedRegex.FindSubmatch(tailBuf)
		if len(pMatch) > 1 {
			prompt = ParseToInt(pMatch[1])
		}
		if len(cMatch) > 1 {
			completion = ParseToInt(cMatch[1])
		}
		if len(cacheMatch) > 1 {
			cached = ParseToInt(cacheMatch[1])
		}
		if prompt > 0 || completion > 0 {
			return prompt, completion, cached, true
		}
	}

	// Try Vertex format: "promptTokenCount", "candidatesTokenCount"
	if bytes.Contains(tailBuf, []byte("promptTokenCount")) || bytes.Contains(tailBuf, []byte("usageMetadata")) {
		pMatch := PromptRegex.FindSubmatch(tailBuf)
		cMatch := CandidateRegex.FindSubmatch(tailBuf)
		cacheMatch := CachedContentRegex.FindSubmatch(tailBuf)
		if len(pMatch) > 1 {
			prompt = ParseToInt(pMatch[1])
		}
		if len(cMatch) > 1 {
			completion = ParseToInt(cMatch[1])
		}
		if len(cacheMatch) > 1 {
			cached = ParseToInt(cacheMatch[1])
		}
		if prompt > 0 || completion > 0 {
			return prompt, completion, cached, true
		}
	}

	return 0, 0, 0, false
}