package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"polaris-hermes/internal/domain"
	"polaris-hermes/internal/pkg/httpclient"
	"polaris-hermes/internal/service/channel"
	"polaris-hermes/internal/translator"
)

// OpenAITranslator 实现 OpenAI → OpenAI 协议的透传翻译器。
// 适用于客户端（如 Codex）发送 OpenAI 格式请求，后端为 OpenAI-Compatible 接口（如 DeepSeek）的场景。
// 主要职责：
//  1. 替换请求体中的 model 字段为实际后端模型名
//  2. 设置正确的鉴权头（Bearer token）
//  3. 透传请求到后端，流式/非流式均兼容
type OpenAITranslator struct{}

func NewOpenAITranslator() *OpenAITranslator {
	return &OpenAITranslator{}
}

func (t *OpenAITranslator) TranslateAndExecute(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	bodyBytes []byte,
	ch *channel.ActiveChannel,
	targetEndpoint *domain.SysAccessEndpoint,
	targetModel string,
) error {
	// 1. 替换请求体中的 model 字段为路由器确定的真实后端模型名
	var reqMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &reqMap); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return nil
	}
	if targetModel != "" {
		reqMap["model"] = targetModel
	}
	newBody, err := json.Marshal(reqMap)
	if err != nil {
		http.Error(w, "Failed to rewrite request body", http.StatusInternalServerError)
		return nil
	}

	// 2. 构造目标 URL（透传到后端的 /v1/chat/completions）
	targetURL := translator.BuildTargetURL(ch, targetEndpoint, "/chat/completions")

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(newBody))
	if err != nil {
		return fmt.Errorf("failed to create upstream request: %w", err)
	}

	// 3. 设置请求头
	translator.CopyHeaders(proxyReq.Header, r.Header)
	proxyReq.Header.Set("Content-Type", "application/json")

	// 4. 注入鉴权信息（从渠道凭证中提取 api_key，以 Bearer 方式携带）
	var creds map[string]interface{}
	if err := json.Unmarshal(ch.Provider.AuthCredentials, &creds); err == nil {
		if apiKey, ok := creds["api_key"].(string); ok && apiKey != "" {
			proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	// 5. 发送请求
	resp, err := httpclient.Client.Do(proxyReq)
	if err != nil {
		slog.Error("⚡ [OpenAI Translator] 上游请求失败", "url", targetURL, "error", err)
		http.Error(w, "Upstream request failed: "+err.Error(), http.StatusBadGateway)
		return nil
	}
	defer resp.Body.Close()

	// 6. 透传响应头与状态码
	translator.CopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// 7. 判断是否为流式响应，并透传响应体
	isStream := false
	if s, ok := reqMap["stream"].(bool); ok {
		isStream = s
	}

	if isStream {
		// 流式：逐块透传，保持 SSE 格式
		translator.ForwardStreamBody(w, resp.Body)
	} else {
		// 非流式：一次性读取并写入
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("⚡ [OpenAI Translator] 读取上游响应失败", "error", err)
			return nil
		}
		_, _ = w.Write(respBody)
	}

	slog.Debug("✅ [OpenAI Translator] 请求完成", "target_model", targetModel, "status", resp.StatusCode)
	return nil
}
