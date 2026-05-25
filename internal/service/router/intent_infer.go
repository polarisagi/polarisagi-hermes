package router

import (
	"context"
	"strings"

	"polaris-gateway/internal/domain"
	"polaris-gateway/internal/repository/sqlite"
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
	lowerID := strings.ToLower(modelID)

	// 1. 正则/关键字快速推断 (优先级高，成本低)
	tier := i.inferByKeywords(lowerID)

	// 2. 如果关键字推断失败，触发 LLM 智能推断 (TODO: 依赖代理层提供一个内部调用接口)
	if tier == "" {
		tier = i.inferByLLM(ctx, modelID)
	}

	// 3. 兜底策略：如果连 LLM 都失败了，默认归类为 fast 极速模型，避免网关阻塞
	if tier == "" {
		tier = "fast"
	}

	// 4. 将推断结果持久化，形成闭环进化
	source := "auto_regex"
	if tier == i.inferByLLM(ctx, modelID) && tier != "" { // 伪逻辑，实际由具体方法返回源
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
	if strings.Contains(modelID, "o1") || strings.Contains(modelID, "o3") || 
	   strings.Contains(modelID, "reason") || strings.Contains(modelID, "r1") {
		return "reasoning"
	}

	// 极速/轻量化模型
	if strings.Contains(modelID, "mini") || strings.Contains(modelID, "flash") || 
	   strings.Contains(modelID, "haiku") || strings.Contains(modelID, "8b") || 
	   strings.Contains(modelID, "turbo") || strings.Contains(modelID, "lite") {
		return "fast"
	}

	// 旗舰/重度模型
	if strings.Contains(modelID, "pro") || strings.Contains(modelID, "opus") || 
	   strings.Contains(modelID, "max") || strings.Contains(modelID, "large") || 
	   strings.Contains(modelID, "70b") || strings.Contains(modelID, "gpt-4") {
		return "smart"
	}

	return "" // 无法从字面推断
}

// inferByLLM 调用内部大模型接口，询问这个未知模型属于什么阵营
func (i *IntentInferer) inferByLLM(ctx context.Context, modelID string) string {
	// TODO: 等待 Proxy 层的内部调用接口就绪
	// 预期逻辑：找到当前系统中一个活着的、最便宜的 fast 模型
	// 发送 Prompt: "模型名叫做 {modelID} 的模型，是旗舰大模型、快速小模型还是具备复杂思考过程的推理大模型？请只回答 'smart', 'fast' 或 'reasoning' 中的一个词。"
	return ""
}
