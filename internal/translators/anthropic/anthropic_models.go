// Anthropic 协议数据模型定义
// 包含：请求/响应结构体、SSE 流式事件结构体
// 用于 Anthropic ↔ Vertex/OpenAI 的协议转换适配
package anthropic

// MessageRequest Anthropic Messages API 请求结构
// 对应 POST https://api.anthropic.com/v1/messages
type MessageRequest struct {
	Model       string    `json:"model"`                 // 模型名，如 "claude-sonnet-4-6"
	Messages    []Message `json:"messages"`              // 对话消息列表
	System      string    `json:"system,omitempty"`      // 系统提示词
	MaxTokens   int       `json:"max_tokens"`            // 最大生成 token 数
	Temperature *float64  `json:"temperature,omitempty"` // 温度参数
	TopP        *float64  `json:"top_p,omitempty"`       // Top-P 采样
	TopK        *int      `json:"top_k,omitempty"`       // Top-K 采样
	Stream      bool      `json:"stream,omitempty"`      // 是否流式响应
}

// Message 对话消息，Content 可以是纯文本字符串或内容块数组
type Message struct {
	Role    string      `json:"role"`    // "user" 或 "assistant"
	Content interface{} `json:"content"` // 字符串或 []ContentBlock（支持多模态）
}

// MessageResponse Anthropic 非流式响应结构
type MessageResponse struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Content      []Content `json:"content"`       // 响应内容块列表
	Model        string    `json:"model"`
	StopReason   string    `json:"stop_reason"`   // 停止原因: "end_turn"/"max_tokens"/"stop_sequence"
	StopSequence string    `json:"stop_sequence"`
	Usage        Usage     `json:"usage"`
}

// Content 内容块，Anthropic 使用 content blocks 而非纯文本
type Content struct {
	Type string `json:"type"` // "text" / "image" / "tool_use" 等
	Text string `json:"text"`
}

// Usage Anthropic 用量统计，区分 input_tokens 和 output_tokens
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ── Anthropic SSE 流式事件结构体 ──
// Anthropic 使用 Server-Sent Events (SSE) 推送流式响应
// 事件序列: message_start → content_block_start → content_block_delta* → content_block_stop → message_delta → message_stop

type StreamEvent struct {
	Type         string           `json:"type"`
	Message      *MessageResponse `json:"message,omitempty"`
	Index        *int             `json:"index,omitempty"`
	ContentBlock *Content         `json:"content_block,omitempty"`
	Delta        *Delta           `json:"delta,omitempty"`
	Usage        *Usage           `json:"usage,omitempty"`
}

// Delta SSE 增量更新，携带文本片段或停止原因
type Delta struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}