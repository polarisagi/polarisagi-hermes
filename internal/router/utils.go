package router

import (
	"regexp"
)

var (
	modelRegexOpenAI    = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
	modelRegexAnthropic = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
)

func extractModelName(body []byte, protocol string) string {
	if protocol == "openai" {
		match := modelRegexOpenAI.FindSubmatch(body)
		if len(match) > 1 {
			return string(match[1])
		}
	} else if protocol == "anthropic" {
		match := modelRegexAnthropic.FindSubmatch(body)
		if len(match) > 1 {
			return string(match[1])
		}
	}
	return ""
}
