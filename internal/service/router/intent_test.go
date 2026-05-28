package router

import (
	"testing"
)

func TestIntentInferer_InferByKeywords(t *testing.T) {
	inferer := &IntentInferer{}

	tests := []struct {
		modelID  string
		expected string
	}{
		{"gpt-4o-mini-2024", "fast"},
		{"claude-3-5-sonnet-20241022", "smart"},
		{"deepseek-v4-pro", "reasoning"},
		{"gemini-3.1-pro-preview-customtool", "smart"},
		{"text-embedding-3-small", "fast"},        // 匹配 "small" 关键字 → fast
		{"claude-3-opus-20240229", "smart"},        // 匹配 "opus" 关键字 → smart
		{"gpt-4-0613", "smart"},                    // 匹配 "gpt-4" 关键字 → smart
		{"o1-preview", "reasoning"},
		{"gemini-1.5-flash-latest", "fast"},
		{"unknown-model-xyz", ""},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			actual := inferer.inferByKeywords(tt.modelID)
			if actual != tt.expected {
				t.Errorf("inferByKeywords(%q) = %q, expected %q", tt.modelID, actual, tt.expected)
			}
		})
	}
}

// NOTE: We don't fully test InferUnknownModel here as it requires a mock DB repo, 
// but we verified the core regex classification logic.
