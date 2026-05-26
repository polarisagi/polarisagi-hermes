package router

import (
	"context"
	"regexp"

	"polaris-hermes/internal/domain"
	"polaris-hermes/internal/repository/sqlite"
)

// IntentInferer 负责推断未知模型的意图标签
type IntentInferer struct {
	intentRepo *sqlite.IntentRepo
}

func NewIntentInferer(intentRepo *sqlite.IntentRepo) *IntentInferer {
	return &IntentInferer{
		intentRepo: intentRepo,
	}
}

// InferUnknownModel 对未知模型进行自动学习与分类
func (i *IntentInferer) InferUnknownModel(ctx context.Context, modelID string) string {
	// 1. 正则/关键字快速推断 (优先级高，成本低)
	tier := i.inferByKeywords(modelID)

	// 2. 如果关键字推断失败，触发 LLM 智能推断
	if tier == "" {
		tier = i.inferByLLM(ctx, modelID)
	}

	// 3. 兜底策略：如果连 LLM 都失败了，默认归类为 flagship 旗舰模型，避免网关阻塞
	if tier == "" {
		tier = "flagship"
	}

	// 4. 将推断结果持久化，形成闭环进化
	source := "auto_regex"
	if tier == i.inferByLLM(ctx, modelID) && tier != "" {
		source = "auto_llm"
	}
	
	_ = i.intentRepo.SaveUserIntent(ctx, &domain.UserModelIntentDict{
		RequestedModelID: modelID,
		CapabilityTier:   tier,
		Source:           source,
	})

	return tier
}

// inferByKeywords 通过内置的高命中率特征字推断模型意图
func (i *IntentInferer) inferByKeywords(modelID string) string {
	// 推理型模型 (Highest Priority for specific keywords)
	if match, _ := regexp.MatchString(`(?i)(\b(o1|o3|r1)\b|reason)`, modelID); match {
		return "reasoning"
	}

	// 向量模型
	if match, _ := regexp.MatchString(`(?i)(embed)`, modelID); match {
		return "embedding"
	}

	// 超大杯模型
	if match, _ := regexp.MatchString(`(?i)(opus|ultra)`, modelID); match {
		return "ultra"
	}

	// 极速/轻量化模型
	if match, _ := regexp.MatchString(`(?i)(\bmini\b|haiku|flash|lite|nano|turbo)`, modelID); match {
		return "light"
	}

	// 旗舰/重度模型
	if match, _ := regexp.MatchString(`(?i)(sonnet|pro|gpt-4|gpt-3\.5|max|large|70b)`, modelID); match {
		return "flagship"
	}

	return "" // 无法从字面推断
}

// inferByLLM 调用内部大模型接口，询问这个未知模型属于什么阵营
func (i *IntentInferer) inferByLLM(ctx context.Context, modelID string) string {
	// TODO: 等待 Proxy 层的内部调用接口就绪
	return ""
}

