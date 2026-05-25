package clients_config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type HermesConfigurator struct {
	configPath string
}

func NewHermesConfigurator() *HermesConfigurator {
	home, _ := getUserHomeDir()
	return &HermesConfigurator{
		configPath: filepath.Join(home, ".hermes", "config.yaml"),
	}
}

func (c *HermesConfigurator) Apply(gatewayAddr string) error {
	if err := ensureDirExists(c.configPath); err != nil {
		return err
	}

	if err := backupFile(c.configPath); err != nil {
		return fmt.Errorf("failed to backup hermes config: %v", err)
	}

	data, err := os.ReadFile(c.configPath)
	var config map[string]interface{}

	if err == nil && len(data) > 0 {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse hermes config: %v", err)
		}
	}

	if config == nil {
		config = make(map[string]interface{})
	}

	// 1. Update custom_providers
	customProvidersRaw, ok := config["custom_providers"].([]interface{})
	if !ok {
		customProvidersRaw = []interface{}{}
	}

	found := false
	for i, cpRaw := range customProvidersRaw {
		if cp, ok := cpRaw.(map[string]interface{}); ok {
			if name, ok := cp["name"].(string); ok && name == "polaris" {
				cp["base_url"] = fmt.Sprintf("http://%s/v1/openai/", gatewayAddr)
				cp["api_key"] = "polaris"
				customProvidersRaw[i] = cp
				found = true
				break
			}
		}
	}

	if !found {
		customProvidersRaw = append(customProvidersRaw, map[string]interface{}{
			"name":     "polaris",
			"base_url": fmt.Sprintf("http://%s/v1/openai/", gatewayAddr),
			"api_key":  "polaris",
		})
	}
	config["custom_providers"] = customProvidersRaw

	// 2. Set default model provider to polaris
	modelSection, ok := config["model"].(map[string]interface{})
	if !ok {
		modelSection = make(map[string]interface{})
	}
	modelSection["provider"] = "polaris"
	config["model"] = modelSection

	newData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to encode hermes config: %v", err)
	}

	return os.WriteFile(c.configPath, newData, 0644)
}

func (c *HermesConfigurator) Restore() error {
	return restoreFile(c.configPath)
}

func (c *HermesConfigurator) Status() (bool, bool, error) {
	hasBak := hasBackup(c.configPath)

	data, err := os.ReadFile(c.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, hasBak, nil
		}
		return false, hasBak, err
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return false, hasBak, err
	}

	isConfigured := false

	// Check if polaris exists in custom_providers
	if customProvidersRaw, ok := config["custom_providers"].([]interface{}); ok {
		for _, cpRaw := range customProvidersRaw {
			if cp, ok := cpRaw.(map[string]interface{}); ok {
				if name, ok := cp["name"].(string); ok && name == "polaris" {
					isConfigured = true
					break
				}
			}
		}
	}

	// Also check if model.provider is polaris
	if isConfigured {
		if modelSection, ok := config["model"].(map[string]interface{}); ok {
			if provider, ok := modelSection["provider"].(string); ok && provider != "polaris" {
				isConfigured = false // Provider changed back to something else
			}
		}
	}

	return isConfigured, hasBak, nil
}
