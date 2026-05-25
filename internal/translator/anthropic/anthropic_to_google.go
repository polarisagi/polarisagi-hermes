package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"polaris-hermes/internal/pkg/httpclient"
	"polaris-hermes/internal/service/channel"
	"polaris-hermes/internal/translator"
)

// anthropicVersionGEAP GEAP 平台要求的 Claude 请求体版本字段
const anthropicVersionGEAP = "vertex-2023-10-16"

// isClaudeModel 判断目标模型是否为 GEAP Claude 合作伙伴模型
func isClaudeModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "claude-")
}

// buildGEAPURL 构建 Google Agent Platform 端点 URL
func buildGEAPURL(ch *channel.ActiveChannel, publisher, subpath, defaultLocation string) string {
	tmpl := ch.Provider.BaseURL
	if tmpl == "" {
		tmpl = "https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{location}/publishers/" + publisher + "/{subpath}"
	}
	
	// 从凭证解析 ProjectID
	var creds map[string]interface{}
	_ = json.Unmarshal(ch.Provider.AuthCredentials, &creds)
	projectID := ""
	if pid, ok := creds["project_id"].(string); ok {
		projectID = pid
	}
	location := defaultLocation
	
	url := strings.ReplaceAll(tmpl, "{project_id}", projectID)
	url = strings.ReplaceAll(url, "{location}", location)
	url = strings.ReplaceAll(url, "{subpath}", subpath)
	return url
}

// AnthropicGoogleTranslator 实现了 translator.Translator 接口
type AnthropicGoogleTranslator struct{}

func NewAnthropicGoogleTranslator() *AnthropicGoogleTranslator {
	return &AnthropicGoogleTranslator{}
}

func (t *AnthropicGoogleTranslator) TranslateAndExecute(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, ch *channel.ActiveChannel, targetModel string) error {
	traceID := r.Header.Get("X-Request-Id")
	if traceID == "" {
		traceID = "req-unknown"
	}

	var req MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"type": "error", "error": {"type": "invalid_request_error", "message": "invalid json"}}`, 400)
		return nil
	}

	extractedBillingHeader := ExtractAndStripBillingHeader(&req)
	if extractedBillingHeader != "" {
		w.Header().Set("X-Anthropic-Billing-Header", extractedBillingHeader)
		bodyBytes, _ = json.Marshal(req)
	}

	finalModel := targetModel
	if finalModel == "" {
		finalModel = req.Model
	}
	useGEAPClaude := isClaudeModel(finalModel)

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		home, err := os.UserHomeDir()
		var dumpPath string
		if err == nil {
			dumpPath = filepath.Join(home, ".polaris-hermes", "claude_debug_body.json")
		} else {
			dumpPath = "claude_debug_body.json"
		}
		_ = os.WriteFile(dumpPath, bodyBytes, 0644)
	}

	// 复杂特征检测（Claude Code Compact）
	isCompact := false
	if len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role == "user" {
			lastMsgBytes, _ := json.Marshal(lastMsg.Content)
			lastMsgStr := string(lastMsgBytes)
			features := 0
			if strings.Contains(lastMsgStr, "TEXT ONLY") { features++ }
			if strings.Contains(strings.ToLower(lastMsgStr), "summary") { features++ }
			if strings.Contains(lastMsgStr, "Do NOT call any tools") { features++ }
			if strings.Contains(lastMsgStr, "<analysis>") { features++ }
			if strings.Contains(lastMsgStr, "<summary>") { features++ }
			
			if req.ContextManagement != nil {
				for _, edit := range req.ContextManagement.Edits {
					if strings.HasPrefix(edit.Type, "clear_thinking_") { features++ }
					if strings.HasPrefix(edit.Type, "compact_") { features++ }
				}
			}
			if features >= 3 {
				isCompact = true
			}
		}
	}

	if isCompact {
		req.Tools = nil
	}

	if useGEAPClaude {
		return handleGEAPClaude(ctx, w, r, bodyBytes, ch, traceID, finalModel, req.Stream)
	} else {
		return handleGemini(ctx, w, bodyBytes, ch, traceID, finalModel, req, isCompact)
	}
}

func handleGEAPClaude(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, ch *channel.ActiveChannel, traceID, model string, stream bool) error {
	geapBody, err := rewriteBodyForGEAPClaude(bodyBytes, false, "")
	if err != nil {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"failed to rewrite body"}}`, http.StatusBadRequest)
		return nil
	}

	var subpath string
	if stream {
		subpath = fmt.Sprintf("models/%s:streamRawPredict", model)
	} else {
		subpath = fmt.Sprintf("models/%s:rawPredict", model)
	}
	targetURL := buildGEAPURL(ch, "anthropic", subpath, "us-east5")

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(geapBody))
	proxyReq.Header.Set("Content-Type", "application/json")

	if r != nil {
		for k, vv := range r.Header {
			if strings.ToLower(k) == "anthropic-beta" {
				for _, v := range vv {
					proxyReq.Header.Add(k, v)
				}
			}
		}
	}

	q := proxyReq.URL.Query()
	q.Set("key", string(ch.Provider.AuthCredentials)) // 简单假设 Credentials 直接是 API Key 字符串或在解析后使用
	proxyReq.URL.RawQuery = q.Encode()

	resp, err := httpclient.Client.Do(proxyReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(errBody)
		return nil
	}

	if stream {
		streamGEAPClaude(w, resp, ch, model, traceID, bodyBytes)
	} else {
		nonStreamGEAPClaude(w, resp, ch, model, traceID, bodyBytes)
	}
	return nil
}

