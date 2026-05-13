package sdk

//go:generate moq -fmt goimports -stub -out config_mock_test.go . Config

// Config carries configuration data into extension factories.
type Config interface {
	FilePath() string
	ProjectDir() string
	ExtensionConfig(scope, name string, target any, envPrefix string) error
	IsHeadless() bool
	Preferences(target any) error
	SavePreferences(target any) error
	SaveProviderKey(providerName, apiKey string) error
	RespectGitignore() bool
}

// NoopConfig is a nil-safe Config implementation that returns empty/zero values.
type NoopConfig struct{}

func (NoopConfig) FilePath() string                                   { return "" }
func (NoopConfig) ProjectDir() string                                 { return "" }
func (NoopConfig) ExtensionConfig(_, _ string, _ any, _ string) error { return nil }
func (NoopConfig) IsHeadless() bool                                   { return true }
func (NoopConfig) Preferences(any) error                              { return nil }
func (NoopConfig) SavePreferences(any) error                          { return nil }
func (NoopConfig) SaveProviderKey(_, _ string) error                  { return nil }
func (NoopConfig) RespectGitignore() bool                             { return true }

type noopConfig = NoopConfig

// FilePathConfig is a Config that returns the given path from FilePath().
type FilePathConfig string

func (f FilePathConfig) FilePath() string                                   { return string(f) }
func (f FilePathConfig) ProjectDir() string                                 { return "" }
func (f FilePathConfig) ExtensionConfig(_, _ string, _ any, _ string) error { return nil }
func (f FilePathConfig) IsHeadless() bool                                   { return true }
func (f FilePathConfig) Preferences(any) error                              { return nil }
func (f FilePathConfig) SavePreferences(any) error                          { return nil }
func (f FilePathConfig) SaveProviderKey(_, _ string) error                  { return nil }
func (f FilePathConfig) RespectGitignore() bool                             { return true }

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
