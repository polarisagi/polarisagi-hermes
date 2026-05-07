package protocol_anthropic

import (
	"strings"
)

const (
	PricePer1MPrompt    = 1.25
	PricePer1MCandidate = 3.75
)

type ModelPrice struct {
	Prompt1M    float64
	Candidate1M float64
}

var vertexPriceDict = map[string]ModelPrice{
	"gemini-3.1-pro-preview-customtools": {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.1-pro-preview":             {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.1-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.1-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-3.1-ultra":                   {Prompt1M: 3.50, Candidate1M: 10.50},
	"gemini-3.0-pro":                     {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-3.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-3-flash-preview":             {Prompt1M: 0.10, Candidate1M: 0.40},
	"gemini-2.5-flash":                   {Prompt1M: 0.075, Candidate1M: 0.30},
	"gemini-2.0-pro-exp":                 {Prompt1M: 1.25, Candidate1M: 3.75},
	"gemini-2.0-flash":                   {Prompt1M: 0.10, Candidate1M: 0.40},
	"default":                            {Prompt1M: PricePer1MPrompt, Candidate1M: PricePer1MCandidate},
}

func calculateCost(modelName string, promptTokens, candidateTokens, cachedTokens int64) float64 {
	price, exists := vertexPriceDict[modelName]
	if !exists {
		price = vertexPriceDict["default"]
	}

	promptRate := price.Prompt1M
	candidateRate := price.Candidate1M

	if strings.HasPrefix(modelName, "gemini-") && promptTokens > 128000 {
		promptRate *= 2.0
		candidateRate *= 2.0
	}

	uncachedTokens := promptTokens - cachedTokens
	if uncachedTokens < 0 {
		uncachedTokens = 0
	}

	// Gemini Cached Context discount is ~25% of standard rate
	cachedRate := promptRate * 0.25

	return (float64(uncachedTokens)/1000000.0*promptRate) +
		(float64(cachedTokens)/1000000.0*cachedRate) +
		(float64(candidateTokens)/1000000.0*candidateRate)
}
