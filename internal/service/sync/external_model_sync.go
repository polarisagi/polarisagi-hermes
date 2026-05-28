package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"polaris-hermes/internal/config"
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
	slog.Info("[Sync] Starting global model dictionary sync process...")

	dataDir := config.GlobalConfig.Sync.DataDir
	if dataDir == "" {
		slog.Warn("[Sync] Sync data_dir is empty, skipping.")
		return nil
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Error("[Sync] Failed to create data_dir", "error", err)
		return err
	}

	useGit := config.GlobalConfig.Sync.EnableGitTracking

	if useGit {
		if _, err := os.Stat(filepath.Join(dataDir, ".git")); os.IsNotExist(err) {
			slog.Info("[Sync] Initializing Git repository in data_dir")
			_ = exec.Command("git", "-C", dataDir, "init").Run()
		}
	}

	changed := false
	if err := downloadFile(ctx, "https://openrouter.ai/api/v1/models", filepath.Join(dataDir, "openrouter_models.json")); err != nil {
		slog.Warn("[Sync] Failed to download openrouter_models.json", "error", err)
	}
	if err := downloadFile(ctx, "https://openrouter.ai/api/v1/providers", filepath.Join(dataDir, "openrouter_providers.json")); err != nil {
		slog.Warn("[Sync] Failed to download openrouter_providers.json", "error", err)
	}
	if err := downloadFile(ctx, "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json", filepath.Join(dataDir, "litellm_models.json")); err != nil {
		slog.Warn("[Sync] Failed to download litellm_models.json", "error", err)
	}

	if useGit {
		cmd := exec.Command("git", "-C", dataDir, "status", "--porcelain")
		out, err := cmd.Output()
		if err == nil && len(bytes.TrimSpace(out)) == 0 {
			slog.Info("[Sync] No changes detected in external data. Skipping database update.")
			return nil
		}
		changed = true
	} else {
		changed = true
	}

	if !changed {
		return nil
	}

	slog.Info("[Sync] Changes detected or Git tracking disabled. Parsing and updating database...")

	globalModels := make(map[string]*domain.SysModel)

	if err := s.parseOpenRouter(filepath.Join(dataDir, "openrouter_models.json"), globalModels); err != nil {
		slog.Warn("[Sync] Failed to parse OpenRouter data", "error", err)
	}

	if err := s.parseLiteLLM(filepath.Join(dataDir, "litellm_models.json"), globalModels); err != nil {
		slog.Warn("[Sync] Failed to parse LiteLLM data", "error", err)
	}

	slog.Info("[Sync] Merging into sys_models...", "count", len(globalModels))
	for _, m := range globalModels {
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
	slog.Info("[Sync] Global model dictionary synced successfully to database!")

	if useGit {
		_ = exec.Command("git", "-C", dataDir, "add", ".").Run()
		msg := fmt.Sprintf("auto-sync: %s", time.Now().Format("2006-01-02 15:04:05"))
		_ = exec.Command("git", "-C", dataDir, "commit", "-m", msg).Run()
		slog.Info("[Sync] Git commit created successfully.")
	}

	return nil
}

func downloadFile(ctx context.Context, url, path string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (s *SyncService) parseOpenRouter(path string, globalModels map[string]*domain.SysModel) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var result openRouterResponse
	if err := json.NewDecoder(f).Decode(&result); err != nil {
		return err
	}

	for _, m := range result.Data {
		parts := strings.SplitN(m.ID, "/", 2)
		modelID := m.ID
		if len(parts) == 2 {
			modelID = parts[1]
		}

		sysModel := &domain.SysModel{
			ModelID:       modelID,
			DisplayName:   m.Name,
			ContextLength: m.ContextLength,
		}

		if p, err := strconv.ParseFloat(m.Pricing.Prompt, 64); err == nil {
			sysModel.PromptPricePer1k = p * 1000
		}
		if p, err := strconv.ParseFloat(m.Pricing.Completion, 64); err == nil {
			sysModel.CompletionPricePer1k = p * 1000
		}

		if m.Created > 0 {
			sysModel.ReleasedAt = m.Created
		}

		for _, mod := range m.Architecture.InputModalities {
			if mod == "image" {
				sysModel.SupportsVision = true
			}
		}

		globalModels[sysModel.ModelID] = sysModel
	}
	return nil
}

func (s *SyncService) parseLiteLLM(path string, globalModels map[string]*domain.SysModel) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(f).Decode(&result); err != nil {
		return err
	}

	for key, val := range result {
		if key == "sample_spec" {
			continue
		}
		
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
