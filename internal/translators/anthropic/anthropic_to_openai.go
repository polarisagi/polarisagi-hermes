package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"polaris-gateway/internal/router"
	"polaris-gateway/internal/translators/utils"
)

// Anthropic → OpenAI 协议转换器
// 完整支持 text、图片、tool_use/tool_result 内容块的双向转换
// 工具调用：Anthropic tools → OpenAI tools；流式/非流式响应中的 tool_calls → Anthropic tool_use 块

// oaiMessage OpenAI Chat Completions 消息格式
// content：assistant 工具调用时可为 null/空字符串，需保留 tool_calls 数组
// tool_call_id：role="tool" 时携带，对应触发本次结果的 tool_call.id
type oaiMessage struct {
	Role       string                   `json:"role"`
	Content    interface{}              `json:"content,omitempty"`
	ToolCalls  []map[string]interface{} `json:"tool_calls,omitempty"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
}

type oaiRequest struct {
	Model       string                   `json:"model"`
	Messages    []oaiMessage             `json:"messages"`
	MaxTokens   int                      `json:"max_tokens,omitempty"`
	Temperature *float64                 `json:"temperature,omitempty"`
	TopP        *float64                 `json:"top_p,omitempty"`
	Stream      bool                     `json:"stream,omitempty"`
	Tools           []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice      interface{}              `json:"tool_choice,omitempty"`
	ReasoningEffort string                   `json:"reasoning_effort,omitempty"`
}

// AnthropicToOpenAI 主入口：解析 Anthropic 请求 → 构造 OpenAI 请求 → 发送 → 流式/非流式回写 Anthropic 格式
func AnthropicToOpenAI(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) {
	// count_tokens 端点：OpenAI 协议无对等接口，本地估算返回
	if isCountTokensPath(r.URL.Path) {
		handleCountTokensLocal(w, bodyBytes, traceID)
		return
	}

	clientType := "Anthropic-Adapter"

	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type": "error", "error": {"type": "invalid_request_error", "message": "invalid json"}}`, 400)
		return
	}

	extractedBillingHeader := ExtractAndStripBillingHeader(&req)
	if extractedBillingHeader != "" {
		w.Header().Set("X-Anthropic-Billing-Header", extractedBillingHeader)
	}

	oaiReq := buildOpenAIRequest(req, dest)
	oaiBody, _ := json.Marshal(oaiReq)

	targetURL := strings.TrimSuffix(dest.Node.BaseURL, "/")
	if targetURL == "" {
		targetURL = "https://api.openai.com/v1"
	}
	targetURL = targetURL + "/chat/completions"

	if dest.IsProbationRun {
		slog.Warn("⚠️ 启用 🟠 Probation 账号执行流量探路 (Anthropic→OpenAI)", "trace_id", traceID, "account", dest.Node.Name)
	}

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(oaiBody))
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+dest.Node.Credentials)

	finalResp, err := httpClient.Do(proxyReq)
	if err != nil {
		utils.HandleNetworkError(w, err, dest, "openai", clientType, "anthropic_adapter", traceID, "Anthropic→OpenAI")
		return
	}

	isNodeFailure, isQuotaExhausted := utils.CheckResponseStatus(finalResp, dest, "openai", clientType, "anthropic_adapter", traceID, "Anthropic→OpenAI")

	if oaiReq.Stream {
		anthropicStreamOpenAI(ctx, w, finalResp, traceID, dest, clientType, oaiReq.Model, bodyBytes)
	} else {
		anthropicNonStreamOpenAI(w, finalResp, traceID, dest, clientType, oaiReq.Model, bodyBytes)
	}

	utils.FinalizeNodeState(dest, isNodeFailure, isQuotaExhausted, traceID)
}

