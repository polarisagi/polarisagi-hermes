package clients_config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type GeminiConfigurator struct {
	envPath      string
	settingsPath string
}

func NewGeminiConfigurator() *GeminiConfigurator {
	home, _ := getUserHomeDir()
	geminiDir := filepath.Join(home, ".gemini")
	return &GeminiConfigurator{
		envPath:      filepath.Join(geminiDir, ".env"),
		settingsPath: filepath.Join(geminiDir, "settings.json"),
	}
}

func (c *GeminiConfigurator) Apply(gatewayAddr string) error {
	if err := ensureDirExists(c.envPath); err != nil {
		return err
	}

	// 1. Configure .env
	if err := backupFile(c.envPath); err != nil {
		return fmt.Errorf("failed to backup gemini .env: %v", err)
	}

	envMap, err := godotenv.Read(c.envPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read gemini .env: %v", err)
		}
		envMap = make(map[string]string)
	}

	envMap["GEMINI_API_KEY"] = "polaris"
	envMap["GOOGLE_GEMINI_BASE_URL"] = fmt.Sprintf("http://%s/v1/google/", gatewayAddr)

	if err := godotenv.Write(envMap, c.envPath); err != nil {
		return fmt.Errorf("failed to write gemini .env: %v", err)
	}

	// 2. Configure settings.json
	if err := backupFile(c.settingsPath); err != nil {
		return fmt.Errorf("failed to backup gemini settings.json: %v", err)
	}

	var settings map[string]interface{}
	data, err := os.ReadFile(c.settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("failed to parse gemini settings: %v", err)
		}
	} else if os.IsNotExist(err) {
		settings = make(map[string]interface{})
	} else {
		return err
	}

	security, _ := settings["security"].(map[string]interface{})
	if security == nil {
		security = make(map[string]interface{})
		settings["security"] = security
	}

	auth, _ := security["auth"].(map[string]interface{})
	if auth == nil {
		auth = make(map[string]interface{})
		security["auth"] = auth
	}

	auth["selectedType"] = "gemini-api-key"

	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode gemini settings: %v", err)
	}

	return os.WriteFile(c.settingsPath, newData, 0644)
}

func (c *GeminiConfigurator) Restore() error {
	if err := restoreFile(c.envPath); err != nil {
		// Even if one fails, try the other
		_ = restoreFile(c.settingsPath)
		return err
	}
	return restoreFile(c.settingsPath)
}

func (c *GeminiConfigurator) Status() (bool, bool, error) {
	hasBak := hasBackup(c.envPath) || hasBackup(c.settingsPath)

	envMap, err := godotenv.Read(c.envPath)
	if err != nil && !os.IsNotExist(err) {
		return false, hasBak, err
	}

	isConfigured := envMap["GEMINI_API_KEY"] == "polaris" && envMap["GOOGLE_GEMINI_BASE_URL"] != ""

	return isConfigured, hasBak, nil
}
