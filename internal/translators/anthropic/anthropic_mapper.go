// Anthropic → Google Agent Platform 请求映射器
// 将 Anthropic Messages API 格式转换为 GEAP GenerateContent API 格式
package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// toolThoughtSigCache 跨请求保存 Gemini 3.x functionCall 携带的 thoughtSignature。
// key: tool_use_id（Anthropic 格式）→ value: thoughtSignature（Gemini 格式）
// Gemini 3.x 要求多轮 function calling 时在历史中带回 thoughtSignature，
// 否则返回 400 "Function call is missing a thought_signature"。
var toolThoughtSigCache sync.Map

// geapSafetySettings BLOCK_NONE 安全配置，针对所有文本内容类别
// 原因：Claude Code 频繁发送含代码、安全研究、命令行等内容，Gemini 默认阈值会误触安全过滤器
// 本网关作为 API 代理，上游客户端已自行承担内容责任，无需二次拦截
var geapSafetySettings = []map[string]interface{}{
	{"category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "BLOCK_NONE"},
	{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_NONE"},
	{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "BLOCK_NONE"},
	{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "threshold": "BLOCK_NONE"},
	{"category": "HARM_CATEGORY_JAILBREAK", "threshold": "BLOCK_NONE"},
}

// findLastCompactionIndex 返回 messages 中最后一个包含 compaction 块的消息下标，
// 未找到时返回 -1。
// compaction 块是 Claude Code /compact 产生的检查点，Anthropic API 规定其之前的消息全部丢弃。
func findLastCompactionIndex(messages []Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if arr, ok := messages[i].Content.([]interface{}); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					if m["type"] == "compaction" {
						return i
					}
				}
			}
		}
	}
	return -1
}

// isGemini3Model 判断目标模型是否为 Gemini 3.x 系列
// Gemini 3.x 的 thinkingConfig 使用 thinkingLevel（LOW/MEDIUM/HIGH）替代 thinkingBudget（整数）
func isGemini3Model(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "gemini-3")
}

// budgetToThinkingLevel 将 Anthropic thinking.budget_tokens 映射到 Gemini 3.x thinkingLevel 枚举值
// Claude Code /effort 默认 budget 约 16000，映射到 HIGH
func budgetToThinkingLevel(budgetTokens int) string {
	switch {
	case budgetTokens <= 0:
		return "MEDIUM" // 未指定时取中档，兼顾质量与速度
	case budgetTokens <= 5000:
		return "LOW"
	case budgetTokens <= 16000:
		return "MEDIUM"
	default:
		return "HIGH"
	}
}