func handleGemini(ctx context.Context, w http.ResponseWriter, bodyBytes []byte, ch *channel.ActiveChannel, traceID, model string, req MessageRequest, isCompact bool) error {
	vReq, _ := mapToVertexRequest(req, model)
	if isCompact {
		// 浓缩为纯文本给 Gemini
		var sb strings.Builder
		for _, msg := range req.Messages {
			sb.WriteString(fmt.Sprintf("<turn role=\"%s\">\n", msg.Role))
			switch c := msg.Content.(type) {
			case string:
				sb.WriteString(c)
			}
			sb.WriteString("\n</turn>\n")
		}
		historyXML := sb.String()
		systemPrompt := flattenAnthropicSystem(req.System)
		promptInjection := fmt.Sprintf("System Context: %s\n\n<conversation_history>\n%s\n</conversation_history>\n\nSystem Task: You are performing a context compaction.", systemPrompt, historyXML)
		
		vReq["contents"] = []map[string]interface{}{{
			"role": "user",
			"parts": []map[string]interface{}{{"text": promptInjection}},
		}}
		delete(vReq, "systemInstruction")
		delete(vReq, "tools")
	}

	vReqBytes, _ := json.Marshal(vReq)

	if model == "" {
		model = "gemini-3.1-pro-preview"
	}

	var subpath string
	if req.Stream {
		subpath = fmt.Sprintf("models/%s:streamGenerateContent", model)
	} else {
		subpath = fmt.Sprintf("models/%s:generateContent", model)
	}
	targetURL := buildGEAPURL(ch, "google", subpath, "global")

	proxyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(vReqBytes))
	proxyReq.Header.Set("Content-Type", "application/json")
	q := proxyReq.URL.Query()
	// 解析凭证以获取 key
	var creds map[string]interface{}
	if err := json.Unmarshal(ch.Provider.AuthCredentials, &creds); err == nil {
		if key, ok := creds["api_key"].(string); ok {
			q.Set("key", key)
		}
	} else {
		// 如果不是 JSON，直接当作普通文本的 Key
		q.Set("key", string(ch.Provider.AuthCredentials))
	}
	
	if req.Stream {
		q.Set("alt", "sse")
	}
	proxyReq.URL.RawQuery = q.Encode()

	resp, err := httpclient.Client.Do(proxyReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(errBody)
		return nil
	}

	if req.Stream {
		streamAnthropicResponse(ctx, w, resp, req, traceID, ch, "Anthropic-Adapter", model, bodyBytes, isCompact)
	} else {
		handleAnthropicNonStreamResponse(w, resp, req, traceID, ch, "Anthropic-Adapter", model, bodyBytes, isCompact)
	}
	return nil
}

func rewriteBodyForGEAPClaude(bodyBytes []byte, isCountTokens bool, targetModel string) ([]byte, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &m); err != nil {
		return nil, err
	}
	m["anthropic_version"] = anthropicVersionGEAP
	if isCountTokens {
		if targetModel != "" { m["model"] = targetModel }
		delete(m, "stream")
		delete(m, "max_tokens")
		delete(m, "temperature")
	} else {
		delete(m, "model")
	}
	return json.Marshal(m)
}

func streamGEAPClaude(w http.ResponseWriter, upstreamResp *http.Response, ch *channel.ActiveChannel, modelName, traceID string, reqBody []byte) {
	translator.CopyHeaders(w.Header(), upstreamResp.Header)
	w.WriteHeader(upstreamResp.StatusCode)
	translator.ForwardStreamBody(w, upstreamResp.Body)
}

func nonStreamGEAPClaude(w http.ResponseWriter, upstreamResp *http.Response, ch *channel.ActiveChannel, modelName, traceID string, reqBody []byte) {
	bodyBytes, _ := io.ReadAll(upstreamResp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(upstreamResp.StatusCode)
	_, _ = w.Write(bodyBytes)
}

func flattenAnthropicSystem(sys interface{}) string {
	if sys == nil {
		return ""
	}
	switch v := sys.(type) {
	case string:
		return v
	case []interface{}:
		var sb strings.Builder
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
					sb.WriteString("\n")
				}
			}
		}
		return strings.TrimSpace(sb.String())
	}
	return ""
}
