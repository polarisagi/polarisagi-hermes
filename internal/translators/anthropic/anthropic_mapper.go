// Anthropic → Google Agent Platform 请求映射器
// 将 Anthropic Messages API 格式转换为 GEAP GenerateContent API 格式
package anthropic

import (
	"fmt"
	"strings"
)

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

// mapToVertexRequest 将 Anthropic Messages 请求转换为 GEAP 原生的 generateContent 请求体
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
func mapToVertexRequest(req MessageRequest) (map[string]interface{}, error) {
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

	// Build a map of tool_use_id to tool name for mapping tool_results later
	toolMap := make(map[string]string)
	for _, msg := range req.Messages {
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
	for _, msg := range req.Messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}

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
						thoughtPart := map[string]interface{}{
							"text":    thinkText,
							"thought": true,
						}
						if sig != "" {
							thoughtPart["thoughtSignature"] = sig
						}
						parts = append(parts, thoughtPart)
					case "redacted_thinking":
						// redacted_thinking 存储 Anthropic 加密的 blob（data 字段），Gemini 无对等概念
						// Gemini 通过 thoughtSignature 机制维持思考连贯性，无需加密 blob，安全丢弃
						continue
					case "image", "audio", "video", "media":
						if source, ok := m["source"].(map[string]interface{}); ok {
							if source["type"] == "base64" {
								parts = append(parts, map[string]interface{}{
									"inlineData": map[string]interface{}{
										"mimeType": source["media_type"],
										"data":     source["data"],
									},
								})
							} else if source["type"] == "url" {
								parts = append(parts, map[string]interface{}{
									"fileData": map[string]interface{}{
										"mimeType": source["media_type"],
										"fileUri":  source["url"],
									},
								})
							}
						}
					case "document":
						// Anthropic document 类型主要用于 PDF（media_type: application/pdf）
						// Gemini 通过 inlineData（base64）或 fileData（URI）接收
						if source, ok := m["source"].(map[string]interface{}); ok {
							mediaType, _ := source["media_type"].(string)
							if mediaType == "" {
								mediaType = "application/pdf"
							}
							if source["type"] == "base64" {
								parts = append(parts, map[string]interface{}{
									"inlineData": map[string]interface{}{
										"mimeType": mediaType,
										"data":     source["data"],
									},
								})
							} else if source["type"] == "url" {
								parts = append(parts, map[string]interface{}{
									"fileData": map[string]interface{}{
										"mimeType": mediaType,
										"fileUri":  source["url"],
									},
								})
							}
						}
					case "tool_use":
						parts = append(parts, map[string]interface{}{
							"functionCall": map[string]interface{}{
								"name": m["name"],
								"args": m["input"],
							},
							"thoughtSignature": "skip_thought_signature_validator",
						})
					case "tool_result":
						toolUseID, _ := m["tool_use_id"].(string)
						name := toolMap[toolUseID]
						if name == "" {
							name = "unknown_function"
						}
						
						isError, _ := m["is_error"].(bool)
						
						// Vertex functionResponse expects a JSON object
						var respContent interface{}
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
										if source, ok := cMap["source"].(map[string]interface{}); ok {
											if source["type"] == "base64" {
												parts = append(parts, map[string]interface{}{
													"inlineData": map[string]interface{}{
														"mimeType": source["media_type"],
														"data":     source["data"],
													},
												})
											} else if source["type"] == "url" {
												parts = append(parts, map[string]interface{}{
													"fileData": map[string]interface{}{
														"mimeType": source["media_type"],
														"fileUri":  source["url"],
													},
												})
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
							if isError {
								respContent = map[string]interface{}{"error": true, "content": m["content"]}
							} else {
								respContent = map[string]interface{}{"content": m["content"]}
							}
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
	// includeThoughts:true 让 Gemini 在响应中返回 thought 标记的 parts，
	// 流式处理器会将其转换为 Anthropic thinking 内容块
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		thinkingCfg := map[string]interface{}{
			"includeThoughts": true,
		}
		if req.Thinking.BudgetTokens > 0 {
			thinkingCfg["thinkingBudget"] = req.Thinking.BudgetTokens
		}
		genConfig["thinkingConfig"] = thinkingCfg
	}
	if len(genConfig) > 0 {
		vertexReq["generationConfig"] = genConfig
	}

	if len(req.Tools) > 0 {
		var functionDeclarations []map[string]interface{}
		for _, t := range req.Tools {
			// 跳过 Anthropic 内置工具类型（computer use / bash / text_editor / web_search）
			// 这些工具在 Gemini 侧无对等 functionDeclaration，强行透传会导致 400
			if t.Type != "" {
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
		// 仅在有有效的 functionDeclarations 时才设置 tools 字段
		// 若所有 tool 都是内置类型（Type != ""）被跳过，不发送空的 tools 数组
		if len(functionDeclarations) > 0 {
			vertexReq["tools"] = []map[string]interface{}{
				{
					"functionDeclarations": functionDeclarations,
				},
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