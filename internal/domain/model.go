package domain

// SysModel 官方大模型的基础物理属性（不含意图分级）
type SysModel struct {
	ID               int    `json:"id"`
	ProviderID       string `json:"provider_id"`
	ActualModelID    string `json:"actual_model_id"` // 如 "gemini-3.0-pro"
	DisplayName      string `json:"display_name"`
	ContextLength    int    `json:"context_length"`
	MaxOutputTokens  int    `json:"max_output_tokens"`
	SupportsVision   bool   `json:"supports_vision"`
	SupportsTools    bool   `json:"supports_tools"`
}

// SysModelIntent 全局公认的模型意图映射字典
type SysModelIntent struct {
	RequestedModelID string `json:"requested_model_id"` // 如 "claude-3-opus-20240229"
	CapabilityTier   string `json:"capability_tier"`    // "smart", "fast", "reasoning"
}

// UserModel 用户为自己的某个渠道配置的模型实体（包含了主观的意图分级）
type UserModel struct {
	ID               int    `json:"id"`
	UserProviderID   int    `json:"user_provider_id"`
	DisplayName      string `json:"display_name"`
	ActualModelID    string `json:"actual_model_id"`
	CapabilityTier   string `json:"capability_tier"`    // 主观标记该模型的梯队
	IsActive         bool   `json:"is_active"`
}

// UserModelIntentDict 用户级别的自动学习推断/覆盖字典
type UserModelIntentDict struct {
	ID               int    `json:"id"`
	RequestedModelID string `json:"requested_model_id"`
	CapabilityTier   string `json:"capability_tier"`
	Source           string `json:"source"` // "manual", "auto_regex", "auto_llm"
}
