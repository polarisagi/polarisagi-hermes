package domain

// SysModel 代表系统内置支持的各厂商的大模型
type SysModel struct {
	ModelID          string `json:"model_id"`
	ProviderID       string `json:"provider_id"`
	DisplayName      string `json:"display_name"`
	CapabilityTier   string `json:"capability_tier"`
	ContextLength    int    `json:"context_length"`
	MaxOutputTokens  int    `json:"max_output_tokens"`
	SupportsVision   bool   `json:"supports_vision"`
	SupportsTools    bool   `json:"supports_tools"`
}

// SysModelEndpointBinding 解决同一模型在不同端点 API 字符串不同的问题
type SysModelEndpointBinding struct {
	ModelID        string `json:"model_id"`
	EndpointID     string `json:"endpoint_id"`
	ActualModelID  string `json:"actual_model_id"`
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
	ModelID          string `json:"model_id"`
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
