package router

import (
	"context"
	"regexp"
	"strconv"
	"strings"

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
	source := "auto_regex"
	tier := i.inferByKeywords(modelID)

	// 2. 如果关键字推断失败，触发 LLM 智能推断
	if tier == "" {
		tier = i.inferByLLM(ctx, modelID)
		if tier != "" {
			source = "auto_llm"
		}
	}

	// 3. 兜底策略：如果连 LLM 都失败了，默认归类为 smart 旗舰模型，避免网关阻塞
	if tier == "" {
		tier = "smart"
	}

	// 4. 将推断结果持久化，形成闭环进化
	_ = i.intentRepo.SaveUserIntent(ctx, &domain.UserModelIntentDict{
		ModelID:        modelID,
		CapabilityTier: tier,
		Source:         source,
	})

	return tier
}

// inferByKeywords 通过内置的高命中率特征字推断模型意图
func (i *IntentInferer) inferByKeywords(modelID string) string {
	// 推理/沉思型模型 (Highest Priority for specific keywords)
	if match, _ := regexp.MatchString(`(?i)(\b(o1|o3|o4|r1|r2)\b|reason|thinking|deepseek-v4-pro)`, modelID); match {
		return "reasoning"
	}

	// 极速/轻量化模型
	if match, _ := regexp.MatchString(`(?i)(\bmini\b|haiku|flash|lite|nano|turbo|fast|small)`, modelID); match {
		return "fast"
	}

	// 旗舰/智能模型
	if match, _ := regexp.MatchString(`(?i)(sonnet|opus|pro|max|large|gpt-4|gpt-5|v3|v4|ultra|70b|120b|405b)`, modelID); match {
		return "smart"
	}

	return "" // 无法从字面推断
}

// inferByLLM 调用内部大模型接口，询问这个未知模型属于什么阵营
func (i *IntentInferer) inferByLLM(ctx context.Context, modelID string) string {
	// TODO: 等待 Proxy 层的内部调用接口就绪
	return ""
}

// ParseVersionWeight 解析模型名称，提取版本权重用于排序
func (i *IntentInferer) ParseVersionWeight(modelID string) int {
	weight := 0
	
	dateRe := regexp.MustCompile(`(202\d)[-]?(\d{2})[-]?(\d{2})`)
	if matches := dateRe.FindStringSubmatch(modelID); len(matches) == 4 {
		year, _ := strconv.Atoi(matches[1])
		month, _ := strconv.Atoi(matches[2])
		day, _ := strconv.Atoi(matches[3])
		weight += year*10000 + month*100 + day
	} else if strings.Contains(modelID, "latest") {
		weight += 20999999 // a very large date representation for "latest"
	}

	verRe := regexp.MustCompile(`(?:gpt-|gemini-|claude-|v|o)(\d+)(?:[-.](\d+))?(o)?`)
	if matches := verRe.FindStringSubmatch(modelID); len(matches) > 1 {
		major, _ := strconv.Atoi(matches[1])
		minor := 0
		if len(matches) > 2 && matches[2] != "" {
			minor, _ = strconv.Atoi(matches[2])
		}
		
		baseWeight := major * 10000 + minor * 100
		if len(matches) > 3 && matches[3] == "o" {
			baseWeight += 500
		}
		// prepend baseWeight so it dominates the date weight
		weight += baseWeight * 100000000
	}

	return weight
}