// mapToVertexRequest 将 Anthropic Messages 请求转换为 GEAP 原生的 generateContent 请求体
// model 参数用于区分 Gemini 2.5（thinkingBudget）与 Gemini 3.x（thinkingLevel）的 thinking API 差异
// 转换规则:
//   - Anthropic system → GEAP systemInstruction
//   - Anthropic user/assistant → GEAP user/model 角色
//   - Anthropic 纯文本/多模态/工具调用内容块 → GEAP parts 数组
//   - Anthropic max_tokens → GEAP maxOutputTokens
//   - Anthropic temperature/topP/topK → GEAP generationConfig
//   - Anthropic tools → GEAP tools (functionDeclarations)
//   - Anthropic tool_choice → GEAP toolConfig
//   - Anthropic metadata.user_id → GEAP labels（用于计费追踪）
//   - 默认附加 safetySettings=BLOCK_NONE 避免安全过滤器误杀代码内容
func mapToVertexRequest(req MessageRequest, model string) (map[string]interface{}, error) {
	vertexReq := make(map[string]interface{})

	if req.System != nil {
		var parts []map[string]interface{}
		switch sys := req.System.(type) {
		case string:
			if sys != "" {
				parts = append(parts, map[string]interface{}{"text": sys})
			}
		case []interface{}:
			for _, item := range sys {
				if m, ok := item.(map[string]interface{}); ok {
					if text, ok := m["text"].(string); ok {
						parts = append(parts, map[string]interface{}{"text": text})
					}
				}
			}
		}
		if len(parts) > 0 {
			vertexReq["systemInstruction"] = map[string]interface{}{
				"parts": parts,
			}
		}
	}

	// 预处理：若消息链中含有 compaction 块（来自上一次 /compact），
	// 截断其之前的所有消息（Anthropic API 的行为：compaction 块是历史检查点，之前内容已被摘要替代）
	messages := req.Messages
	if lastIdx := findLastCompactionIndex(messages); lastIdx >= 0 {
		messages = messages[lastIdx:]
	}

	// Build a map of tool_use_id to tool name for mapping tool_results later
	toolMap := make(map[string]string)
	for _, msg := range messages {
		if arr, ok := msg.Content.([]interface{}); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					if m["type"] == "tool_use" {
						if id, ok := m["id"].(string); ok {
							if name, ok := m["name"].(string); ok {
								toolMap[id] = name
							}
						}
					}
				}
			}
		}
	}

	var contents []map[string]interface{}
	for _, msg := range messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}

		// Keep track of the signature from a thinking block in the same message
		var lastSignature string
		var parts []map[string]interface{}
		switch v := msg.Content.(type) {
		case string:
			parts = append(parts, map[string]interface{}{"text": v})
		case []interface{}:
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					switch m["type"] {
					case "text":
						parts = append(parts, map[string]interface{}{"text": m["text"]})
					case "thinking":
						// Anthropic thinking 块 → Gemini thought part
						// signature 存储上一轮 Gemini 返回的 thoughtSignature，
						// 原样传回以维持多轮对话中的思考连贯性（Gemini 校验签名匹配）
						thinkText, _ := m["thinking"].(string)
						sig, _ := m["signature"].(string)
						if sig != "" {
							lastSignature = sig
						}
						thoughtPart := map[string]interface{}{
							"text":    thinkText,
							"thought": true,
						}
						if sig != "" {
							thoughtPart["thoughtSignature"] = sig
						}
						parts = append(parts, thoughtPart)
					case "compaction":
						// compaction 块是 /compact 产生的历史摘要检查点
						// 将摘要内容作为普通文本传给 Gemini，让模型知晓之前对话的要点
						if content, ok := m["content"].(string); ok && content != "" {
							parts = append(parts, map[string]interface{}{"text": content})
						}
					case "redacted_thinking":
						// redacted_thinking 存储 Anthropic 加密的 blob（data 字段），Gemini 无对等概念
						// Gemini 通过 thoughtSignature 机制维持思考连贯性，无需加密 blob，安全丢弃
						continue
					case "image", "audio", "video", "media":
						if source, ok := m["source"].(map[string]interface{}); ok {
							if part := convertMediaSourceToVertexPart(source, ""); part != nil {
								parts = append(parts, part)
							}
						}
					case "document":
						// Anthropic document 类型主要用于 PDF（media_type: application/pdf）
						// Gemini 通过 inlineData（base64）或 fileData（URI）接收
						if source, ok := m["source"].(map[string]interface{}); ok {
							if part := convertMediaSourceToVertexPart(source, "application/pdf"); part != nil {
								parts = append(parts, part)
							}
						}
					case "tool_use":
						fc := map[string]interface{}{
							"name": m["name"],
							"args": m["input"],
						}
						partObj := map[string]interface{}{
							"functionCall": fc,
						}
						
						// Gemini 3.x 要求多轮 function calling 时在历史中带回 thoughtSignature，
						// 否则返回 400 "Function call is missing a thought_signature"。
						var thoughtSig string
						if id, ok := m["id"].(string); ok && id != "" {
							if sig, ok := toolThoughtSigCache.Load(id); ok {
								thoughtSig = sig.(string)
							} else if idx := strings.Index(id, "_sig_"); idx != -1 {
								// 如果缓存穿透（如网关重启），尝试从 toolID 中提取
								thoughtSig = id[idx+5:]
							}
						}
						
						// 回退1：使用同一个 message 中的前一个 thinking 块的 signature
						if thoughtSig == "" && lastSignature != "" {
							thoughtSig = lastSignature
						}
						// 回退2：若是 Gemini 3.x，必须提供 thoughtSignature，否则上游直接 400
						if thoughtSig == "" && isGemini3Model(model) {
							// 提供一个伪造的 signature，避免 Vertex 拒绝请求。
							thoughtSig = "00000000000000000000000000000000"
						}
						
						if thoughtSig != "" {
							partObj["thoughtSignature"] = thoughtSig
						}

						parts = append(parts, partObj)
					case "tool_result":
						toolUseID, _ := m["tool_use_id"].(string)
						name := toolMap[toolUseID]
						if name == "" {
							name = "unknown_function"
						}
						
						isError, _ := m["is_error"].(bool)
						
						var respContent map[string]interface{}
						if contentStr, ok := m["content"].(string); ok {
							if isError {
								contentStr = fmt.Sprintf("Error: %s", contentStr)
							}
							respContent = map[string]interface{}{"content": contentStr}
						} else if contentArr, ok := m["content"].([]interface{}); ok {
							var textContents []string
							for _, cItem := range contentArr {
								if cMap, ok := cItem.(map[string]interface{}); ok {
									if cMap["type"] == "text" {
										if textStr, ok := cMap["text"].(string); ok {
											textContents = append(textContents, textStr)
										}
									} else if t, ok := cMap["type"].(string); ok && (t == "image" || t == "document" || t == "audio" || t == "video" || t == "media") {
										// 多媒体块放在 functionResponse 之外作为独立 part，让 Gemini 能同时处理文本结果和媒体文件
										if source, ok := cMap["source"].(map[string]interface{}); ok {
											if part := convertMediaSourceToVertexPart(source, ""); part != nil {
												parts = append(parts, part)
											}
										}
									}
								}
							}
							
							combinedText := strings.Join(textContents, "\n")
							if isError {
								combinedText = fmt.Sprintf("Error: %s", combinedText)
							}
							respContent = map[string]interface{}{"content": combinedText}
						} else {
							// 非 string、非数组的 content：序列化为 JSON 字符串嵌入
							rawBytes, _ := json.Marshal(m["content"])
							contentStr := string(rawBytes)
							if isError {
								contentStr = fmt.Sprintf("Error: %s", contentStr)
							}
							respContent = map[string]interface{}{"content": contentStr}
						}
						
						parts = append(parts, map[string]interface{}{
							"functionResponse": map[string]interface{}{
								"name":     name,
								"response": respContent,
							},
						})
					}
				}
			}
		}
		
		if len(parts) == 0 {
			parts = append(parts, map[string]interface{}{"text": ""})
		}
		
		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": parts,
		})
	}
	// GEAP 要求 contents 中 user/model 严格交替。
	// Anthropic 客户端偶尔会连续发出同 role 的消息（如批量 tool_result、重试注入等），
	// 直接转发会触发 GEAP 400 "roles must alternate"。
	// 解决方案：将连续同 role 的消息合并为一条（parts 拼接）。
	if len(contents) > 1 {
		merged := []map[string]interface{}{contents[0]}
		for i := 1; i < len(contents); i++ {
			last := merged[len(merged)-1]
			curr := contents[i]
			if last["role"] == curr["role"] {
				lastParts, _ := last["parts"].([]map[string]interface{})
				currParts, _ := curr["parts"].([]map[string]interface{})
				last["parts"] = append(lastParts, currParts...)
			} else {
				merged = append(merged, curr)
			}
		}
		contents = merged
	}
	// GEAP 的第一条消息必须是 user 角色；若客户端发送了以 assistant 开头的历史，
	// 在前面插入一条空占位，避免 400 错误
	if len(contents) > 0 {
		if contents[0]["role"] == "model" {
			contents = append([]map[string]interface{}{
				{"role": "user", "parts": []map[string]interface{}{{"text": ""}}},
			}, contents...)
		}
	}
	// GEAP 的最后一条消息必须是 user 角色。若客户端发送了以 assistant 结尾的历史（如 Claude Code 的 Assistant Prefill），
	// Gemini 会认为 model 已经发言完毕从而返回空响应。在末尾插入一条占位 user 消息促使模型继续生成。
	if len(contents) > 0 {
		if contents[len(contents)-1]["role"] == "model" {
			contents = append(contents, map[string]interface{}{
				"role": "user",
				"parts": []map[string]interface{}{{"text": "Please continue."}},
			})
		}
	}

	vertexReq["contents"] = contents

	genConfig := make(map[string]interface{})
	if req.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		genConfig["topP"] = *req.TopP
	}
	if req.TopK != nil {
		genConfig["topK"] = *req.TopK
	}
	if len(req.StopSequences) > 0 {
		genConfig["stopSequences"] = req.StopSequences
	}
	// 扩展思考映射：Anthropic thinking.budget_tokens → Gemini thinkingConfig
	// Gemini 2.5：thinkingBudget（整数 token 数）
	// Gemini 3.x：thinkingLevel（LOW/MEDIUM/HIGH），两者不可同时使用（API 会返回 400）
	// includeThoughts:true 让 Gemini 在响应中返回 thought 标记的 parts，
	// 流式/非流式处理器会将其转换为 Anthropic thinking 内容块 + signature_delta
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		if isGemini3Model(model) {
			// Gemini 3.x：使用 thinkingLevel 枚举，不接受 thinkingBudget 整数
			genConfig["thinkingConfig"] = map[string]interface{}{
				"includeThoughts": true,
				"thinkingMode":    budgetToThinkingLevel(req.Thinking.BudgetTokens),
			}
		} else {
			// Gemini 2.5：使用 thinkingBudget 整数
			thinkingCfg := map[string]interface{}{
				"includeThoughts": true,
			}
			if req.Thinking.BudgetTokens > 0 {
				thinkingCfg["thinkingBudget"] = req.Thinking.BudgetTokens
			}
			genConfig["thinkingConfig"] = thinkingCfg
		}
	}
	if len(genConfig) > 0 {
		vertexReq["generationConfig"] = genConfig
	}

	if len(req.Tools) > 0 {
		var functionDeclarations []map[string]interface{}
		for _, t := range req.Tools {
			if t.Type != "" {
				// Anthropic 内置工具（bash、text_editor、computer、web_search 等）在 Gemini 侧无对等执行环境，
				// 但 Claude Code 依赖这些工具的函数签名来完成工具调用循环。
				// 将其转换为普通 functionDeclaration 透传，让 LLM 能看到函数签名并做出调用决策。
				if decl := builtinToolToFunctionDecl(t); decl != nil {
					functionDeclarations = append(functionDeclarations, decl)
				}
				continue
			}
			var params map[string]interface{}
			if t.InputSchema != nil {
				params = sanitizeSchema(t.InputSchema)
			}
			decl := map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			}
			functionDeclarations = append(functionDeclarations, decl)
		}
		if len(functionDeclarations) > 0 {
			vertexReq["tools"] = []map[string]interface{}{
				{
					"functionDeclarations": functionDeclarations,
				},
			}
			// Gemini 2.5 自动 thinking（未显式配置 thinkingConfig）与 function calling 存在已知冲突：
			// 模型在无法输出原生 thought 时，会生成 name="thought" 的非法 functionCall，导致 MALFORMED_FUNCTION_CALL。
			// 由于 Gemini 2.5 Pro 不支持 thinkingBudget=0 来禁用思考，
			// 我们统一设置 includeThoughts: true 允许原生思考，流式处理器会自动将其转换为 thinking 块，
			// 从而避免模型尝试伪造 functionCall 引发崩溃。
			if _, hasExplicitThinking := genConfig["thinkingConfig"]; !hasExplicitThinking {
				genConfig["thinkingConfig"] = map[string]interface{}{"includeThoughts": true}
				vertexReq["generationConfig"] = genConfig
			}
		}
	}

	if req.ToolChoice != nil {
		mode := "AUTO"
		var allowedNames []string
		
		switch req.ToolChoice.Type {
		case "none":
			// Anthropic "none" = 不允许模型调用任何工具
			// 对应 GEAP functionCallingConfig.mode=NONE
			mode = "NONE"
		case "any":
			mode = "ANY"
		case "tool":
			mode = "ANY"
			if req.ToolChoice.Name != "" {
				allowedNames = []string{req.ToolChoice.Name}
			}
		}
		
		funcConfig := map[string]interface{}{
			"mode": mode,
		}
		if len(allowedNames) > 0 {
			funcConfig["allowedFunctionNames"] = allowedNames
		}
		// disable_parallel_tool_use → Gemini disableParallelFunctionCalls
		// 用于强制模型串行执行工具调用，避免并行调用导致副作用竞争
		if req.ToolChoice.DisableParallelToolUse {
			funcConfig["disableParallelFunctionCalls"] = true
		}

		vertexReq["toolConfig"] = map[string]interface{}{
			"functionCallingConfig": funcConfig,
		}
	}

	// safetySettings：默认对所有类别设置 BLOCK_NONE，防止 Gemini 安全过滤器误杀代理流量
	vertexReq["safetySettings"] = geapSafetySettings

	// labels：将 Anthropic metadata.user_id 映射为 GEAP 请求标签，便于计费与审计追踪
	// GEAP label 限制：key/value 最长 63 字符，仅允许小写字母、数字、下划线、连字符
	if req.Metadata != nil && req.Metadata.UserID != "" {
		sanitized := sanitizeLabelValue(req.Metadata.UserID)
		if sanitized != "" {
			vertexReq["labels"] = map[string]string{
				"user-id": sanitized,
			}
		}
	}

	return vertexReq, nil
}