// buildOpenAIRequest 把 Anthropic MessageRequest 转换为 OpenAI Chat Completions 请求
// 处理：模型映射、system prompt、messages（含 tool_use/tool_result）、tools、tool_choice
func buildOpenAIRequest(req MessageRequest, dest *router.MatchedDestination) oaiRequest {
	oaiReq := oaiRequest{
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	// 模型选择：路由 target 优先；客户端非 claude- 透传；否则报错（不应继续）
	if dest.TargetModel != "" {
		oaiReq.Model = dest.TargetModel
	} else if req.Model != "" && !strings.Contains(strings.ToLower(req.Model), "claude") {
		oaiReq.Model = req.Model
	}
	// 此处不再硬编码兜底为 "gemini-1.5-pro"——OpenAI 后端不一定有 Gemini，
	// 让上游返回错误更明确

	// system prompt → OpenAI system 消息
	if sysText := flattenAnthropicSystem(req.System); sysText != "" {
		oaiReq.Messages = append(oaiReq.Messages, oaiMessage{Role: "system", Content: sysText})
	}

	// 转换消息列表（含 tool_use / tool_result）
	oaiReq.Messages = append(oaiReq.Messages, convertAnthropicMessages(req.Messages)...)

	// 转换工具定义
	if len(req.Tools) > 0 {
		oaiReq.Tools = convertAnthropicTools(req.Tools)
	}

	// 转换工具选择策略
	if req.ToolChoice != nil {
		oaiReq.ToolChoice = convertAnthropicToolChoice(req.ToolChoice)
	}

	// 转换思考配置 (Claude Code /effort -> OpenAI reasoning_effort)
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		budget := req.Thinking.BudgetTokens
		if budget <= 5000 {
			oaiReq.ReasoningEffort = "low"
		} else if budget <= 16000 {
			oaiReq.ReasoningEffort = "medium"
		} else {
			oaiReq.ReasoningEffort = "high"
		}
	}

	return oaiReq
}

// flattenAnthropicSystem 把 Anthropic system（string 或 []Content）扁平化为单一字符串
func flattenAnthropicSystem(system interface{}) string {
	switch sys := system.(type) {
	case string:
		return sys
	case []interface{}:
		var sb strings.Builder
		for _, item := range sys {
			if m, ok := item.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						sb.WriteString(t)
					}
				}
			}
		}
		return sb.String()
	}
	return ""
}

// convertAnthropicTools Anthropic tools → OpenAI tools (type=function)
func convertAnthropicTools(tools []Tool) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(tools))
	for _, t := range tools {
		// input_schema 为空时给 OpenAI 一个合法的空对象 schema
		params := t.InputSchema
		if params == nil {
			params = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
		}
		result = append(result, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			},
		})
	}
	return result
}

// convertAnthropicToolChoice Anthropic tool_choice → OpenAI tool_choice
// auto → "auto"；any → "required"；tool(name=X) → {"type":"function","function":{"name":"X"}}
func convertAnthropicToolChoice(tc *ToolChoice) interface{} {
	switch tc.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		if tc.Name != "" {
			return map[string]interface{}{
				"type":     "function",
				"function": map[string]interface{}{"name": tc.Name},
			}
		}
		return "required"
	case "none":
		return "none"
	}
	return nil
}

