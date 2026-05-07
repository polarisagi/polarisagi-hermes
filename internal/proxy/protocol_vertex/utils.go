package protocol_vertex

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"strings"

	"polaris-gateway/internal/config"
)

// Project Atlas: Polaris Gateway (Vertex Native Protocol Module)
// Author: mrlaoliai

const (
	PricePer1MPrompt    = 1.25
	PricePer1MCandidate = 3.75
)

type ModelPrice struct {
	Prompt1M    float64
	Candidate1M float64
}

var vertexPriceDict = map[string]ModelPrice{
	"gemini-3.1-pro-preview-customtools": {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.1-pro-preview":             {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.1-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.1-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-3.1-ultra":                   {Prompt1M: 3.50, Candidate1M: 10.50},
	"gemini-3.0-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-3-flash-preview":             {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-2.5-flash":                   {Prompt1M: 0.075, Candidate1M: 0.30},
	"gemini-2.0-pro-exp":                 {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-2.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"default":                            {Prompt1M: PricePer1MPrompt, Candidate1M: PricePer1MCandidate},
}

func extractModelName(targetURL string) string {
	modelName := "default"
	if idx := strings.Index(targetURL, "/models/"); idx != -1 {
		sub := targetURL[idx+8:]
		if colonIdx := strings.Index(sub, ":"); colonIdx != -1 {
			modelName = sub[:colonIdx]
		} else {
			modelName = sub
		}
	}
	return modelName
}

func extractMethodName(targetURL string) string {
	if idx := strings.LastIndex(targetURL, ":"); idx != -1 {
		return targetURL[idx+1:]
	}
	return "unknown"
}

func calculateCost(modelName string, promptTokens, candidateTokens, cachedTokens int64) float64 {
	price, exists := vertexPriceDict[modelName]
	if !exists {
		price = vertexPriceDict["default"]
	}

	promptRate := price.Prompt1M
	candidateRate := price.Candidate1M

	if strings.HasPrefix(modelName, "gemini-") && promptTokens > 128000 {
		promptRate *= 2.0
		candidateRate *= 2.0
	}

	uncachedTokens := promptTokens - cachedTokens
	if uncachedTokens < 0 {
		uncachedTokens = 0
	}

	// Gemini Cached Context discount is ~25% of standard rate
	cachedRate := promptRate * 0.25

	cost := (float64(uncachedTokens)/1000000.0*promptRate) +
		(float64(cachedTokens)/1000000.0*cachedRate) +
		(float64(candidateTokens)/1000000.0*candidateRate)
	return math.Ceil(cost*10000) / 10000
}

var (
	promptRegex        = regexp.MustCompile(`"promptTokenCount":\s*(\d+)`)
	candidateRegex     = regexp.MustCompile(`"candidatesTokenCount":\s*(\d+)`)
	cachedContentRegex = regexp.MustCompile(`"cachedContentTokenCount":\s*(\d+)`)
)

func identifyClient(r *http.Request) string {
	userAgent := r.UserAgent()
	lowerUA := strings.ToLower(userAgent)

	if strings.Contains(lowerUA, "aider") {
		return "Aider"
	}
	if strings.Contains(lowerUA, "curl") {
		return "cURL"
	}
	if strings.Contains(lowerUA, "opencode") || strings.Contains(lowerUA, "vscode") {
		return "OpenCode"
	}
	if strings.Contains(lowerUA, "litellm") {
		return "Aider(LiteLLM)"
	}

	if userAgent == "" {
		return "Unknown"
	}
	if len(userAgent) > 20 {
		return userAgent[:20] + "..."
	}
	return userAgent
}

// buildTargetURL: 支持 BaseURL 与 ProjectID 注入的 Vertex 原生端点路由引擎
func buildTargetURL(acc config.AccountDetail, incomingPath string) string {
	// 缺失模型名自愈防御
	if strings.Contains(incomingPath, "models/:") {
		incomingPath = strings.Replace(incomingPath, "models/:", "models/gemini-3.1-pro-preview-customtools:", 1)
		log.Printf("🛠️ [Vertex 网关介入] 探测到上游丢失模型名，已自动补全为 3.1-pro-preview")
	}

	// 提取核心子路径 (例如 models/gemini-3.1-pro:generateContent)
	subPath := incomingPath
	if idx := strings.Index(incomingPath, "models/"); idx != -1 {
		subPath = incomingPath[idx:]
	} else {
		subPath = strings.TrimPrefix(incomingPath, "/")
	}

	location := acc.Location
	if location == "" {
		location = "global" // 默认退化为 global 区域
	}

	// 1. 如果用户定义了完整的 BaseURL，使用模板替换逻辑
	if acc.BaseURL != "" {
		template := acc.BaseURL
		resURL := strings.ReplaceAll(template, "{project_id}", acc.ProjectID)
		resURL = strings.ReplaceAll(resURL, "{location}", location)

		baseURL := strings.TrimSuffix(resURL, "/")
		if !strings.Contains(baseURL, "publishers/google") && strings.HasPrefix(subPath, "models/") {
			return baseURL + "/publishers/google/" + subPath
		}
		return baseURL + "/" + subPath
	}

	// 2. 如果配置了 ProjectID，使用官方推荐的企业级全路径格式 (你所要求的格式)
	if acc.ProjectID != "" {
		if strings.HasPrefix(subPath, "models/") {
			return fmt.Sprintf("https://aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/%s", acc.ProjectID, location, subPath)
		}
		return fmt.Sprintf("https://aiplatform.googleapis.com/v1/projects/%s/locations/%s/%s", acc.ProjectID, location, subPath)
	}

	// 3. 默认回退至不带 ProjectID 的 Legacy 格式 (作为兜底)
	if strings.HasPrefix(subPath, "models/") {
		return "https://aiplatform.googleapis.com/v1/publishers/google/" + subPath
	}
	return "https://aiplatform.googleapis.com/v1/" + subPath
}

func parseToInt(b []byte) int64 {
	var n int64
	if _, err := fmt.Sscanf(string(b), "%d", &n); err != nil {
		return 0
	}
	return n
}
