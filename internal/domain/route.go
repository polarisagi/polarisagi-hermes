package domain

import "time"

// UserCustomRoute 专业模式：强制 1对1 路由
type UserCustomRoute struct {
	ID                int    `json:"id"`
	RequestedModelID  string `json:"requested_model_id"` // 客户端请求的模型，如 "gpt-4o"
	TargetUserModelID int    `json:"target_user_model_id"` // 强制绑定的后台 UserModels 的 ID
	IsActive          bool   `json:"is_active"`
}

// AccountLog 请求流水账单
type AccountLog struct {
	ID               int       `json:"id"`
	UserProviderID   int       `json:"user_provider_id"`
	RequestedModelID string    `json:"requested_model_id"`
	ActualModelID    string    `json:"actual_model_id"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	LatencyMs        int       `json:"latency_ms"`
	StatusCode       int       `json:"status_code"`
	ErrorMsg         string    `json:"error_msg"`
	CreatedAt        time.Time `json:"created_at"`
}
