// Anthropic → Vertex 请求映射器
// 将 Anthropic Messages API 格式转换为 Vertex GenerateContent API 格式
package anthropic

import (
	"fmt"
	"strings"
)

// mapToVertexRequest 将 Anthropic Messages 请求转换为 Vertex 原生的 generateContent 请求体
// 转换规则:
//   - Anthropic system → Vertex systemInstruction
//   - Anthropic user/assistant → Vertex user/model 角色
//   - Anthropic 纯文本/多模态/工具调用内容块 → Vertex parts 数组
//   - Anthropic max_tokens → Vertex maxOutputTokens
//   - Anthropic temperature/topP/topK → Vertex generationConfig
//   - Anthropic tools → Vertex tools (functionDeclarations)
//   - Anthropic tool_choice → Vertex toolConfig
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
					case "image", "document", "audio", "video", "media":
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
									} else if cMap["type"] == "image" {
										if source, ok := cMap["source"].(map[string]interface{}); ok {
											if source["type"] == "base64" {
												parts = append(parts, map[string]interface{}{
													"inlineData": map[string]interface{}{
														"mimeType": source["media_type"],
														"data":     source["data"],
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
	if len(genConfig) > 0 {
		vertexReq["generationConfig"] = genConfig
	}

	if len(req.Tools) > 0 {
		var functionDeclarations []map[string]interface{}
		for _, t := range req.Tools {
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
		vertexReq["tools"] = []map[string]interface{}{
			{
				"functionDeclarations": functionDeclarations,
			},
		}
	}

	if req.ToolChoice != nil {
		mode := "AUTO"
		var allowedNames []string
		
		switch req.ToolChoice.Type {
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
		
		vertexReq["toolConfig"] = map[string]interface{}{
			"functionCallingConfig": funcConfig,
		}
	}

	return vertexReq, nil
}

// sanitizeSchema 递归清洗 JSON Schema，将所有字段透传，并专门针对 Vertex AI 的 Type 枚举限制处理混合类型和 nullable
func sanitizeSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}

	result := make(map[string]interface{})
	
	// 先拷贝所有属性实现全面透传，但要过滤掉 Vertex API 明确不支持的 JSON Schema 字段
	for k, v := range schema {
		if k == "$schema" || k == "propertyNames" || k == "const" || k == "exclusiveMinimum" {
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

	for _, compKey := range []string{"anyOf", "allOf", "oneOf"} {
		if arr, ok := result[compKey].([]interface{}); ok {
			cleanArr := make([]interface{}, 0, len(arr))
			for _, item := range arr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					cleanArr = append(cleanArr, sanitizeSchema(itemMap))
				} else {
					cleanArr = append(cleanArr, item)
				}
			}
			result[compKey] = cleanArr
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