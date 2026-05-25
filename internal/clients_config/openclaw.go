package clients_config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/titanous/json5"
)

type OpenClawConfigurator struct {
	configPath string
}

func NewOpenClawConfigurator() *OpenClawConfigurator {
	home, _ := getUserHomeDir()
	return &OpenClawConfigurator{
		configPath: filepath.Join(home, ".openclaw", "openclaw.json"),
	}
}

func (c *OpenClawConfigurator) Apply(gatewayAddr string) error {
	if err := ensureDirExists(c.configPath); err != nil {
		return err
	}

	if err := backupFile(c.configPath); err != nil {
		return fmt.Errorf("failed to backup openclaw config: %v", err)
	}

	var config map[string]interface{}
	data, err := os.ReadFile(c.configPath)
	if err == nil {
		if err := json5.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse openclaw config: %v", err)
		}
	} else if os.IsNotExist(err) {
		config = make(map[string]interface{})
	} else {
		return err
	}

	// Navigate and create models/providers
	modelsRaw, ok := config["models"].(map[string]interface{})
	if !ok {
		modelsRaw = make(map[string]interface{})
		config["models"] = modelsRaw
	}

	providersRaw, ok := modelsRaw["providers"].(map[string]interface{})
	if !ok {
		providersRaw = make(map[string]interface{})
		modelsRaw["providers"] = providersRaw
	}

	providersRaw["polaris"] = map[string]interface{}{
		"base_url": fmt.Sprintf("http://%s/v1/openai/", gatewayAddr),
		"api_key":  "polaris",
	}

	// Write back as normal JSON (which is valid JSON5)
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode openclaw config: %v", err)
	}

	return os.WriteFile(c.configPath, newData, 0644)
}

func (c *OpenClawConfigurator) Restore() error {
	return restoreFile(c.configPath)
}

func (c *OpenClawConfigurator) Status() (bool, bool, error) {
	hasBak := hasBackup(c.configPath)

	data, err := os.ReadFile(c.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, hasBak, nil
		}
		return false, hasBak, err
	}

	var config map[string]interface{}
	if err := json5.Unmarshal(data, &config); err != nil {
		return false, hasBak, err
	}

	isConfigured := false
	if modelsRaw, ok := config["models"].(map[string]interface{}); ok {
		if providersRaw, ok := modelsRaw["providers"].(map[string]interface{}); ok {
			if _, ok := providersRaw["polaris"]; ok {
				isConfigured = true
			}
		}
	}

	return isConfigured, hasBak, nil
}
