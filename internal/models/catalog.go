// Package models 提供各协议的模型目录，用于管理后台模型选择下拉框
// 模型列表按协议分组，包含模型名称、描述信息
package models

// ModelInfo 单个模型的元信息
type ModelInfo struct {
	Name        string `json:"name"`        // 模型标识名（用于路由匹配）
	DisplayName string `json:"display_name"` // 展示名称
	Protocol    string `json:"protocol"`    // 所属协议: openai, anthropic, google
	Category    string `json:"category"`    // 分类: flagship, reasoning, cost-efficient, vision, legacy
}

// GetModelsByProtocol 根据协议返回可用的模型列表
// 支持: openai, anthropic, google（Google Agent Platform）
// "gemini" 作为 "google" 的别名保持向后兼容（旧客户端/数据库迁移前的兼容层）
func GetModelsByProtocol(protocol string) []ModelInfo {
	if protocol == "gemini" {
		protocol = "google"
	}
	models, ok := modelCatalog[protocol]
	if !ok {
		return nil
	}
	return models
}

// GetAllProtocols 返回所有已支持的协议列表
func GetAllProtocols() []string {
	protocols := make([]string, 0, len(modelCatalog))
	for k := range modelCatalog {
		protocols = append(protocols, k)
	}
	return protocols
}

