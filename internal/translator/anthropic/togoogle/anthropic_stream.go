// Anthropic 响应流式/非流式处理 + SSE 写入工具
// 从 Google Agent Platform 后端读取 GenerateContentResponse，实时转换为 Anthropic SSE 格式并推送给客户端
package togoogle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// writeSSE 写入一条 Anthropic SSE 事件到 HTTP 响应流
// 格式: event: <type>\ndata: <json>\n\n
func writeSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b)
	if flusher != nil {
		flusher.Flush()
	}
}

func ptrInt(i int) *int {
	return &i
}

// normalizeFunctionCallArgs 统一将 Gemini functionCall.args 转换为规范的 JSON 字节。
// Gemini 可能将 args 返回为 map[string]interface{}、JSON 字符串或其他类型，
// 此函数负责将所有可能的形式归一化为紧凑的 JSON 字节数组。
func normalizeFunctionCallArgs(args interface{}) []byte {
	if args == nil {
		return []byte("{}")
	}

	switch v := args.(type) {
	case map[string]interface{}:
		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		_ = encoder.Encode(v)
		result := buffer.Bytes()
		if len(result) > 0 && result[len(result)-1] == '\n' {
			result = result[:len(result)-1]
		}
		if len(result) == 0 || string(result) == "null" {
			return []byte("{}")
		}
		return result
	case string:
		if v == "" || v == "null" {
			return []byte("{}")
		}
		// args 可能是 JSON 字符串，尝试解析后再序列化以规范化格式
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			buffer := &bytes.Buffer{}
			encoder := json.NewEncoder(buffer)
			encoder.SetEscapeHTML(false)
			_ = encoder.Encode(parsed)
			result := buffer.Bytes()
			if len(result) > 0 && result[len(result)-1] == '\n' {
				result = result[:len(result)-1]
			}
			return result
		}
		// 不是合法 JSON，直接当纯文本返回
		return []byte(v)
	default:
		raw, _ := json.Marshal(v)
		if len(raw) == 0 || string(raw) == "null" {
			return []byte("{}")
		}
		return raw
	}
}