// convertAnthropicMessages 把 Anthropic messages 转换为 OpenAI messages
// 关键拆分规则：
//   - assistant 消息含 text + tool_use 块时，合并为单条 OpenAI assistant 消息（content + tool_calls）
//   - user 消息含 tool_result 块时，每个 tool_result 拆为独立的 role=tool 消息
//   - 其他文本/图片块按原顺序作为 multi-part content
func convertAnthropicMessages(msgs []Message) []oaiMessage {
	var result []oaiMessage
	for _, msg := range msgs {
		switch v := msg.Content.(type) {
		case string:
			// 纯字符串内容，直接转
			result = append(result, oaiMessage{Role: msg.Role, Content: v})

		case []interface{}:
			// 分类收集本消息中的各类块
			var textParts []map[string]interface{}
			var toolCalls []map[string]interface{}
			var toolResults []oaiMessage // 拆出来的独立 tool 消息

			for _, item := range v {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				switch m["type"] {
				case "text":
					if t, ok := m["text"].(string); ok && t != "" {
						textParts = append(textParts, map[string]interface{}{
							"type": "text",
							"text": t,
						})
					}
				case "image", "document", "audio", "video", "media":
					if part := convertAnthropicMediaBlock(m); part != nil {
						textParts = append(textParts, part)
					}
				case "tool_use":
					// assistant 工具调用：转为 OpenAI tool_calls 项
					id, _ := m["id"].(string)
					name, _ := m["name"].(string)
					var argsStr string
					if input, ok := m["input"]; ok {
						argsBytes, _ := json.Marshal(input)
						argsStr = string(argsBytes)
					}
					if argsStr == "" || argsStr == "null" {
						argsStr = "{}"
					}
					toolCalls = append(toolCalls, map[string]interface{}{
						"id":   id,
						"type": "function",
						"function": map[string]interface{}{
							"name":      name,
							"arguments": argsStr,
						},
					})
				case "tool_result":
					// 用户回填的工具结果：拆为独立 role=tool 消息
					toolUseID, _ := m["tool_use_id"].(string)
					toolResults = append(toolResults, oaiMessage{
						Role:       "tool",
						ToolCallID: toolUseID,
						Content:    flattenToolResultContent(m),
					})
				}
			}

			// 组装本条消息
			// 情况 A：assistant + tool_use → 一条 oaiMessage，content + tool_calls
			// 情况 B：user + tool_result → 拆出的多条 role=tool 消息（已收集）
			// 情况 C：普通文本/图片 → 一条 oaiMessage 含 content
			// 情况 D：仅含 thinking/redacted_thinking 块（已全部被过滤）→ 空占位，保持对话结构完整
			if len(toolCalls) > 0 {
				// assistant 工具调用：合并文本与 tool_calls
				m := oaiMessage{Role: msg.Role, ToolCalls: toolCalls}
				if len(textParts) > 0 {
					m.Content = collapseTextParts(textParts)
				}
				result = append(result, m)
			} else if len(toolResults) > 0 {
				// 先把同消息中的非 tool_result 文本拼到第一条 tool 消息前（罕见，但合法）
				if len(textParts) > 0 {
					result = append(result, oaiMessage{Role: msg.Role, Content: collapseTextParts(textParts)})
				}
				result = append(result, toolResults...)
			} else if len(textParts) > 0 {
				result = append(result, oaiMessage{Role: msg.Role, Content: collapseTextParts(textParts)})
			} else if msg.Role == "assistant" {
				// 所有块均为 thinking/redacted_thinking（OpenAI 不支持，已过滤），
				// 保留空 assistant 消息以维持对话轮次结构，避免 OpenAI 后端因缺少 assistant 历史而报错
				result = append(result, oaiMessage{Role: "assistant", Content: ""})
			}
		}
	}
	return result
}

// convertAnthropicMediaBlock 把 image/document/etc. 块转为 OpenAI multi-part image_url 项
func convertAnthropicMediaBlock(m map[string]interface{}) map[string]interface{} {
	source, ok := m["source"].(map[string]interface{})
	if !ok {
		return nil
	}
	var url string
	if source["type"] == "base64" {
		mediaType, _ := source["media_type"].(string)
		data, _ := source["data"].(string)
		url = fmt.Sprintf("data:%s;base64,%s", mediaType, data)
	} else if source["type"] == "url" {
		url, _ = source["url"].(string)
	}
	if url == "" {
		return nil
	}
	return map[string]interface{}{
		"type":      "image_url",
		"image_url": map[string]interface{}{"url": url},
	}
}

// collapseTextParts 单文本块塌缩为字符串，多块/图文混合保留 multi-part 数组
// OpenAI 兼容服务对纯文本字符串支持更普遍，避免不必要的 multi-part 包装
func collapseTextParts(parts []map[string]interface{}) interface{} {
	if len(parts) == 1 && parts[0]["type"] == "text" {
		if t, ok := parts[0]["text"].(string); ok {
			return t
		}
	}
	return parts
}

// flattenToolResultContent 把 Anthropic tool_result.content（string 或 [Content]）扁平化为字符串
// OpenAI role=tool 消息要求 content 是字符串
func flattenToolResultContent(m map[string]interface{}) string {
	switch c := m["content"].(type) {
	case string:
		return c
	case []interface{}:
		var sb strings.Builder
		for _, item := range c {
			if cm, ok := item.(map[string]interface{}); ok {
				if cm["type"] == "text" {
					if t, ok := cm["text"].(string); ok {
						sb.WriteString(t)
					}
				}
			}
		}
		return sb.String()
	}
	return ""
}

