package clients_config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type CodexConfigurator struct {
	configPath string
}

func NewCodexConfigurator() *CodexConfigurator {
	home, _ := getUserHomeDir()
	return &CodexConfigurator{
		configPath: filepath.Join(home, ".codex", "config.toml"),
	}
}

func (c *CodexConfigurator) Apply(gatewayAddr string) error {
	if err := ensureDirExists(c.configPath); err != nil {
		return err
	}

	// Backup original config
	if err := backupFile(c.configPath); err != nil {
		return fmt.Errorf("failed to backup codex config: %v", err)
	}

	// Read existing config
	var config map[string]interface{}
	data, err := os.ReadFile(c.configPath)
	if err == nil {
		if err := toml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse codex config: %v", err)
		}
	} else if os.IsNotExist(err) {
		config = make(map[string]interface{})
	} else {
		return err
	}

	baseURL := fmt.Sprintf("http://%s/v1/openai/", gatewayAddr)

	// Set base_url logic (simulating cc-switch logic)
	// We check if there's an active model_provider
	modelProviderRaw, hasProvider := config["model_provider"]
	if hasProvider {
		modelProvider, ok := modelProviderRaw.(string)
		if ok && modelProvider != "" {
			// Ensure model_providers table exists
			providersMap, ok := config["model_providers"].(map[string]interface{})
			if !ok {
				providersMap = make(map[string]interface{})
				config["model_providers"] = providersMap
			}

			// Ensure specific provider table exists
			providerConfig, ok := providersMap[modelProvider].(map[string]interface{})
			if !ok {
				providerConfig = make(map[string]interface{})
				providersMap[modelProvider] = providerConfig
			}

			providerConfig["base_url"] = baseURL
			// Optional: force wire_api = "openai" or "responses" based on original cc-switch behavior
			// We skip wire_api to avoid breaking user's specific setup unless necessary
		} else {
			// Fallback to top-level base_url
			config["base_url"] = baseURL
		}
	} else {
		// No model_provider, set top-level base_url
		config["base_url"] = baseURL
	}

	// Write back
	newData, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to encode codex config: %v", err)
	}

	return os.WriteFile(c.configPath, newData, 0644)
}

func (c *CodexConfigurator) Restore() error {
	return restoreFile(c.configPath)
}

func (c *CodexConfigurator) Status() (bool, bool, error) {
	hasBak := hasBackup(c.configPath)

	// Check if configured
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, hasBak, nil
		}
		return false, hasBak, err
	}

	var config map[string]interface{}
	if err := toml.Unmarshal(data, &config); err != nil {
		return false, hasBak, err
	}

	isConfigured := false
	modelProviderRaw, hasProvider := config["model_provider"]
	if hasProvider {
		if modelProvider, ok := modelProviderRaw.(string); ok {
			if providersMap, ok := config["model_providers"].(map[string]interface{}); ok {
				if providerConfig, ok := providersMap[modelProvider].(map[string]interface{}); ok {
					if url, ok := providerConfig["base_url"].(string); ok {
						isConfigured = url != "" && url == "http://127.0.0.1:27777/v1/openai/" // Rough check
					}
				}
			}
		}
	}

	// Fallback check top level
	if !isConfigured {
		if url, ok := config["base_url"].(string); ok {
			isConfigured = url != "" && url == "http://127.0.0.1:27777/v1/openai/"
		}
	}

	return isConfigured, hasBak, nil
}
