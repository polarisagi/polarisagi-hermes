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
	
	if strings.Contains(modelID, "latest") {
		weight += 9999999 // latest remains a high weight
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
		// prepended baseWeight ensures newer major versions take precedence
		weight += baseWeight * 10000
	}

	return weight
}

// IsLegacyModel 判断模型是否为过时的快照版本（例如带有日期后缀的 gpt-4o-2024-11-20，或带有 legacy 标识）
func (i *IntentInferer) IsLegacyModel(modelID string) bool {
	// 包含具体日期格式的，通常是历史快照，属于旧模型 (e.g. 2024-11-20, 20241120)
	dateRe := regexp.MustCompile(`(202\d)[-]?(\d{2})[-]?(\d{2})`)
	if dateRe.MatchString(modelID) {
		return true
	}
	
	// 其他四位或六位数字格式的日期后缀（如 -0314, -0613, -1106, -0125, -240718）
	// (避免误伤 gpt-4 之类的数字，所以要求前面有连字符，或者是典型的月日结构)
	shortDateRe := regexp.MustCompile(`-(0[1-9]|1[0-2])([0-2][0-9]|3[01])$|-\d{6}$`)
	if shortDateRe.MatchString(modelID) {
		return true
	}
	
	// 明确标记为过时的
	lowerID := strings.ToLower(modelID)
	if strings.Contains(lowerID, "legacy") || strings.Contains(lowerID, "deprecated") || strings.Contains(lowerID, "gpt-3.5") {
		return true
	}
	
	return false
}