// anthropicStreamOpenAI 流式：读 OpenAI SSE → 转 Anthropic SSE
// 状态机管理多种内容块（text、tool_use）的开/关：
//   - 文本增量：blockIndex=0 的 text block，复用 content_block_delta (text_delta)
//   - 工具调用：每个 OpenAI tool_call.index 映射一个独立的 Anthropic block，
//     首次出现时 emit content_block_start (tool_use)，后续 arguments 增量
//     emit content_block_delta (input_json_delta)
func anthropicStreamOpenAI(ctx context.Context, w http.ResponseWriter, oaiResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string, reqBody []byte) {
	defer oaiResp.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	// 用估算 input_tokens 填充 message_start，让 /context 首事件就显示进度
	var parsedReq MessageRequest
	_ = json.Unmarshal(reqBody, &parsedReq)
	estimatedInput := estimateAnthropicTokens(parsedReq)
	writeSSEMessageStart(w, flusher, traceID, modelName, estimatedInput)

	// 状态机：
	//   - textOpen 标记是否已 emit text content_block_start
	//   - thinkingOpen 标记是否已 emit thinking content_block_start
	//   - toolBlocks 映射 OpenAI tool_call.index → Anthropic block index
	//   - nextBlockIndex 下一个可用 Anthropic block index
	textOpen := false
	textBlockIndex := 0
	thinkingOpen := false
	thinkingBlockIndex := 0
	toolBlocks := make(map[int]int)
	nextBlockIndex := 0
	stopReason := "end_turn"

	buf := make([]byte, 32*1024)
	var tailBuf []byte
	const tailWindowSize = 8192
	var totalWritten int64

	// 跨 chunk 行缓存：OpenAI SSE 一行可能跨多个 read 边界
	var lineBuf bytes.Buffer

	for {
		if ctx.Err() != nil {
			slog.Debug("🔌 [Stream] 客户端已断开，终止 OpenAI 流式响应", "trace_id", traceID, "account", dest.Node.Name)
			return
		}

		n, readErr := oaiResp.Body.Read(buf)
		if n > 0 {
			totalWritten += int64(n)
			chunk := buf[:n]
			tailBuf = append(tailBuf, chunk...)
			if len(tailBuf) > tailWindowSize {
				tailBuf = tailBuf[len(tailBuf)-tailWindowSize:]
			}

			lineBuf.Write(chunk)
			for {
				idx := bytes.IndexByte(lineBuf.Bytes(), '\n')
				if idx < 0 {
					break
				}
				line := make([]byte, idx)
				copy(line, lineBuf.Bytes()[:idx])
				lineBuf.Next(idx + 1)
				processOAIStreamLine(line, w, flusher, &textOpen, &textBlockIndex, &thinkingOpen, &thinkingBlockIndex, toolBlocks, &nextBlockIndex, &stopReason)
			}
		}
		if readErr != nil {
			break
		}
	}
	// 处理 buffer 残留（最后一行可能无换行）
	if rest := lineBuf.Bytes(); len(rest) > 0 {
		processOAIStreamLine(rest, w, flusher, &textOpen, &textBlockIndex, &thinkingOpen, &thinkingBlockIndex, toolBlocks, &nextBlockIndex, &stopReason)
	}

	// 关闭所有打开的 content block
	if thinkingOpen {
		writeSSE(w, flusher, "content_block_delta", StreamEvent{
			Type:  "content_block_delta",
			Index: ptrInt(thinkingBlockIndex),
			Delta: &Delta{Type: "signature_delta", Signature: "oai_reasoning"},
		})
		writeSSEContentBlockStop(w, flusher, thinkingBlockIndex)
	}
	if textOpen {
		writeSSEContentBlockStop(w, flusher, textBlockIndex)
	}
	for _, blockIdx := range toolBlocks {
		writeSSEContentBlockStop(w, flusher, blockIdx)
	}

	// 解析 usage
	prompt, completion, cached, found := utils.ParseUsageFromStreamTail(tailBuf)
	if !found {
		prompt = utils.EstimatePromptTokens(reqBody)
		completion = utils.EstimateCompletionTokens(totalWritten)
		slog.Warn("⚠️ 响应流中断，启用 token 估算补偿", "trace_id", traceID, "node", dest.Node.Name, "prompt", prompt, "completion", completion)
	}

	// message_delta：携带最终 stop_reason 与 usage
	msgDeltaEvent := StreamEvent{
		Type:  "message_delta",
		Delta: &Delta{StopReason: stopReason},
		Usage: &Usage{
			InputTokens:  int(prompt),
			OutputTokens: int(completion),
		},
	}
	writeSSE(w, flusher, "message_delta", msgDeltaEvent)
	writeSSEMessageStop(w, flusher)

	settleBilling("openai", dest.Node.Name, clientType, "anthropic_adapter", modelName, prompt, completion, cached, oaiResp.StatusCode, dest, reqBody, traceID)
}

