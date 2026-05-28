package domain

// SysModel 代表系统内置支持的各厂商的大模型（仅存储客观技术元数据）
type SysModel struct {
	ModelID          string `json:"model_id"`
	ProviderID       string `json:"provider_id"`
	DisplayName      string `json:"display_name"`
	// capability_tier 已移至 sys_model_intent_dict，不再在此冗余存储
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

// SysModelIntent 全局模型 ID → 能力梯队字典
// 同时覆盖客户端模型名（gpt-4o、claude-sonnet）和服务端模型名（deepseek-v4-flash、gemini-2.5-pro）
type SysModelIntent struct {
	ModelID        string `json:"model_id"`       // 任意模型标识符，不限于客户端请求
	CapabilityTier string `json:"capability_tier"` // "smart", "fast", "reasoning"
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

// UserModelIntentDict 用户级别的手动覆盖字典（优先级高于系统字典）
type UserModelIntentDict struct {
	ID             int    `json:"id"`
	ModelID        string `json:"model_id"`         // 任意模型标识符（客户端或服务端均可）
	CapabilityTier string `json:"capability_tier"`
	Source         string `json:"source"` // "manual", "auto_regex", "auto_llm"
}
