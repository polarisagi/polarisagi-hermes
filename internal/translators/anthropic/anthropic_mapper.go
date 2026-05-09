// Anthropic → Vertex 请求映射器
// 将 Anthropic Messages API 格式转换为 Vertex GenerateContent API 格式
package anthropic

import "strings"

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
						})
					case "tool_result":
						toolUseID, _ := m["tool_use_id"].(string)
						name := toolMap[toolUseID]
						if name == "" {
							name = "unknown_function"
						}
						
						// Vertex functionResponse expects a JSON object
						var respContent interface{}
						if contentStr, ok := m["content"].(string); ok {
							respContent = map[string]interface{}{"content": contentStr}
						} else {
							respContent = map[string]interface{}{"content": m["content"]}
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
			if t.InputSchema != nil {
				uppercaseTypeFields(t.InputSchema)
			}
			decl := map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
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

// uppercaseTypeFields 递归地将 JSON Schema 中的 type 字段转换为大写，以满足 Vertex API 的要求
func uppercaseTypeFields(schema map[string]interface{}) {
	if t, ok := schema["type"].(string); ok {
		schema["type"] = strings.ToUpper(t)
	}
	if props, ok := schema["properties"].(map[string]interface{}); ok {
		for _, v := range props {
			if propMap, ok := v.(map[string]interface{}); ok {
				uppercaseTypeFields(propMap)
			}
		}
	}
	if items, ok := schema["items"].(map[string]interface{}); ok {
		uppercaseTypeFields(items)
	}
}