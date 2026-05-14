package sdk

//go:generate moq -fmt goimports -stub -out config_mock_test.go . Config

// Config carries configuration data into extension factories.
type Config interface {
	FilePath() string
	ProjectDir() string
	ExtensionConfig(scope, name string, target any) error
	IsHeadless() bool
	RespectGitignore() bool
}

// PreferenceStore provides access to user preferences and provider keys.
type PreferenceStore interface {
	Preferences(target any) error
	SavePreferences(target any) error
	SaveProviderKey(providerName, apiKey string) error
}

// NoopConfig is a nil-safe Config implementation that returns empty/zero values.
type NoopConfig struct{}

func (NoopConfig) FilePath() string                         { return "" }
func (NoopConfig) ProjectDir() string                       { return "" }
func (NoopConfig) ExtensionConfig(_, _ string, _ any) error { return nil }
func (NoopConfig) IsHeadless() bool                         { return true }
func (NoopConfig) RespectGitignore() bool                   { return true }

// NoopPreferenceStore is a nil-safe PreferenceStore that returns empty values.
type NoopPreferenceStore struct{}

func (NoopPreferenceStore) Preferences(any) error             { return nil }
func (NoopPreferenceStore) SavePreferences(any) error         { return nil }
func (NoopPreferenceStore) SaveProviderKey(_, _ string) error { return nil }

// FilePathConfig is a Config that returns the given path from FilePath().
type FilePathConfig string

func (f FilePathConfig) FilePath() string                         { return string(f) }
func (f FilePathConfig) ProjectDir() string                       { return "" }
func (f FilePathConfig) ExtensionConfig(_, _ string, _ any) error { return nil }
func (f FilePathConfig) IsHeadless() bool                         { return true }
func (f FilePathConfig) RespectGitignore() bool                   { return true }

func configOrDefault(cfg Config) Config {
	if cfg != nil {
		return cfg
	}

	return NoopConfig{}
}

func preferenceStoreFrom(cfg Config) PreferenceStore {
	if ps, ok := cfg.(PreferenceStore); ok {
		return ps
	}

	return NoopPreferenceStore{}
}

// HeadlessConfig wraps a Config and overrides IsHeadless.
type HeadlessConfig struct {
	Config
	Headless bool
}

func (h HeadlessConfig) IsHeadless() bool { return h.Headless }

func (h HeadlessConfig) Preferences(target any) error {
	if ps, ok := h.Config.(PreferenceStore); ok {
		return ps.Preferences(target) //nolint:wrapcheck // transparent delegation
	}

	return NoopPreferenceStore{}.Preferences(target)
}

func (h HeadlessConfig) SavePreferences(target any) error {
	if ps, ok := h.Config.(PreferenceStore); ok {
		return ps.SavePreferences(target) //nolint:wrapcheck // transparent delegation
	}

	return NoopPreferenceStore{}.SavePreferences(target)
}

func (h HeadlessConfig) SaveProviderKey(providerName, apiKey string) error {
	if ps, ok := h.Config.(PreferenceStore); ok {
		return ps.SaveProviderKey(providerName, apiKey) //nolint:wrapcheck // transparent delegation
	}

	return NoopPreferenceStore{}.SaveProviderKey(providerName, apiKey)
}
