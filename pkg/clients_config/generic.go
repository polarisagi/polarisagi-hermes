package clients_config

// GenericConfigurator provides fallback instructions for unknown clients.
type GenericConfigurator struct{}

func NewGenericConfigurator() *GenericConfigurator {
	return &GenericConfigurator{}
}

func (c *GenericConfigurator) Apply(gatewayAddr string) error {
	// Generic configurator does not modify local files.
	// It relies on users exporting environment variables.
	// In the future, this could generate an "env.sh" script locally.
	return nil
}

func (c *GenericConfigurator) Restore() error {
	return nil
}

func (c *GenericConfigurator) Status() (bool, bool, error) {
	// Since generic clients are configured via environment vars dynamically,
	// we assume false or rely on the dashboard to provide the guide.
	return false, false, nil
}

// GetGenericEnvMap returns the environment variables required to configure a generic client
// to point to Polaris Hermes's OpenAI-compatible endpoint.
func GetGenericEnvMap(gatewayAddr string) map[string]string {
	return map[string]string{
		"OPENAI_API_KEY":  "polaris",
		"OPENAI_BASE_URL": "http://" + gatewayAddr + "/v1/openai/",
		// Some clients use OPENAI_API_BASE
		"OPENAI_API_BASE": "http://" + gatewayAddr + "/v1/openai/",
	}
}
