// Anthropic → Vertex 请求映射器
// 将 Anthropic Messages API 格式转换为 Vertex GenerateContent API 格式
package anthropic

// mapToVertexRequest 将 Anthropic Messages 请求转换为 Vertex 原生的 generateContent 请求体
// 转换规则:
//   - Anthropic system → Vertex systemInstruction
//   - Anthropic user/assistant → Vertex user/model 角色
//   - Anthropic 纯文本/多模态内容块 → Vertex parts 数组
//   - Anthropic max_tokens → Vertex maxOutputTokens
//   - Anthropic temperature/topP/topK → Vertex generationConfig
func mapToVertexRequest(req MessageRequest) (map[string]interface{}, error) {
	vertexReq := make(map[string]interface{})

	if req.System != "" {
		vertexReq["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": req.System},
			},
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
					if m["type"] == "text" {
						parts = append(parts, map[string]interface{}{"text": m["text"]})
					} else if m["type"] == "image" {
						if source, ok := m["source"].(map[string]interface{}); ok {
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
	vertexReq["generationConfig"] = genConfig

	return vertexReq, nil
}