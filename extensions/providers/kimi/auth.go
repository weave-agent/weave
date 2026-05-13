package kimi

// AuthConfig holds authentication credentials for the Kimi provider.
type AuthConfig struct {
	APIKey string `json:"api_key" env:"KIMI_API_KEY" description:"API key"`
}