// modelCatalog 各协议模型目录，key 为协议名
// 占位符 "*" 表示匹配所有模型
var modelCatalog = map[string][]ModelInfo{
	"openai": {
		// 通配符 / catch-all
		{Name: "*", DisplayName: "全部 OpenAI 模型 (通配符)", Protocol: "openai", Category: "wildcard"},

		// GPT-5 系列
		{Name: "gpt-5.5", DisplayName: "GPT-5.5 — 最强旗舰", Protocol: "openai", Category: "flagship"},
		{Name: "gpt-5.4", DisplayName: "GPT-5.4 — 平衡性能", Protocol: "openai", Category: "flagship"},
		{Name: "gpt-5.4-mini", DisplayName: "GPT-5.4 Mini — 快速轻量", Protocol: "openai", Category: "cost-efficient"},

		// GPT-4.1 系列
		{Name: "gpt-4.1", DisplayName: "GPT-4.1 — 通用旗舰", Protocol: "openai", Category: "flagship"},
		{Name: "gpt-4.1-mini", DisplayName: "GPT-4.1 Mini — 轻量版", Protocol: "openai", Category: "cost-efficient"},
		{Name: "gpt-4.1-nano", DisplayName: "GPT-4.1 Nano — 最轻量", Protocol: "openai", Category: "cost-efficient"},

		// O 系列推理模型
		{Name: "o3", DisplayName: "o3 — 深度推理", Protocol: "openai", Category: "reasoning"},
		{Name: "o4-mini", DisplayName: "o4-mini — 轻量推理", Protocol: "openai", Category: "reasoning"},
		{Name: "o1", DisplayName: "o1 — 高级推理", Protocol: "openai", Category: "reasoning"},
		{Name: "o1-mini", DisplayName: "o1-mini — 快速推理", Protocol: "openai", Category: "reasoning"},
		{Name: "o1-pro", DisplayName: "o1-pro — 专业推理", Protocol: "openai", Category: "reasoning"},
		{Name: "o3-mini", DisplayName: "o3-mini — 迷你推理", Protocol: "openai", Category: "reasoning"},

		// GPT-4o 系列
		{Name: "gpt-4o", DisplayName: "GPT-4o — 多模态旗舰", Protocol: "openai", Category: "flagship"},
		{Name: "gpt-4o-mini", DisplayName: "GPT-4o Mini — 经济多模态", Protocol: "openai", Category: "cost-efficient"},

		// GPT-4 系列
		{Name: "gpt-4-turbo", DisplayName: "GPT-4 Turbo — 快速版", Protocol: "openai", Category: "legacy"},
		{Name: "gpt-4", DisplayName: "GPT-4 — 经典标准", Protocol: "openai", Category: "legacy"},
		{Name: "gpt-4-32k", DisplayName: "GPT-4 32K — 长上下文", Protocol: "openai", Category: "legacy"},

		// GPT-3.5 系列
		{Name: "gpt-3.5-turbo", DisplayName: "GPT-3.5 Turbo — 经典性价比", Protocol: "openai", Category: "legacy"},
		{Name: "gpt-3.5-turbo-16k", DisplayName: "GPT-3.5 Turbo 16K — 长上下文", Protocol: "openai", Category: "legacy"},
	},

	"anthropic": {
		// 通配符
		{Name: "*", DisplayName: "全部 Anthropic 模型 (通配符)", Protocol: "anthropic", Category: "wildcard"},

		// Claude 4 系列
		{Name: "claude-opus-4-7", DisplayName: "Claude Opus 4.7 — 最强推理", Protocol: "anthropic", Category: "flagship"},
		{Name: "claude-opus-4-6", DisplayName: "Claude Opus 4.6", Protocol: "anthropic", Category: "flagship"},
		{Name: "claude-sonnet-4-6", DisplayName: "Claude Sonnet 4.6", Protocol: "anthropic", Category: "flagship"},
		{Name: "claude-sonnet-4-5", DisplayName: "Claude Sonnet 4.5 — 平衡版", Protocol: "anthropic", Category: "flagship"},
		{Name: "claude-haiku-4-5", DisplayName: "Claude Haiku 4.5 — 极速轻量", Protocol: "anthropic", Category: "cost-efficient"},

		// Claude 3.5 系列
		{Name: "claude-3-5-sonnet-20241022", DisplayName: "Claude 3.5 Sonnet (新)", Protocol: "anthropic", Category: "flagship"},
		{Name: "claude-3-5-haiku-20241022", DisplayName: "Claude 3.5 Haiku — 快速轻量", Protocol: "anthropic", Category: "cost-efficient"},

		// Claude 3 系列
		{Name: "claude-3-opus-20240229", DisplayName: "Claude 3 Opus — 前代旗舰", Protocol: "anthropic", Category: "legacy"},
		{Name: "claude-3-sonnet-20240229", DisplayName: "Claude 3 Sonnet", Protocol: "anthropic", Category: "legacy"},
		{Name: "claude-3-haiku-20240307", DisplayName: "Claude 3 Haiku", Protocol: "anthropic", Category: "legacy"},
	},

	// google 键对应 Google Agent Platform (GEAP) 协议
	// 所有节点均需配置 project_id + location + API Key，统一走 aiplatform.googleapis.com 端点
	// 模型分两类：claude-* 前缀走 GEAP rawPredict（Claude 直通）；gemini-* 走 GenerateContent（协议转换）
	"google": {
		// 通配符
		{Name: "*", DisplayName: "全部 Google/Gemini 模型 (通配符)", Protocol: "google", Category: "wildcard"},

		// Claude 合作伙伴模型（GEAP 专属，需 project_id）
		{Name: "claude-opus-4-7", DisplayName: "Claude Opus 4.7 (GEAP)", Protocol: "google", Category: "flagship"},
		{Name: "claude-opus-4-6", DisplayName: "Claude Opus 4.6 (GEAP)", Protocol: "google", Category: "flagship"},
		{Name: "claude-sonnet-4-6", DisplayName: "Claude Sonnet 4.6 (GEAP)", Protocol: "google", Category: "flagship"},
		{Name: "claude-sonnet-4-5", DisplayName: "Claude Sonnet 4.5 (GEAP)", Protocol: "google", Category: "flagship"},
		{Name: "claude-haiku-4-5", DisplayName: "Claude Haiku 4.5 (GEAP)", Protocol: "google", Category: "cost-efficient"},
		{Name: "claude-3-5-sonnet-20241022", DisplayName: "Claude 3.5 Sonnet (GEAP)", Protocol: "google", Category: "flagship"},
		{Name: "claude-3-5-haiku-20241022", DisplayName: "Claude 3.5 Haiku (GEAP)", Protocol: "google", Category: "cost-efficient"},

		// Gemini 3.1 系列
		{Name: "gemini-3.1-pro-preview-customtools", DisplayName: "Gemini 3.1 Pro CustomTools", Protocol: "google", Category: "flagship"},
		{Name: "gemini-3.1-pro-preview", DisplayName: "Gemini 3.1 Pro Preview", Protocol: "google", Category: "flagship"},
		{Name: "gemini-3.1-pro", DisplayName: "Gemini 3.1 Pro — 旗舰", Protocol: "google", Category: "flagship"},
		{Name: "gemini-3.1-flash", DisplayName: "Gemini 3.1 Flash — 快速版", Protocol: "google", Category: "cost-efficient"},
		{Name: "gemini-3.1-flash-lite-preview", DisplayName: "Gemini 3.1 Flash Lite Preview", Protocol: "google", Category: "cost-efficient"},
		{Name: "gemini-3.1-flash-lite", DisplayName: "Gemini 3.1 Flash Lite", Protocol: "google", Category: "cost-efficient"},
		{Name: "gemini-3.1-ultra", DisplayName: "Gemini 3.1 Ultra — 最强版", Protocol: "google", Category: "flagship"},

		// Gemini 3.0 系列
		{Name: "gemini-3.0-pro", DisplayName: "Gemini 3.0 Pro", Protocol: "google", Category: "flagship"},
		{Name: "gemini-3.0-flash", DisplayName: "Gemini 3.0 Flash", Protocol: "google", Category: "cost-efficient"},
		{Name: "gemini-3-flash-preview", DisplayName: "Gemini 3 Flash Preview", Protocol: "google", Category: "cost-efficient"},

		// Gemini 2.5 系列
		{Name: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", Protocol: "google", Category: "flagship"},
		{Name: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", Protocol: "google", Category: "cost-efficient"},

		// Gemini 2.0 系列
		{Name: "gemini-2.0-pro-exp", DisplayName: "Gemini 2.0 Pro Exp", Protocol: "google", Category: "flagship"},
		{Name: "gemini-2.0-flash", DisplayName: "Gemini 2.0 Flash", Protocol: "google", Category: "cost-efficient"},
		{Name: "gemini-2.0-flash-lite", DisplayName: "Gemini 2.0 Flash Lite", Protocol: "google", Category: "cost-efficient"},

		// Gemini 1.5 系列
		{Name: "gemini-1.5-pro", DisplayName: "Gemini 1.5 Pro", Protocol: "google", Category: "legacy"},
		{Name: "gemini-1.5-flash", DisplayName: "Gemini 1.5 Flash", Protocol: "google", Category: "legacy"},

		// 带 google/ 前缀的 Gemini 模型（用于 OpenAI 兼容端点）
		{Name: "google/gemini-3.1-pro-preview-customtools", DisplayName: "Gemini 3.1 Pro CT (OpenAI兼容)", Protocol: "google", Category: "flagship"},
		{Name: "google/gemini-3.1-pro-preview", DisplayName: "Gemini 3.1 Pro Preview (OpenAI兼容)", Protocol: "google", Category: "flagship"},
		{Name: "google/gemini-3.1-pro", DisplayName: "Gemini 3.1 Pro (OpenAI兼容)", Protocol: "google", Category: "flagship"},
		{Name: "google/gemini-3.1-flash", DisplayName: "Gemini 3.1 Flash (OpenAI兼容)", Protocol: "google", Category: "cost-efficient"},
		{Name: "google/gemini-3.1-flash-lite-preview", DisplayName: "Gemini 3.1 Flash Lite Preview (OpenAI兼容)", Protocol: "google", Category: "cost-efficient"},
		{Name: "google/gemini-3.1-flash-lite", DisplayName: "Gemini 3.1 Flash Lite (OpenAI兼容)", Protocol: "google", Category: "cost-efficient"},
		{Name: "google/gemini-3.0-pro", DisplayName: "Gemini 3.0 Pro (OpenAI兼容)", Protocol: "google", Category: "flagship"},
		{Name: "google/gemini-3.0-flash", DisplayName: "Gemini 3.0 Flash (OpenAI兼容)", Protocol: "google", Category: "cost-efficient"},
		{Name: "google/gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash (OpenAI兼容)", Protocol: "google", Category: "cost-efficient"},
		{Name: "google/gemini-2.0-pro-exp", DisplayName: "Gemini 2.0 Pro Exp (OpenAI兼容)", Protocol: "google", Category: "flagship"},
		{Name: "google/gemini-2.0-flash", DisplayName: "Gemini 2.0 Flash (OpenAI兼容)", Protocol: "google", Category: "cost-efficient"},
		{Name: "google/gemini-1.5-pro", DisplayName: "Gemini 1.5 Pro (OpenAI兼容)", Protocol: "google", Category: "legacy"},
		{Name: "google/gemini-1.5-flash", DisplayName: "Gemini 1.5 Flash (OpenAI兼容)", Protocol: "google", Category: "legacy"},
	},

}
