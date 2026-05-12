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
	ProjectDir() string
	ProviderConfig(name string) *ProviderConfigEntry
	ResolveKey(providerName, envVar string) (string, error)
	ToolConfig(name string, target any) error
	UIConfig(target any) error
	IsHeadless() bool
	Preferences(target any) error
	SavePreferences(target any) error
	SaveProviderKey(providerName, apiKey string) error
	RespectGitignore() bool
}

type noopConfig struct{}

func (noopConfig) FilePath() string                           { return "" }
func (noopConfig) ProjectDir() string                         { return "" }
func (noopConfig) ProviderConfig(string) *ProviderConfigEntry { return nil }
func (noopConfig) ResolveKey(_, envVar string) (string, error) {
	return os.Getenv(envVar), nil
}
func (noopConfig) ToolConfig(string, any) error      { return nil }
func (noopConfig) UIConfig(any) error                { return nil }
func (noopConfig) IsHeadless() bool                  { return true }
func (noopConfig) Preferences(any) error             { return nil }
func (noopConfig) SavePreferences(any) error         { return nil }
func (noopConfig) SaveProviderKey(_, _ string) error { return nil }
func (noopConfig) RespectGitignore() bool            { return true }

// FilePathConfig is a Config that returns the given path from FilePath().
type FilePathConfig string

func (f FilePathConfig) FilePath() string                           { return string(f) }
func (f FilePathConfig) ProjectDir() string                         { return "" }
func (f FilePathConfig) ProviderConfig(string) *ProviderConfigEntry { return nil }
func (f FilePathConfig) ResolveKey(_, envVar string) (string, error) {
	return os.Getenv(envVar), nil
}
func (f FilePathConfig) ToolConfig(string, any) error      { return nil }
func (f FilePathConfig) UIConfig(any) error                { return nil }
func (f FilePathConfig) IsHeadless() bool                  { return true }
func (f FilePathConfig) Preferences(any) error             { return nil }
func (f FilePathConfig) SavePreferences(any) error         { return nil }
func (f FilePathConfig) SaveProviderKey(_, _ string) error { return nil }
func (f FilePathConfig) RespectGitignore() bool            { return true }

func configOrDefault(cfg Config) Config {
	if cfg != nil {
		return cfg
	}

	return noopConfig{}
}

// HeadlessConfig wraps a Config and overrides IsHeadless.
type HeadlessConfig struct {
	Config
	Headless bool
}

func (h HeadlessConfig) IsHeadless() bool { return h.Headless }