// processOAIStreamLine 解析单行 OpenAI SSE 数据，emit 对应 Anthropic SSE 事件
// 出参通过指针修改：textOpen、nextBlockIndex、stopReason
// toolBlocks 是引用类型可直接修改
func processOAIStreamLine(line []byte, w http.ResponseWriter, flusher http.Flusher, textOpen *bool, textBlockIndex *int, thinkingOpen *bool, thinkingBlockIndex *int, toolBlocks map[int]int, nextBlockIndex *int, stopReason *string) {
	line = bytes.TrimSpace(line)
	if !bytes.HasPrefix(line, []byte("data: ")) {
		return
	}
	data := bytes.TrimPrefix(line, []byte("data: "))
	if string(data) == "[DONE]" {
		return
	}

	var chunkJSON map[string]interface{}
	if err := json.Unmarshal(data, &chunkJSON); err != nil {
		return
	}

	choices, ok := chunkJSON["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return
	}
	choice, _ := choices[0].(map[string]interface{})

	// finish_reason → Anthropic stop_reason 映射
	if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
		switch fr {
		case "stop":
			// 保持默认 end_turn
		case "length":
			*stopReason = "max_tokens"
		case "tool_calls", "function_call":
			*stopReason = "tool_use"
		case "content_filter":
			*stopReason = "end_turn"
		}
	}

	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return
	}

	// 推理内容增量 (OpenAI o1/o3-mini reasoning_content)
	if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
		if *textOpen {
			// 如果文本块已开，规范上不应再有 thinking，但为了防御性编程，先关掉文本块
			writeSSEContentBlockStop(w, flusher, *textBlockIndex)
			*textOpen = false
		}
		if !*thinkingOpen {
			*thinkingBlockIndex = *nextBlockIndex
			*nextBlockIndex++
			writeSSE(w, flusher, "content_block_start", StreamEvent{
				Type:         "content_block_start",
				Index:        ptrInt(*thinkingBlockIndex),
				ContentBlock: &Content{Type: "thinking", Thinking: ""},
			})
			*thinkingOpen = true
		}
		writeSSE(w, flusher, "content_block_delta", StreamEvent{
			Type:  "content_block_delta",
			Index: ptrInt(*thinkingBlockIndex),
			Delta: &Delta{Type: "thinking_delta", Thinking: reasoning},
		})
	}

	// 文本增量
	if content, ok := delta["content"].(string); ok && content != "" {
		if *thinkingOpen {
			// 关闭前先发签名
			writeSSE(w, flusher, "content_block_delta", StreamEvent{
				Type:  "content_block_delta",
				Index: ptrInt(*thinkingBlockIndex),
				Delta: &Delta{Type: "signature_delta", Signature: "oai_reasoning"},
			})
			writeSSEContentBlockStop(w, flusher, *thinkingBlockIndex)
			*thinkingOpen = false
		}
		if !*textOpen {
			*textBlockIndex = *nextBlockIndex
			*nextBlockIndex++
			// emit text content_block_start
			writeSSE(w, flusher, "content_block_start", StreamEvent{
				Type:         "content_block_start",
				Index:        ptrInt(*textBlockIndex),
				ContentBlock: &Content{Type: "text", Text: ""},
			})
			*textOpen = true
		}
		writeSSE(w, flusher, "content_block_delta", StreamEvent{
			Type:  "content_block_delta",
			Index: ptrInt(*textBlockIndex),
			Delta: &Delta{Type: "text_delta", Text: content},
		})
	}

	// 工具调用增量
	toolCalls, _ := delta["tool_calls"].([]interface{})
	for _, tcIfc := range toolCalls {
		tc, ok := tcIfc.(map[string]interface{})
		if !ok {
			continue
		}
		oaiIndex := 0
		if idxF, ok := tc["index"].(float64); ok {
			oaiIndex = int(idxF)
		}

		// 首次见到该 index：emit content_block_start (tool_use)
		blockIdx, exists := toolBlocks[oaiIndex]
		if !exists {
			// 文本块或思考块若已开，需要先关掉再开 tool_use（Anthropic 协议要求块按顺序开关）
			if *thinkingOpen {
				writeSSE(w, flusher, "content_block_delta", StreamEvent{
					Type:  "content_block_delta",
					Index: ptrInt(*thinkingBlockIndex),
					Delta: &Delta{Type: "signature_delta", Signature: "oai_reasoning"},
				})
				writeSSEContentBlockStop(w, flusher, *thinkingBlockIndex)
				*thinkingOpen = false
			}
			if *textOpen {
				writeSSEContentBlockStop(w, flusher, *textBlockIndex)
				*textOpen = false
			}

			if *nextBlockIndex < 1 {
				*nextBlockIndex = 1
			}
			blockIdx = *nextBlockIndex
			toolBlocks[oaiIndex] = blockIdx
			*nextBlockIndex++

			toolID, _ := tc["id"].(string)
			var toolName string
			if fn, ok := tc["function"].(map[string]interface{}); ok {
				toolName, _ = fn["name"].(string)
			}
			if toolID == "" {
				toolID = fmt.Sprintf("toolu_%d", blockIdx)
			}

			writeSSE(w, flusher, "content_block_start", StreamEvent{
				Type:  "content_block_start",
				Index: ptrInt(blockIdx),
				ContentBlock: &Content{
					Type:  "tool_use",
					ID:    toolID,
					Name:  toolName,
					Input: struct{}{}, // 确保序列化为 {}
				},
			})
		}

		// arguments 增量 → input_json_delta
		if fn, ok := tc["function"].(map[string]interface{}); ok {
			if args, ok := fn["arguments"].(string); ok && args != "" {
				writeSSE(w, flusher, "content_block_delta", StreamEvent{
					Type:  "content_block_delta",
					Index: ptrInt(blockIdx),
					Delta: &Delta{Type: "input_json_delta", PartialJson: args},
				})
			}
		}
	}
}

