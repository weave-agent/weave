package zai

// AuthConfig holds authentication credentials for the Z.ai provider.
type AuthConfig struct {
	APIKey string `json:"api_key" env:"ZAI_API_KEY" validate:"required" description:"API key"`
}