// builtinToolToFunctionDecl 将 Anthropic 内置工具类型（bash、text_editor 等）转换为 Gemini functionDeclaration
// Claude Code 依赖这些工具的函数签名来完成工具调用循环。
// 由于 Gemini 无对等内置执行环境，仅保留函数签名供 LLM 做出调用决策，
// 实际执行由 Claude Code 在客户端完成，网关只负责透传调用/响应。
func builtinToolToFunctionDecl(t Tool) map[string]interface{} {
	name := t.Name
	desc := t.Description
	var params map[string]interface{}

	// 优先使用工具自带的 InputSchema
	if t.InputSchema != nil {
		params = sanitizeSchema(t.InputSchema)
	} else {
		// 无 schema 时为常见内置工具提供基础参数定义，确保 functionCall 格式合法
		switch {
		case strings.Contains(t.Type, "bash") || strings.Contains(name, "bash"):
			params = map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "STRING",
						"description": "The bash command to run",
					},
					"restart": map[string]interface{}{
						"type":        "BOOLEAN",
						"description": "Restart the bash shell session, clearing environment and history",
					},
					"timeout": map[string]interface{}{
						"type":        "INTEGER",
						"description": "Timeout in milliseconds for the command",
					},
				},
				"required": []interface{}{"command"},
			}
		case strings.Contains(t.Type, "str_replace_based_edit_tool") || strings.Contains(name, "str_replace_based_edit_tool"),
			strings.Contains(t.Type, "text_editor") || strings.Contains(name, "text_editor"):
			// text_editor_20250728 新增 view_range（view 命令的行范围）和 insert_text（insert 命令使用，替换旧版 new_str）
			// str_replace_based_edit_tool_20250124 / text_editor_20250124 命名等价，schema 相同
			isNewEditor := strings.Contains(t.Type, "20250728") || strings.Contains(name, "20250728")
			editorProps := map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "STRING",
					"description": "The command to run: view, create, str_replace, insert, undo_edit",
				},
				"path": map[string]interface{}{
					"type":        "STRING",
					"description": "Absolute path to file or directory",
				},
				"file_text": map[string]interface{}{
					"type":        "STRING",
					"description": "Required for create command — the new file content",
				},
				"old_str": map[string]interface{}{
					"type":        "STRING",
					"description": "Required for str_replace command — the text to be replaced",
				},
				"new_str": map[string]interface{}{
					"type":        "STRING",
					"description": "Required for str_replace command — the replacement text",
				},
				"insert_line": map[string]interface{}{
					"type":        "INTEGER",
					"description": "Required for insert command — line number after which to insert",
				},
			}
			if isNewEditor {
				editorProps["view_range"] = map[string]interface{}{
					"type":        "ARRAY",
					"description": "Optional [start_line, end_line] for view command",
					"items":       map[string]interface{}{"type": "INTEGER"},
				}
				editorProps["insert_text"] = map[string]interface{}{
					"type":        "STRING",
					"description": "Required for insert command — the text to insert (20250728+)",
				}
			}
			params = map[string]interface{}{
				"type":       "OBJECT",
				"properties": editorProps,
				"required":   []interface{}{"command", "path"},
			}
		case strings.Contains(t.Type, "computer") || strings.Contains(name, "computer"):
			// computer_20251124 新增 zoom 动作（带 region 参数）；20250124 起支持 scroll/drag/多键鼠标
			isNewComputer := strings.Contains(t.Type, "20251124") || strings.Contains(name, "20251124")
			actionDesc := "The computer action: screenshot, key, type, mouse_move, left_click, right_click, middle_click, double_click, triple_click, left_click_drag, left_mouse_down, left_mouse_up, scroll, hold_key, wait, cursor_position"
			if isNewComputer {
				actionDesc += ", zoom"
			}
			computerProps := map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "STRING",
					"description": actionDesc,
				},
				"coordinate": map[string]interface{}{
					"type":        "ARRAY",
					"description": "[x, y] pixel coordinates for mouse actions",
					"items":       map[string]interface{}{"type": "INTEGER"},
				},
				"text": map[string]interface{}{
					"type":        "STRING",
					"description": "Text to type or key sequence (e.g. 'Return', 'ctrl+c')",
				},
				"direction": map[string]interface{}{
					"type":        "STRING",
					"description": "Scroll direction: up, down, left, right",
				},
				"amount": map[string]interface{}{
					"type":        "INTEGER",
					"description": "Number of scroll clicks",
				},
				"start_coordinate": map[string]interface{}{
					"type":        "ARRAY",
					"description": "[x, y] drag start coordinates for left_click_drag",
					"items":       map[string]interface{}{"type": "INTEGER"},
				},
				"duration": map[string]interface{}{
					"type":        "NUMBER",
					"description": "Duration in seconds for hold_key or wait actions",
				},
			}
			if isNewComputer {
				computerProps["region"] = map[string]interface{}{
					"type":        "ARRAY",
					"description": "[x, y, width, height] region for zoom action (20251124+)",
					"items":       map[string]interface{}{"type": "INTEGER"},
				}
			}
			params = map[string]interface{}{
				"type":       "OBJECT",
				"properties": computerProps,
				"required":   []interface{}{"action"},
			}
		case strings.Contains(t.Type, "web_search") || strings.Contains(name, "web_search"):
			params = map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "STRING",
						"description": "The search query",
					},
					"explanation": map[string]interface{}{
						"type":        "STRING",
						"description": "One sentence explanation as to why this tool is being used",
					},
				},
				"required": []interface{}{"query", "explanation"},
			}
		case strings.Contains(t.Type, "web_fetch") || strings.Contains(name, "web_fetch"):
			// web_fetch_20250910 / web_fetch_20260209 — 抓取指定 URL 页面内容
			params = map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "STRING",
						"description": "The URL to fetch",
					},
					"prompt": map[string]interface{}{
						"type":        "STRING",
						"description": "Optional prompt describing what to extract from the page",
					},
					"max_length": map[string]interface{}{
						"type":        "INTEGER",
						"description": "Maximum number of characters to return from the response",
					},
					"raw": map[string]interface{}{
						"type":        "BOOLEAN",
						"description": "Return raw HTML/content instead of processed text",
					},
				},
				"required": []interface{}{"url"},
			}
		case strings.Contains(t.Type, "code_execution") || strings.Contains(name, "code_execution"):
			// code_execution_20250825 / code_execution_20260120 — 执行代码片段
			params = map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"language": map[string]interface{}{
						"type":        "STRING",
						"description": "Programming language: python, javascript, typescript, bash, etc.",
					},
					"code": map[string]interface{}{
						"type":        "STRING",
						"description": "The code to execute",
					},
					"session_id": map[string]interface{}{
						"type":        "STRING",
						"description": "Optional session ID to reuse an existing execution environment",
					},
					"timeout_ms": map[string]interface{}{
						"type":        "INTEGER",
						"description": "Timeout in milliseconds (default 10000)",
					},
				},
				"required": []interface{}{"language", "code"},
			}
		case strings.Contains(t.Type, "memory") || strings.Contains(name, "memory"):
			// memory_20250818 — 读写 /memories 文件，命令集类似 text_editor，
			// 额外支持 delete（删除文件）和 rename（重命名文件）
			params = map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "STRING",
						"description": "The command: view, create, str_replace, insert, delete, rename",
					},
					"path": map[string]interface{}{
						"type":        "STRING",
						"description": "Path within the memories store (always required)",
					},
					"file_text": map[string]interface{}{
						"type":        "STRING",
						"description": "Required for create command — initial file content",
					},
					"old_str": map[string]interface{}{
						"type":        "STRING",
						"description": "Required for str_replace — text to be replaced",
					},
					"new_str": map[string]interface{}{
						"type":        "STRING",
						"description": "Required for str_replace — replacement text",
					},
					"insert_line": map[string]interface{}{
						"type":        "INTEGER",
						"description": "Required for insert — line number after which to insert",
					},
					"insert_text": map[string]interface{}{
						"type":        "STRING",
						"description": "Required for insert — text to insert",
					},
					"new_path": map[string]interface{}{
						"type":        "STRING",
						"description": "Required for rename — the new path/name",
					},
				},
				"required": []interface{}{"command", "path"},
			}
		default:
			// 未知内置工具类型，生成通用 object 参数
			params = map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"input": map[string]interface{}{
						"type":        "OBJECT",
						"description": "Tool input parameters",
					},
				},
			}
		}
	}

	return map[string]interface{}{
		"name":        name,
		"description": desc,
		"parameters":  params,
	}
}

