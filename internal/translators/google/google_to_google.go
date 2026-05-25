// Google Agent Platform 原生直通处理器
// 支持 GEAP → GEAP（同协议直通），使用 generateContent/streamGenerateContent 端点
// 官方 REST API 参考：https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest
// 自动构建包含 project_id/location/model 的 GEAP 端点 URL，并注入 API Key 认证
package google

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"polaris-gateway/internal/router"
)

// GoogleToGoogle 将 Google Agent Platform 原生请求协议直通到 GEAP 后端：不做协议转换，仅替换端点 URL 和认证方式
func GoogleToGoogle(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, dest *router.MatchedDestination, traceID string) error {
	clientType := router.IdentifyClient(r)
	methodName := router.ExtractMethodName(r.URL.Path)

	targetURL := buildGoogleTargetURL(dest.Node, r.URL.Path, dest.TargetModel)

	bodyBytes = fixGoogleRequestBody(bodyBytes)

	proxyReq, _ := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	for k, vv := range r.Header {
		if !strings.EqualFold(k, "Host") && !strings.EqualFold(k, "Content-Length") &&
			!strings.EqualFold(k, "Accept-Encoding") && !strings.EqualFold(k, "Authorization") {
			for _, v := range vv {
				proxyReq.Header.Add(k, v)
			}
		}
	}

	q := proxyReq.URL.Query()
	for k, vv := range r.URL.Query() {
		for _, v := range vv {
			q.Add(k, v)
		}
	}
	proxyReq.URL.RawQuery = q.Encode()

	if err := dest.Node.InjectGoogleAuth(proxyReq); err != nil {
		slog.Error("❌ [GoogleToGoogle] 注入认证信息失败", "node", dest.Node.Name, "err", err)
		http.Error(w, "Failed to generate ADC Token", http.StatusInternalServerError)
		return nil
	}

	return router.ExecuteAndStream(w, proxyReq, dest, "google", clientType, methodName, traceID, "Google Agent Platform",
		func(finalResp *http.Response, startTime time.Time) bool {
			streamGoogleResponse(w, finalResp, dest, dest.TargetModel, clientType, methodName, traceID, startTime, bodyBytes)
			return false
		})
}

