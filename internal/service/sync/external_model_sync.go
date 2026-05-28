package sync

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"polaris-hermes/internal/domain"
	"polaris-hermes/internal/repository/sqlite"
	"polaris-hermes/internal/service/router"
)

type SyncService struct {
	modelRepo *sqlite.ModelRepo
	inferer   *router.IntentInferer
}

func NewSyncService(modelRepo *sqlite.ModelRepo, inferer *router.IntentInferer) *SyncService {
	return &SyncService{
		modelRepo: modelRepo,
		inferer:   inferer,
	}
}

// openRouterResponse 结构
type openRouterResponse struct {
	Data []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
		Created       int64  `json:"created"`
		Pricing       struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		} `json:"pricing"`
		Architecture struct {
			InputModalities []string `json:"input_modalities"`
		} `json:"architecture"`
	} `json:"data"`
}

func (s *SyncService) SyncGlobalModels(ctx context.Context) error {
	slog.Info("[Sync] Fetching global model dictionary...")

	globalModels := make(map[string]*domain.SysModel)

	// Fetch from OpenRouter
	if err := s.fetchOpenRouter(ctx, globalModels); err != nil {
		slog.Warn("[Sync] Failed to fetch OpenRouter data", "error", err)
	}

	// Fetch from LiteLLM (Optional, OpenRouter gives us a lot already, but let's parse LiteLLM for context limits not in OpenRouter)
	if err := s.fetchLiteLLM(ctx, globalModels); err != nil {
		slog.Warn("[Sync] Failed to fetch LiteLLM data", "error", err)
	}

	// Persist
	slog.Info("[Sync] Merging into sys_models...", "count", len(globalModels))
	for _, m := range globalModels {
		// Populate capability tier and legacy status
		if m.CapabilityTier == "" {
			m.CapabilityTier = s.inferer.InferUnknownModel(ctx, m.ModelID)
		}
		if !m.IsLegacy {
			m.IsLegacy = s.inferer.IsLegacyModel(m.ModelID)
		}
		m.VersionWeight = s.inferer.ParseVersionWeight(m.ModelID)
		m.IsActive = true

		if err := s.modelRepo.UpsertSysModel(ctx, m); err != nil {
			slog.Error("[Sync] Failed to upsert sys_model", "model_id", m.ModelID, "error", err)
		}
	}
	slog.Info("[Sync] Global model dictionary synced successfully!")
	return nil
}

func (s *SyncService) fetchOpenRouter(ctx context.Context, globalModels map[string]*domain.SysModel) error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, m := range result.Data {
		// OpenRouter returns id like "openai/gpt-4o"
		parts := strings.SplitN(m.ID, "/", 2)
		modelID := m.ID
		if len(parts) == 2 {
			modelID = parts[1] // Use just "gpt-4o" as the global ID
		}

		sysModel := &domain.SysModel{
			ModelID:       modelID,
			DisplayName:   m.Name,
			ContextLength: m.ContextLength,
		}

		// Pricing
		if p, err := strconv.ParseFloat(m.Pricing.Prompt, 64); err == nil {
			sysModel.PromptPricePer1k = p * 1000
		}
		if p, err := strconv.ParseFloat(m.Pricing.Completion, 64); err == nil {
			sysModel.CompletionPricePer1k = p * 1000
		}

		// Created
		if m.Created > 0 {
			t := time.Unix(m.Created, 0).Format(time.RFC3339)
			sysModel.ReleasedAt = &t
		}

		// Vision
		for _, mod := range m.Architecture.InputModalities {
			if mod == "image" {
				sysModel.SupportsVision = true
			}
		}

		globalModels[sysModel.ModelID] = sysModel
	}
	return nil
}

func (s *SyncService) fetchLiteLLM(ctx context.Context, globalModels map[string]*domain.SysModel) error {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json", nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for key, val := range result {
		if key == "sample_spec" {
			continue
		}
		
		// Some litellm keys are like "anthropic/claude-3-opus-20240229" or just "gpt-4o"
		modelID := key
		parts := strings.SplitN(key, "/", 2)
		if len(parts) == 2 && !strings.Contains(parts[0], "-") {
			modelID = parts[1]
		}
		
		info, ok := val.(map[string]interface{})
		if !ok {
			continue
		}

		m, exists := globalModels[modelID]
		if !exists {
			m = &domain.SysModel{
				ModelID:     modelID,
				DisplayName: modelID,
			}
			globalModels[modelID] = m
		}

		if maxIn, ok := info["max_input_tokens"].(float64); ok && m.ContextLength == 0 {
			m.ContextLength = int(maxIn)
		}
		if maxOut, ok := info["max_output_tokens"].(float64); ok && m.MaxOutputTokens == 0 {
			m.MaxOutputTokens = int(maxOut)
		}
		if vision, ok := info["supports_vision"].(bool); ok {
			m.SupportsVision = m.SupportsVision || vision
		}
		if tools, ok := info["supports_function_calling"].(bool); ok {
			m.SupportsTools = m.SupportsTools || tools
		}
	}
	return nil
}
