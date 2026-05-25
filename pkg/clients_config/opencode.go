package clients_config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type OpenCodeConfigurator struct {
	configPath string
}

func NewOpenCodeConfigurator() *OpenCodeConfigurator {
	appData, _ := getAppDataDir("opencode")
	return &OpenCodeConfigurator{
		configPath: filepath.Join(appData, "opencode.json"),
	}
}

func (c *OpenCodeConfigurator) Apply(gatewayAddr string) error {
	if err := ensureDirExists(c.configPath); err != nil {
		return err
	}

	if err := backupFile(c.configPath); err != nil {
		return fmt.Errorf("failed to backup opencode config: %v", err)
	}

	var config map[string]interface{}
	data, err := os.ReadFile(c.configPath)
	if err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse opencode config: %v", err)
		}
	} else if os.IsNotExist(err) {
		config = make(map[string]interface{})
		config["$schema"] = "https://opencode.ai/config.json"
	} else {
		return err
	}

	// Update provider section
	providers, ok := config["provider"].(map[string]interface{})
	if !ok {
		providers = make(map[string]interface{})
		config["provider"] = providers
	}

	polarisProvider := map[string]interface{}{
		"apiEndpoint": fmt.Sprintf("http://%s/v1/openai/", gatewayAddr),
		"apiKey":      "polaris",
		"name":        "Polaris Hermes",
	}

	providers["polaris"] = polarisProvider

	// Also add the plugin if needed, like cc-switch does:
	// "plugin": ["oh-my-openagent"]
	pluginsRaw, hasPlugins := config["plugin"]
	var plugins []interface{}
	if hasPlugins {
		if pList, ok := pluginsRaw.([]interface{}); ok {
			plugins = pList
		}
	}

	hasOmo := false
	for _, p := range plugins {
		if pStr, ok := p.(string); ok && pStr == "oh-my-openagent" {
			hasOmo = true
			break
		}
	}
	if !hasOmo {
		plugins = append(plugins, "oh-my-openagent")
		config["plugin"] = plugins
	}

	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode opencode config: %v", err)
	}

	return os.WriteFile(c.configPath, newData, 0644)
}

func (c *OpenCodeConfigurator) Restore() error {
	return restoreFile(c.configPath)
}

func (c *OpenCodeConfigurator) Status() (bool, bool, error) {
	hasBak := hasBackup(c.configPath)

	data, err := os.ReadFile(c.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, hasBak, nil
		}
		return false, hasBak, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return false, hasBak, err
	}

	isConfigured := false
	if providers, ok := config["provider"].(map[string]interface{}); ok {
		if _, ok := providers["polaris"]; ok {
			isConfigured = true
		}
	}

	return isConfigured, hasBak, nil
}