// streamGoogleResponse 流式转发 Google Agent Platform 上游响应到客户端，同步从尾部提取 usageMetadata 完成计费
func streamGoogleResponse(w http.ResponseWriter, finalResp *http.Response, dest *router.MatchedDestination, modelName, clientType, methodName, traceID string, startTime time.Time, reqBody []byte) {
	defer finalResp.Body.Close()

	for k, vv := range finalResp.Header {
		if !strings.EqualFold(k, "Content-Length") && !strings.EqualFold(k, "Content-Encoding") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(finalResp.StatusCode)

	tailBuf, totalWritten := router.ForwardStreamBody(w, finalResp.Body)

	prompt, completion, cached, found := router.ParseUsageFromStreamTail(tailBuf)
	if !found {
		prompt = router.EstimatePromptTokens(reqBody)
		completion = router.EstimateCompletionTokens(totalWritten)
		slog.Warn("⚠️ 响应流中断，启用 token 估算补偿", "trace_id", traceID, "node", dest.Node.Name, "prompt", prompt, "completion", completion)
	}

	router.SettleBilling("google", dest.Node.Name, clientType, methodName, modelName,
		prompt, completion, cached, finalResp.StatusCode, dest, reqBody, traceID)
}

// buildGoogleTargetURL 构建 Google Agent Platform (GEAP) 端点 URL
// 官方文档：https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest
// 完全忽略客户端传递的乱七八糟的路径，直接提取 method 和 model，
// 按照后台配置的自己的 URL 规则重新拼接，确保代理的请求格式永远是正确一致的。
func buildGoogleTargetURL(node *router.NodeState, incomingPath string, targetModel string) string {
	if targetModel != "" {
		targetModel = strings.TrimPrefix(targetModel, "google/")
	}

	// 提取请求方法 (如 generateContent, streamGenerateContent)
	method := "generateContent"
	if idx := strings.LastIndex(incomingPath, ":"); idx >= 0 {
		method = incomingPath[idx+1:]
	} else if strings.HasSuffix(incomingPath, "generateContent") {
		method = "generateContent"
	} else if strings.HasSuffix(incomingPath, "streamGenerateContent") {
		method = "streamGenerateContent"
	} else if strings.HasSuffix(incomingPath, "countTokens") {
		method = "countTokens"
	}

	location := node.Location
	if location == "" {
		location = "global"
	}

	template := node.BaseURL
	if template == "" {
		template = inferGEAPHost(location) + "/v1/projects/{project_id}/locations/{location}/{publisher_subpath}"
	}

	publisherSubpath := "publishers/google/models/" + targetModel + ":" + method

	resURL := strings.ReplaceAll(template, "{project_id}", node.ProjectID)
	resURL = strings.ReplaceAll(resURL, "{location}", location)
	resURL = strings.ReplaceAll(resURL, "{publisher_subpath}", publisherSubpath)
	resURL = strings.ReplaceAll(resURL, "{subpath}", "models/"+targetModel+":"+method)

	return resURL
}

// inferGEAPHost 根据 location 推断 Google Agent Platform API host
// 参考：https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest
// global 端点用 aiplatform.googleapis.com，区域端点用 {region}-aiplatform.googleapis.com
func inferGEAPHost(location string) string {
	if location == "" || location == "global" {
		return "https://aiplatform.googleapis.com"
	}
	return "https://" + location + "-aiplatform.googleapis.com"
}



// fixGoogleRequestBody 修复客户端发送的非法请求体（如缺少 parts 字段）
// GEAP 强制要求 systemInstruction 和 contents 中的每一项必须包含至少一个 parts。
func fixGoogleRequestBody(bodyBytes []byte) []byte {
	if len(bodyBytes) == 0 {
		return bodyBytes
	}

	var reqData map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &reqData); err != nil {
		return bodyBytes
	}

	modified := false

	if sysInstr, ok := reqData["systemInstruction"].(map[string]interface{}); ok {
		if parts, ok := sysInstr["parts"].([]interface{}); !ok || len(parts) == 0 {
			sysInstr["parts"] = []interface{}{map[string]interface{}{"text": ""}}
			modified = true
		}
	}

	if contents, ok := reqData["contents"].([]interface{}); ok {
		for _, item := range contents {
			if contentMap, ok := item.(map[string]interface{}); ok {
				if parts, ok := contentMap["parts"].([]interface{}); ok {
					var newParts []interface{}
					for _, p := range parts {
						if partMap, ok := p.(map[string]interface{}); ok {
							// Filter out thought parts to prevent 499 Cancelled errors
							if isThought, ok := partMap["thought"].(bool); ok && isThought {
								// Keep the part only if it has a thoughtSignature, but strip thought and text
								sig, hasSig := partMap["thoughtSignature"]
								if hasSig {
									newParts = append(newParts, map[string]interface{}{
										"thoughtSignature": sig,
									})
								}
								modified = true
								continue
							}
							newParts = append(newParts, p)
						} else {
							newParts = append(newParts, p)
						}
					}
					
					if len(newParts) == 0 {
						newParts = []interface{}{map[string]interface{}{"text": ""}}
						modified = true
					} else if len(newParts) != len(parts) {
						modified = true
					}
					contentMap["parts"] = newParts
				} else {
					contentMap["parts"] = []interface{}{map[string]interface{}{"text": ""}}
					modified = true
				}
			}
		}
	}

	if modified {
		if newBody, err := json.Marshal(reqData); err == nil {
			return newBody
		}
	}

	return bodyBytes
}

func init() {
	router.RegisterTranslator("google", "google", GoogleToGoogle)
}