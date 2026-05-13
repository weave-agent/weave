package anthropic

// AuthConfig holds authentication credentials for the Anthropic provider.
type AuthConfig struct {
	APIKey string `json:"api_key" env:"ANTHROPIC_API_KEY" description:"API key"`
}
