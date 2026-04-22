package sdk

import "os"

//go:generate moq -fmt goimports -stub -out config_mock_test.go . Config

// ProviderConfigEntry holds per-provider configuration from the config file.
type ProviderConfigEntry struct {
	Model     string
	MaxTokens int64
	BaseURL   string
	APIKey    string // raw value (may be !command or literal)
}

// Config carries configuration data into extension factories.
type Config interface {
	FilePath() string
	ProviderConfig(name string) *ProviderConfigEntry
	ResolveKey(providerName, envVar string) (string, error)
}

type noopConfig struct{}

func (noopConfig) FilePath() string                           { return "" }
func (noopConfig) ProviderConfig(string) *ProviderConfigEntry { return nil }
func (noopConfig) ResolveKey(_, envVar string) (string, error) {
	return os.Getenv(envVar), nil
}

// FilePathConfig is a Config that returns the given path from FilePath().
type FilePathConfig string

func (f FilePathConfig) FilePath() string                           { return string(f) }
func (f FilePathConfig) ProviderConfig(string) *ProviderConfigEntry { return nil }
func (f FilePathConfig) ResolveKey(_, envVar string) (string, error) {
	return os.Getenv(envVar), nil
}

func configOrDefault(cfg Config) Config {
	if cfg != nil {
		return cfg
	}

	return noopConfig{}
}
