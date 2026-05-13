package utils

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"regexp"
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
	ResponseHeaderTimeout: 60 * time.Second, // 防止上游 hang 在 header 阶段
	ForceAttemptHTTP2:     true,
}

// Project Atlas: Polaris Gateway (OpenAI Protocol Module)
// Author: mrlaoliai

type ModelPrice struct {
	Prompt1M    float64
	Candidate1M float64
}

var modelPriceDict = map[string]ModelPrice{
	// ── DeepSeek 系列 ──────────────────────────────────────────
	"deepseek-chat":     {Prompt1M: 0.14, Candidate1M: 0.28},  // deepseek-v4-flash (non-thinking, alias)
	"deepseek-reasoner": {Prompt1M: 0.55, Candidate1M: 2.19},  // deepseek-r1
	"deepseek-v4-flash": {Prompt1M: 0.14, Candidate1M: 0.28},
	"deepseek-v4-pro":   {Prompt1M: 1.74, Candidate1M: 3.48},  // standard; 75% off til 2026/05/31 → $0.435/0.87

	// ── Anthropic Claude 系列 ────────────────────────────
	"claude-opus-4-7":             {Prompt1M: 5.0, Candidate1M: 25.0},
	"claude-opus-4-6":             {Prompt1M: 5.0, Candidate1M: 25.0},
	"claude-sonnet-4-6":           {Prompt1M: 3.0, Candidate1M: 15.0},
	"claude-sonnet-4-5":           {Prompt1M: 3.0, Candidate1M: 15.0},
	"claude-haiku-4-5":            {Prompt1M: 1.0, Candidate1M: 5.0},
	"claude-3-7-sonnet-20250219":  {Prompt1M: 3.0, Candidate1M: 15.0},
	"claude-3-5-sonnet-20241022":  {Prompt1M: 3.0, Candidate1M: 15.0},
	"claude-3-5-sonnet-20240620":  {Prompt1M: 3.0, Candidate1M: 15.0},
	"claude-3-5-haiku-20241022":   {Prompt1M: 0.80, Candidate1M: 4.0},
	"claude-3-opus-20240229":      {Prompt1M: 15.0, Candidate1M: 75.0},
	"claude-3-sonnet-20240229":    {Prompt1M: 3.0, Candidate1M: 15.0},
	"claude-3-haiku-20240307":     {Prompt1M: 0.25, Candidate1M: 1.25},

	// ── OpenAI GPT 系列 ──────────────────────────────────
	"gpt-5.5":             {Prompt1M: 5.0, Candidate1M: 30.0},
	"gpt-5.4":             {Prompt1M: 2.5, Candidate1M: 15.0},
	"gpt-5.4-mini":        {Prompt1M: 0.75, Candidate1M: 4.5},
	"gpt-4o":              {Prompt1M: 2.5, Candidate1M: 10.0},
	"gpt-4o-2024-05-13":   {Prompt1M: 5.0, Candidate1M: 15.0},
	"gpt-4o-mini":         {Prompt1M: 0.15, Candidate1M: 0.60},
	"o1":                  {Prompt1M: 15.0, Candidate1M: 60.0},
	"o1-preview":          {Prompt1M: 15.0, Candidate1M: 60.0},
	"o1-mini":             {Prompt1M: 1.1, Candidate1M: 4.4},
	"o3-mini":             {Prompt1M: 1.1, Candidate1M: 4.4},

	// ── Google Gemini — OpenAI 兼容协议 (google/ prefix) ────────
	"google/gemini-3.1-pro-preview-customtools": {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-3.1-pro-preview":             {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-3.1-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-3.1-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"google/gemini-3.1-ultra":                   {Prompt1M: 3.50, Candidate1M: 10.50},
	"google/gemini-3.0-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-3.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"google/gemini-3-flash-preview":             {Prompt1M: 0.10, Candidate1M: 0.40},
	"google/gemini-2.5-pro":                     {Prompt1M: 2.0, Candidate1M: 8.0},
	"google/gemini-2.5-flash":                   {Prompt1M: 0.075, Candidate1M: 0.30},
	"google/gemini-2.0-pro-exp":                 {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-2.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"google/gemini-1.5-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"google/gemini-1.5-flash":                   {Prompt1M: 0.075, Candidate1M: 0.30},
	"google/gemini-1.5-flash-8b":                {Prompt1M: 0.0375, Candidate1M: 0.15},

	// ── Google Gemini — Vertex 原生协议 (无 prefix) ─────────────
	"gemini-3.1-pro-preview-customtools": {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.1-pro-preview":             {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.1-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.1-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-3.1-ultra":                   {Prompt1M: 3.50, Candidate1M: 10.50},
	"gemini-3.0-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-3-flash-preview":             {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-2.5-pro":                     {Prompt1M: 2.0, Candidate1M: 8.0},
	"gemini-2.5-flash":                   {Prompt1M: 0.075, Candidate1M: 0.30},
	"gemini-2.0-pro-exp":                 {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-2.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-1.5-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-1.5-flash":                   {Prompt1M: 0.075, Candidate1M: 0.30},
	"gemini-1.5-flash-8b":                {Prompt1M: 0.0375, Candidate1M: 0.15},

	// ── 兜底基准 ────────────────────────────────────────────────
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

func EstimatePromptTokens(bodyBytes []byte) int64 {
	// 简单的经验法则: 1 token ≈ 4 字节
	return int64(len(bodyBytes)) / 4
}

func EstimateCompletionTokens(sentBytes int64) int64 {
	// SSE JSON overhead 较大，1 个 token 往往带有 50-80 字节的结构包装
	return sentBytes / 60
}

// CalculateCost 根据模型名和 token 用量计算费用
// 定价策略: 从 modelPriceDict 查找模型单价 → 区分 cached/uncached prompt tokens
// Gemini 模型超过 128K prompt tokens 时费率翻倍（长上下文定价）
// cached tokens 折扣: Gemini 25%, DeepSeek 10%, 其他 50%
func CalculateCost(provider, modelName string, promptTokens, candidateTokens, cachedTokens int64, bodyBytes []byte) float64 {
	price, exists := modelPriceDict[modelName]
	if !exists {
		price = modelPriceDict["default"]
	}

	promptRate := price.Prompt1M
	candidateRate := price.Candidate1M

	// 如果是通过 Vertex AI 渠道调用 Gemini，部分模型价格更贵（覆盖 AI Studio 价格）
	if provider == "google" {
		if strings.Contains(modelName, "gemini-2.0-flash") {
			promptRate = 0.15
			candidateRate = 0.60
		}
	}

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
	} else if strings.Contains(modelName, "claude-") {
		// Claude cached read tokens are typically 10% of standard rate
		cachedRate = promptRate * 0.10
	}

	cost := (float64(uncachedTokens)/1000000.0*promptRate) +
		(float64(cachedTokens)/1000000.0*cachedRate) +
		(float64(candidateTokens)/1000000.0*candidateRate)

	// 多模态补偿逻辑 (系数 1.05)
	if provider == "google" {
		hasMultimodal := bytes.Contains(bodyBytes, []byte(`"image_url"`)) ||
			bytes.Contains(bodyBytes, []byte(`"inlineData"`)) ||
			bytes.Contains(bodyBytes, []byte(`"inline_data"`)) ||
			bytes.Contains(bodyBytes, []byte(`"file_uri"`)) ||
			bytes.Contains(bodyBytes, []byte(`"fileUri"`))
		if hasMultimodal {
			cost *= 1.05
		}
	}

	return math.Ceil(cost*10000) / 10000
}

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