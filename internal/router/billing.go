// 统一计费引擎：模型定价 + 费用计算 + Token 估算 + 用量解析 + 结算入口
// 所有协议转换器在处理完响应后调用 SettleBilling() 完成计费结算
// 禁止在协议层直接调用 db.SaveUsage 或 Node.RecordCost
package router

import (
	"bytes"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"
	"polaris-gateway/internal/db"
)

// ── tiktoken 高精度分词器 ──
// 使用 OpenAI o200k_base 分词器提供接近真实的 token 计数
// 该实例全局共享，线程安全，启动时异步预加载

var (
	tke     *tiktoken.Tiktoken
	tkeOnce sync.Once
)

func init() {
	// 异步预加载 tiktoken 字典，避免首次请求时的加载延迟
	go GetTiktoken()
}

// GetTiktoken 返回全局共享的 tiktoken 实例（惰性初始化，线程安全）
func GetTiktoken() *tiktoken.Tiktoken {
	tkeOnce.Do(func() {
		var err error
		// o200k_base 是 OpenAI 最新分词器，对 CJK 字符密度更接近 Claude/Gemini 实际分词
		tke, err = tiktoken.GetEncoding("o200k_base")
		if err != nil {
			slog.Error("⚠️ [Tiktoken] 初始化失败，token 估算将使用字节数兜底", "error", err)
		} else {
			slog.Debug("✅ [Tiktoken] o200k_base 分词器初始化完成")
		}
	})
	return tke
}

// CountTextTokens 使用 tiktoken 精确计算文本的 token 数
// 这是全局公共的高精度分词函数，供 count_tokens 端点和计费估算共用
func CountTextTokens(text string) int {
	tk := GetTiktoken()
	if tk != nil {
		return len(tk.Encode(text, nil, nil))
	}
	// tiktoken 初始化失败时的字节数兜底（1 token ≈ 4 bytes）
	return len(text) / 4
}

// ModelPrice 模型单价（美元/百万 token）
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

// ── 用量提取正则 ──

var (
	OpenAIPromptRegex     = regexp.MustCompile(`"prompt_tokens"\s*:\s*(\d+)`)
	OpenAICompletionRegex = regexp.MustCompile(`"completion_tokens"\s*:\s*(\d+)`)
	OpenAICachedRegex     = regexp.MustCompile(`"cached_tokens"\s*:\s*(\d+)`)
	BillingModelRegex     = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)

	PromptRegex        = regexp.MustCompile(`"promptTokenCount":\s*(\d+)`)
	CandidateRegex     = regexp.MustCompile(`"candidatesTokenCount":\s*(\d+)`)
	CachedContentRegex = regexp.MustCompile(`"cachedContentTokenCount":\s*(\d+)`)
)

// ExtractModelNameFromBody 从 JSON body 中用正则快速提取 model 字段值（计费用途）
func ExtractModelNameFromBody(body []byte) string {
	match := BillingModelRegex.FindSubmatch(body)
	if len(match) > 1 {
		return string(match[1])
	}
	return "unknown"
}

// EstimatePromptTokens 使用 tiktoken 精确估算请求体中的 prompt token 数
// 当 SSE 流中断未能获取上游返回的 usageMetadata 时，此函数作为兜底估算
func EstimatePromptTokens(bodyBytes []byte) int64 {
	tk := GetTiktoken()
	if tk != nil {
		return int64(len(tk.Encode(string(bodyBytes), nil, nil)))
	}
	// tiktoken 不可用时的字节数兜底（1 token ≈ 4 bytes）
	return int64(len(bodyBytes)) / 4
}

// EstimateCompletionTokens 基于 SSE 流字节数的启发式 token 估算（1 token ≈ 60 字节 SSE 开销）
// SSE 格式包含大量 data: 前缀和 JSON 结构包装，因此比例远高于纯文本
func EstimateCompletionTokens(sentBytes int64) int64 {
	return sentBytes / 60
}

// CalculateCost 根据模型名和 token 用量计算费用
// 定价策略: 从 modelPriceDict 查找模型单价 → 区分 cached/uncached prompt tokens
// Gemini 模型超过 128K prompt tokens 时费率翻倍（长上下文定价）
// cached tokens 折扣: Gemini 25%, DeepSeek 10%, Claude 10%, 其他 50%
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
		cachedRate = promptRate * 0.10
	} else if strings.Contains(modelName, "gemini-") {
		cachedRate = promptRate * 0.25
	} else if strings.Contains(modelName, "claude-") {
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

// ParseUsageFromStreamTail 从 SSE 流尾部缓冲中提取 token 用量
// 支持 OpenAI 格式 (prompt_tokens/completion_tokens) 和 Vertex 格式 (promptTokenCount/candidatesTokenCount)
func ParseUsageFromStreamTail(tailBuf []byte) (prompt, completion, cached int64, found bool) {
	// Try OpenAI format
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

	// Try Vertex format
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

// SettleBilling 全局统一计费入口
// 所有协议转换器在处理完响应后调用此函数完成计费结算
// 禁止在协议层直接调用 db.SaveUsage 或 Node.RecordCost
func SettleBilling(provider, nodeName, clientType, methodName, modelName string,
	promptTokens, completionTokens, cachedTokens int64,
	statusCode int, dest *MatchedDestination, reqBody []byte, traceID string) {
	if promptTokens <= 0 && completionTokens <= 0 {
		return
	}
	cost := CalculateCost(provider, modelName, promptTokens, completionTokens, cachedTokens, reqBody)
	db.SaveUsage(provider, nodeName, clientType, methodName, promptTokens, completionTokens, cost, statusCode)
	dest.Node.RecordCost(cost, traceID)
	if cachedTokens > 0 {
		slog.Info("💰 结算完成", "trace_id", traceID, "account", nodeName, "model", modelName, "prompt", promptTokens, "cached", cachedTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))
	} else {
		slog.Info("💰 结算完成", "trace_id", traceID, "account", nodeName, "model", modelName, "prompt", promptTokens, "completion", completionTokens, "cost", fmt.Sprintf("%.4f", cost))
	}
}
