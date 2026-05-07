package anthropic

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
			// Simplistic handling of content blocks for now
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					if m["type"] == "text" {
						parts = append(parts, map[string]interface{}{"text": m["text"]})
					}
				}
			}
		}
		
		// Optional: handling for empty parts if needed, though Anthropic typically doesn't send empty content
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