// anthropicNonStreamOpenAI 非流式响应：解析 OpenAI message.tool_calls 转为 Anthropic Content tool_use
func anthropicNonStreamOpenAI(w http.ResponseWriter, oaiResp *http.Response, traceID string, dest *router.MatchedDestination, clientType, modelName string, reqBody []byte) {
	defer oaiResp.Body.Close()

	var oaiResponse struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(oaiResp.Body).Decode(&oaiResponse); err != nil {
		http.Error(w, "Failed to parse response", http.StatusBadGateway)
		return
	}

	stopReason := "end_turn"
	var contents []Content

	if len(oaiResponse.Choices) > 0 {
		choice := oaiResponse.Choices[0]

		// 推理内容
		if choice.Message.ReasoningContent != "" {
			contents = append(contents, Content{
				Type:      "thinking",
				Thinking:  choice.Message.ReasoningContent,
				Signature: "oai_reasoning",
			})
		}

		// 文本内容
		if choice.Message.Content != "" {
			contents = append(contents, Content{Type: "text", Text: choice.Message.Content})
		}

		// 工具调用
		for _, tc := range choice.Message.ToolCalls {
			var input interface{}
			if tc.Function.Arguments != "" {
				// arguments 是 JSON 字符串，反序列化以便 Anthropic Content.Input 字段填 map
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
					input = map[string]interface{}{} // 解析失败给空对象，避免序列化崩溃
				}
			} else {
				input = map[string]interface{}{}
			}
			id := tc.ID
			if id == "" {
				id = fmt.Sprintf("toolu_%s_%d", traceID, len(contents))
			}
			contents = append(contents, Content{
				Type:  "tool_use",
				ID:    id,
				Name:  tc.Function.Name,
				Input: input,
			})
		}

		// finish_reason 映射
		switch choice.FinishReason {
		case "length":
			stopReason = "max_tokens"
		case "tool_calls", "function_call":
			stopReason = "tool_use"
		}
	}

	if len(contents) == 0 {
		// 防止 content:[] 被 Claude Code 视为空消息
		contents = []Content{{Type: "text", Text: ""}}
	}

	promptTokens := int64(oaiResponse.Usage.PromptTokens)
	completionTokens := int64(oaiResponse.Usage.CompletionTokens)
	settleBilling("openai", dest.Node.Name, clientType, "anthropic_adapter", modelName, promptTokens, completionTokens, 0, oaiResp.StatusCode, dest, reqBody, traceID)

	anthropicResp := MessageResponse{
		ID:           fmt.Sprintf("msg_%s", traceID),
		Type:         "message",
		Role:         "assistant",
		Model:        modelName,
		StopReason:   stopReason,
		StopSequence: "",
		Usage: Usage{
			InputTokens:  oaiResponse.Usage.PromptTokens,
			OutputTokens: oaiResponse.Usage.CompletionTokens,
		},
		Content: contents,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(oaiResp.StatusCode)
	_ = json.NewEncoder(w).Encode(anthropicResp)
}

func init() {
	router.RegisterTranslator("anthropic", "openai", AnthropicToOpenAI)
}
