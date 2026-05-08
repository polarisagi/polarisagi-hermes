package translators

import (
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
	openAIPromptRegex     = regexp.MustCompile(`"prompt_tokens"\s*:\s*(\d+)`)
	openAICompletionRegex = regexp.MustCompile(`"completion_tokens"\s*:\s*(\d+)`)
	openAICachedRegex     = regexp.MustCompile(`"cached_tokens"\s*:\s*(\d+)`)
	modelRegex            = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
	
	promptRegex        = regexp.MustCompile(`"promptTokenCount":\s*(\d+)`)
	candidateRegex     = regexp.MustCompile(`"candidatesTokenCount":\s*(\d+)`)
	cachedContentRegex = regexp.MustCompile(`"cachedContentTokenCount":\s*(\d+)`)
)

func extractModelName(body []byte) string {
	match := modelRegex.FindSubmatch(body)
	if len(match) > 1 {
		return string(match[1])
	}
	return "unknown"
}

// extractMethodName 从 URL 路径中动态推导 OpenAPI 标准接口 (如 chat/completions, embeddings)
func extractMethodName(incomingPath string) string {
	sub := strings.TrimPrefix(incomingPath, "/v1/")
	sub = strings.TrimPrefix(sub, "/")
	if sub == "" {
		return "unknown"
	}
	return sub
}

func calculateCost(modelName string, promptTokens, candidateTokens, cachedTokens int64) float64 {
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

func identifyClient(r *http.Request) string {
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

// buildTargetURL 实现多态路由分发，原生支持 Vertex 端点的多子路径拼接
func buildTargetURL(acc config.AccountDetail, incomingPath string) string {
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

func parseToInt(b []byte) int64 {
	var n int64
	if _, err := fmt.Sscanf(string(b), "%d", &n); err != nil {
		return 0
	}
	return n
}
