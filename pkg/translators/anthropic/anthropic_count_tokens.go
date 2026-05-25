// Anthropic /v1/messages/count_tokens 端点支持
// Claude Code 客户端使用此端点驱动 /context 命令显示 token 占用、
// 以及自动判断是否触发 /compact 上下文压缩。
//
// 协议层面：与 /v1/messages 共享同一请求体 schema，响应为 {"input_tokens": N}。
//
// 本网关实现策略：
//   - anthropic→anthropic 透传：转发到上游真实的 count_tokens 端点，最高精度
//   - anthropic→google (GEAP Claude)：转发到 GEAP rawPredict count-tokens 端点，精确计数
//   - anthropic→google (Gemini)：调用 Gemini countTokens 端点，精确计数
//   - anthropic→openai：本地估算（OpenAI 无对等端点）
package anthropic

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"polaris-gateway/internal/core/router"
)

func init() {
	router.RegisterCountTokensHandler("anthropic", handleCountTokensLocal)
}


// isCountTokensPath 判断请求路径是否为 count_tokens 端点
// 路由层已剥离协议前缀，此处看到的路径形如 /v1/messages/count_tokens
func isCountTokensPath(path string) bool {
	return strings.Contains(path, "/count_tokens")
}

// estimateAnthropicTokens 本地估算 Anthropic Messages 请求的 input token 数
// 使用 tiktoken (o200k_base) 提供高精度的精确内存估算
// 此计算能够支撑 Claude Code 准确判断上下文并触发 /compact，避免超限报错。
func estimateAnthropicTokens(req MessageRequest) int {
	total := 0

	// System prompt：支持字符串或内容块数组
	switch sys := req.System.(type) {
	case string:
		total += router.CountTextTokens(sys)
	case []interface{}:
		for _, item := range sys {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					total += router.CountTextTokens(t)
				}
			}
		}
	}

	// Messages
	for _, msg := range req.Messages {
		switch v := msg.Content.(type) {
		case string:
			total += router.CountTextTokens(v)
		case []interface{}:
			for _, item := range v {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				switch m["type"] {
				case "text":
					if t, ok := m["text"].(string); ok {
						total += router.CountTextTokens(t)
					}
				case "image", "document":
					// 图片/PDF 按 Anthropic 官方经验值近似
					total += 1500
				case "tool_use":
					if input, ok := m["input"]; ok {
						b, _ := json.Marshal(input)
						total += router.CountTextTokens(string(b))
					}
					if name, ok := m["name"].(string); ok {
						total += router.CountTextTokens(name)
					}
				case "tool_result":
					if c, ok := m["content"].(string); ok {
						total += router.CountTextTokens(c)
					} else if arr, ok := m["content"].([]interface{}); ok {
						for _, ci := range arr {
							if cm, ok := ci.(map[string]interface{}); ok {
								if t, ok := cm["text"].(string); ok {
									total += router.CountTextTokens(t)
								}
							}
						}
					}
				case "thinking":
					// Claude Code 历史中保留的思考块仍计入上下文
					if t, ok := m["thinking"].(string); ok {
						total += router.CountTextTokens(t)
					}
				case "redacted_thinking":
					// 加密 thinking blob，固定开销估算
					total += 50
				case "compaction":
					// /compact 产生的历史摘要检查点
					if c, ok := m["content"].(string); ok {
						total += router.CountTextTokens(c)
					}
				}
			}
		}
		total += 4 // role 等结构开销
	}

	// Tools：name + description + input_schema
	// Anthropic 对内置工具类型有固定 token 开销（官方统计）：
	//   bash: ~245 tokens, text_editor: ~700 tokens, computer: ~735 tokens
	for _, tool := range req.Tools {
		if tool.Type != "" {
			// 内置工具按类型计固定开销，无需序列化 schema（schema 内置于模型中）
			total += builtinToolTokenCost(tool.Type)
			continue
		}
		total += router.CountTextTokens(tool.Name)
		total += router.CountTextTokens(tool.Description)
		if tool.InputSchema != nil {
			b, _ := json.Marshal(tool.InputSchema)
			total += router.CountTextTokens(string(b))
		}
	}

	return total
}

// builtinToolTokenCost 返回 Anthropic 内置工具的固定 token 开销估算
// 参考 Anthropic 官方文档的 token overhead 数据
func builtinToolTokenCost(toolType string) int {
	switch {
	case strings.Contains(toolType, "bash"):
		return 245
	case strings.Contains(toolType, "text_editor"), strings.Contains(toolType, "str_replace_based_edit_tool"):
		return 700
	case strings.Contains(toolType, "computer"):
		return 735
	case strings.Contains(toolType, "web_search"):
		return 100
	case strings.Contains(toolType, "web_fetch"):
		return 100
	case strings.Contains(toolType, "code_execution"):
		return 150
	case strings.Contains(toolType, "memory"):
		return 600
	default:
		return 100
	}
}

// handleCountTokensLocal 在网关本地估算后直接返回 Anthropic 格式响应
// 适用于上游协议不是 Anthropic 的场景（Vertex、OpenAI），因为这些协议
// 没有可对等的 count_tokens 端点。
func handleCountTokensLocal(w http.ResponseWriter, bodyBytes []byte, traceID string) {
	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"invalid json"}}`, http.StatusBadRequest)
		return
	}

	tokens := estimateAnthropicTokens(req)

	slog.Debug("📏 [CountTokens] 本地估算返回", "trace_id", traceID, "input_tokens", tokens, "model", req.Model)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]int{
		"input_tokens": tokens,
	})
}

