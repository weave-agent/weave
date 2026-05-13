package openai

// AuthConfig holds authentication credentials for the OpenAI provider.
type AuthConfig struct {
	APIKey string `json:"api_key" env:"OPENAI_API_KEY" description:"API key"`
}
