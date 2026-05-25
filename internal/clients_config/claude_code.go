package clients_config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ClaudeCodeConfigurator struct {
	configPath string
}

func NewClaudeCodeConfigurator() *ClaudeCodeConfigurator {
	home, _ := getUserHomeDir()
	return &ClaudeCodeConfigurator{
		configPath: filepath.Join(home, ".claude", "config.json"), // Assuming claude code uses ~/.claude/config.json
	}
}

func (c *ClaudeCodeConfigurator) Apply(gatewayAddr string) error {
	if err := ensureDirExists(c.configPath); err != nil {
		return err
	}

	if err := backupFile(c.configPath); err != nil {
		return fmt.Errorf("failed to backup claude config: %v", err)
	}

	var config map[string]interface{}
	data, err := os.ReadFile(c.configPath)
	if err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse claude config: %v", err)
		}
	} else if os.IsNotExist(err) {
		config = make(map[string]interface{})
	} else {
		return err
	}

	// As seen in cc-switch claude_plugin.rs: primaryApiKey = "any"
	config["primaryApiKey"] = "any"
	// For routing, Claude Code may need an env var ANTHROPIC_BASE_URL.
	// We'll also inject a generic anthropic endpoint into the config if it's supported,
	// but primaryApiKey="any" triggers its proxy mode if the proxy is running.
	// Or we explicitly set the base URL. Let's set it in the config just in case.
	config["apiEndpoint"] = fmt.Sprintf("http://%s/v1/anthropic/", gatewayAddr)

	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode claude config: %v", err)
	}

	return os.WriteFile(c.configPath, newData, 0644)
}

func (c *ClaudeCodeConfigurator) Restore() error {
	return restoreFile(c.configPath)
}

func (c *ClaudeCodeConfigurator) Status() (bool, bool, error) {
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

	apiKey, ok := config["primaryApiKey"].(string)
	isConfigured := ok && apiKey == "any"

	return isConfigured, hasBak, nil
}