// sanitizeLabelValue 将任意字符串截断并清洗为合法的 GEAP label value
// GEAP 要求：小写字母、数字、下划线、连字符；最长 63 字符
func sanitizeLabelValue(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
		if b.Len() >= 63 {
			break
		}
	}
	result := strings.Trim(b.String(), "-")
	return result
}

// sanitizeSchema 递归清洗 JSON Schema，将所有字段透传，并专门针对 Vertex AI 的 Type 枚举限制处理混合类型和 nullable
func sanitizeSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}

	result := make(map[string]interface{})
	
	// 先拷贝所有属性实现全面透传，过滤 Gemini API 明确不支持的 JSON Schema 字段
	// 参考：https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest
	for k, v := range schema {
		switch k {
		case "$schema", "$ref", "$defs", "$id",
			"propertyNames", "exclusiveMinimum", "exclusiveMaximum",
			"additionalProperties",    // Gemini 不支持，传入会导致 400
			"unevaluatedProperties", "unevaluatedItems",
			"if", "then", "else", "not",        // 条件 schema 不支持
			"contentEncoding", "contentMediaType", // 内容编码不支持
			"patternProperties":                 // 不支持按模式匹配的属性定义
			continue
		case "const":
			// const 转为 enum 单值
			result["enum"] = []interface{}{v}
			continue
		}
		result[k] = v
	}

	// 递归处理嵌套属性
	if props, ok := result["properties"].(map[string]interface{}); ok {
		cleanProps := make(map[string]interface{})
		for pk, pv := range props {
			if propMap, ok := pv.(map[string]interface{}); ok {
				cleanProps[pk] = sanitizeSchema(propMap)
			} else {
				cleanProps[pk] = pv
			}
		}
		result["properties"] = cleanProps
	}

	if items, ok := result["items"].(map[string]interface{}); ok {
		result["items"] = sanitizeSchema(items)
	}

	// anyOf：递归清洗每个子 schema
	if arr, ok := result["anyOf"].([]interface{}); ok {
		cleanArr := make([]interface{}, 0, len(arr))
		for _, item := range arr {
			if itemMap, ok := item.(map[string]interface{}); ok {
				cleanArr = append(cleanArr, sanitizeSchema(itemMap))
			} else {
				cleanArr = append(cleanArr, item)
			}
		}
		result["anyOf"] = cleanArr
	}

	// oneOf → anyOf：Gemini 仅支持 anyOf，语义近似（结构上都是"任选其一"）
	if oneOfArr, ok := result["oneOf"].([]interface{}); ok {
		delete(result, "oneOf")
		cleanArr := make([]interface{}, 0, len(oneOfArr))
		for _, item := range oneOfArr {
			if itemMap, ok := item.(map[string]interface{}); ok {
				cleanArr = append(cleanArr, sanitizeSchema(itemMap))
			} else {
				cleanArr = append(cleanArr, item)
			}
		}
		if existing, ok := result["anyOf"].([]interface{}); ok {
			result["anyOf"] = append(existing, cleanArr...)
		} else {
			result["anyOf"] = cleanArr
		}
	}

	// allOf：Gemini 不支持，尝试将子 schema 的 properties/required/type 合并到当前层
	// 常见场景：{"allOf":[{"type":"object","properties":{...}},{"required":[...]}]}
	if allOfArr, ok := result["allOf"].([]interface{}); ok {
		delete(result, "allOf")
		for _, item := range allOfArr {
			if itemMap, ok := item.(map[string]interface{}); ok {
				cleaned := sanitizeSchema(itemMap)
				// 合并 properties
				if props, ok := cleaned["properties"].(map[string]interface{}); ok {
					if existProps, ok := result["properties"].(map[string]interface{}); ok {
						for pk, pv := range props {
							existProps[pk] = pv
						}
					} else {
						result["properties"] = props
					}
				}
				// 合并 required
				if req, ok := cleaned["required"].([]interface{}); ok {
					if existReq, ok := result["required"].([]interface{}); ok {
						result["required"] = append(existReq, req...)
					} else {
						result["required"] = req
					}
				}
				// 继承 type（当前层无 type 时）
				if _, hasType := result["type"]; !hasType {
					if t, ok := cleaned["type"]; ok {
						result["type"] = t
					}
				}
				// 继承 description（当前层无 description 时）
				if _, hasDesc := result["description"]; !hasDesc {
					if d, ok := cleaned["description"]; ok {
						result["description"] = d
					}
				}
			}
		}
	}

	// 自动推断缺失的 type（Vertex AI 强制要求属性必须有 type，除了 anyOf 场景）
	if _, hasType := result["type"]; !hasType {
		if _, hasAnyOf := result["anyOf"]; !hasAnyOf {
			if _, hasProps := result["properties"]; hasProps {
				result["type"] = "OBJECT"
			} else if _, hasItems := result["items"]; hasItems {
				result["type"] = "ARRAY"
			} else if _, hasEnum := result["enum"]; hasEnum {
				result["type"] = "STRING" // enum 兜底推断为字符串
			} else {
				// 兜底为 STRING 以防止 Vertex 报 400 "type must be specified"
				// 但要注意只对最内层的属性兜底，这里简单应用
				result["type"] = "STRING"
			}
		}
	}

	// 专门处理 Vertex AI 极其严格的 Type 字段限制
	if typeVal, ok := result["type"]; ok {
		if tStr, ok := typeVal.(string); ok {
			// 单一字符串类型：直接转为大写
			result["type"] = strings.ToUpper(tStr)
		} else if tArr, ok := typeVal.([]interface{}); ok {
			// Anthropic 的 JSON Schema 允许 type 是数组（如 ["string", "null"] 或 ["string", "number"]）
			// 但 Vertex AI 的 type 是一个强类型 Enum，绝对不支持数组！
			var types []string
			hasNull := false
			
			for _, item := range tArr {
				if ts, ok := item.(string); ok {
					if strings.ToLower(ts) == "null" {
						hasNull = true
					} else {
						types = append(types, strings.ToUpper(ts))
					}
				}
			}
			
			if hasNull {
				result["nullable"] = true
			}
			
			if len(types) == 1 {
				result["type"] = types[0]
			} else if len(types) > 1 {
				// 如果移除了 null 之后还有多个类型（如 ["string", "integer"]），
				// Vertex 只能通过 anyOf 来支持，必须删去当前级的 type 字段
				delete(result, "type")
				anyOfArr := make([]interface{}, 0, len(types))
				for _, t := range types {
					anyOfArr = append(anyOfArr, map[string]interface{}{"type": t})
				}
				result["anyOf"] = anyOfArr
			} else if len(types) == 0 && hasNull {
				// 如果原本只有 null
				result["type"] = "NULL"
			}
		}
	}

	return result
}

// convertMediaSourceToVertexPart 把 Anthropic 的媒体 source (base64/url) 转换为 Vertex AI 支持的 inlineData/fileData
func convertMediaSourceToVertexPart(source map[string]interface{}, defaultMediaType string) map[string]interface{} {
	if source == nil {
		return nil
	}
	mediaType, _ := source["media_type"].(string)
	if mediaType == "" {
		mediaType = defaultMediaType
	}
	if source["type"] == "base64" {
		return map[string]interface{}{
			"inlineData": map[string]interface{}{
				"mimeType": mediaType,
				"data":     source["data"],
			},
		}
	} else if source["type"] == "url" {
		return map[string]interface{}{
			"fileData": map[string]interface{}{
				"mimeType": mediaType,
				"fileUri":  source["url"],
			},
		}
	}
	return nil
}