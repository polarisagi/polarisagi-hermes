package clients_config

import (
	"fmt"
	"strings"
)

// ClientConfigurator defines the interface for configuring different AI clients
type ClientConfigurator interface {
	// Apply applies the configuration to point to Polaris Hermes.
	// gatewayAddr is the base listen address of the gateway (e.g., "127.0.0.1:28888").
	Apply(gatewayAddr string) error

	// Restore restores the original configuration from the backup file in the client's directory.
	Restore() error

	// Status returns the current state of the client configuration.
	// isConfigured indicates if the client is pointing to Polaris.
	// hasBackup indicates if a backup file exists.
	Status() (isConfigured bool, hasBackup bool, err error)
}

// ClientStatus represents the status of a specific client
type ClientStatus struct {
	Name         string `json:"name"`
	IsConfigured bool   `json:"is_configured"`
	HasBackup    bool   `json:"has_backup"`
	Error        string `json:"error,omitempty"`
}

// GetConfigurator returns the configurator for the given client name.
func GetConfigurator(clientName string) (ClientConfigurator, error) {
	switch strings.ToLower(clientName) {
	case "opencode":
		return NewOpenCodeConfigurator(), nil
	case "claude_code", "claude":
		return NewClaudeCodeConfigurator(), nil
	case "codex":
		return NewCodexConfigurator(), nil
	case "gemini":
		return NewGeminiConfigurator(), nil
	case "openclaw":
		return NewOpenClawConfigurator(), nil
	case "hermes":
		return NewHermesConfigurator(), nil
	case "generic":
		return NewGenericConfigurator(), nil
	default:
		return nil, fmt.Errorf("unsupported client: %s", clientName)
	}
}

// GetAllSupportedClients returns a list of all supported client names.
func GetAllSupportedClients() []string {
	return []string{"opencode", "claude_code", "codex", "gemini", "openclaw", "hermes", "generic"}
}
