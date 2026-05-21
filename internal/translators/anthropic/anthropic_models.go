// Anthropic 协议数据模型定义
// 包含：请求/响应结构体、SSE 流式事件结构体
// 用于 Anthropic ↔ Vertex/OpenAI 的协议转换适配
package anthropic

// MessageRequest Anthropic Messages API 请求结构
// 对应 POST https://api.anthropic.com/v1/messages
type MessageRequest struct {
	Model             string             `json:"model"`                    // 模型名，如 "claude-sonnet-4-6"
	Messages          []Message          `json:"messages"`                 // 对话消息列表
	System            interface{}        `json:"system,omitempty"`         // 系统提示词 (string 或 []Content)
	MaxTokens         int                `json:"max_tokens"`               // 最大生成 token 数
	Temperature       *float64           `json:"temperature,omitempty"`    // 温度参数 [0,1]
	TopP              *float64           `json:"top_p,omitempty"`          // Top-P 采样
	TopK              *int               `json:"top_k,omitempty"`          // Top-K 采样
	Stream            bool               `json:"stream,omitempty"`         // 是否流式响应
	Tools             []Tool             `json:"tools,omitempty"`          // 可用工具列表
	ToolChoice        *ToolChoice        `json:"tool_choice,omitempty"`    // 工具选择策略
	StopSequences     []string           `json:"stop_sequences,omitempty"` // 自定义停止序列
	Thinking          *ThinkingConfig    `json:"thinking,omitempty"`       // 扩展思考配置（Claude Code /effort 命令使用）
	Metadata          *RequestMetadata   `json:"metadata,omitempty"`       // 请求元数据（用户标识等）
	ContextManagement *ContextManagement `json:"context_management,omitempty"` // 上下文管理配置 (自动压缩/清理等)
}

// ContextManagement Anthropic 上下文管理配置 (beta 特性)
type ContextManagement struct {
	Edits []ContextEdit `json:"edits,omitempty"`
}

type ContextEdit struct {
	Type     string `json:"type,omitempty"`     // 比如 "clear_thinking_20251015" 或 "compact_20260112"
	Strategy string `json:"strategy,omitempty"` // "auto" 等
	Keep     string `json:"keep,omitempty"`     // "all" 等
}

// ThinkingConfig Anthropic 扩展思考配置
// type="enabled" 时启用思考，budget_tokens 给出思考可用的 token 预算
// 映射到 Gemini 的 generationConfig.thinkingConfig.thinkingBudget + includeThoughts:true
type ThinkingConfig struct {
	Type         string `json:"type,omitempty"`          // "enabled" / "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // 思考阶段可用的 token 数
}

// RequestMetadata Anthropic 请求元数据
type RequestMetadata struct {
	UserID string `json:"user_id,omitempty"` // 终端用户标识，用于滥用防控
}

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
	// Type 标识 Anthropic 内置工具类型，如 "bash_20250124"、"computer_20250124"、"text_editor_20250124"
	// Gemini 无对等内置工具，处理时需跳过这类工具避免无效的 functionDeclaration
	Type        string                 `json:"type,omitempty"`
}

type ToolChoice struct {
	Type                   string `json:"type"`                               // "auto", "any", "tool"
	Name                   string `json:"name,omitempty"`                     // required if type is "tool"
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

// Message 对话消息，Content 可以是纯文本字符串或内容块数组
type Message struct {
	Role    string      `json:"role"`    // "user" 或 "assistant"
	Content interface{} `json:"content"` // 字符串或 []Content（支持多模态和工具调用）
}

// MessageResponse Anthropic 非流式响应结构
type MessageResponse struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Content      []Content `json:"content"`       // 响应内容块列表
	Model        string    `json:"model"`
	StopReason   string    `json:"stop_reason"`   // 停止原因: "end_turn"/"max_tokens"/"stop_sequence"/"tool_use"
	StopSequence string    `json:"stop_sequence"`
	Usage        Usage     `json:"usage"`
}

// Content 内容块，Anthropic 使用 content blocks 而非纯文本
type Content struct {
	Type      string                 `json:"type"`                  // "text" / "image" / "tool_use" / "tool_result" / "thinking"
	Text      string                 `json:"text,omitempty"`        // for text
	Thinking  string                 `json:"thinking,omitempty"`    // for thinking
	Signature string                 `json:"signature,omitempty"`   // for thinking（Anthropic 要求携带，用于验证）
	ID        string                 `json:"id,omitempty"`          // for tool_use
	Name      string                 `json:"name,omitempty"`        // for tool_use
	Input     interface{}            `json:"input,omitempty"`       // for tool_use
	ToolUseID string                 `json:"tool_use_id,omitempty"` // for tool_result
	Content   interface{}            `json:"content,omitempty"`     // for tool_result
	Source    map[string]interface{} `json:"source,omitempty"`      // for image/document
}

// Usage Anthropic 用量统计
// Claude Code 的 /context 与 /cost 命令通过这些字段计算上下文占用百分比与累计费用
//   - cache_creation_input_tokens：本次新写入 prompt cache 的 token 数（Anthropic 原生支持）
//   - cache_read_input_tokens：命中 prompt cache 的 token 数（映射自 Gemini cachedContentTokenCount）
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
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
	Type         string `json:"type,omitempty"`
	Text         string `json:"text,omitempty"`
	Thinking     string `json:"thinking,omitempty"`      // for thinking_delta
	Signature    string `json:"signature,omitempty"`     // for signature_delta（thinking 块的签名，用于多轮对话验证）
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"` // 触发停止的序列
	PartialJson  string `json:"partial_json,omitempty"`  // for input_json_delta (tool_use)
	Content      string `json:"content,omitempty"`       // for compaction_delta（/compact 上下文压缩摘要块）
}