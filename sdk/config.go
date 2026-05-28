package sdk

//go:generate moq -fmt goimports -stub -out config_mock_test.go . Config

// Config carries configuration data into extension factories.
type Config interface {
	FilePath() string
	ProjectDir() string
	// ExtensionConfig loads scoped extension configuration into target. Concrete
	// implementations may also persist missing schema defaults for discoverability.
	ExtensionConfig(scope, name string, target any) error
	IsHeadless() bool
	RespectGitignore() bool
}

// PreferenceReader provides read-only access to user preferences.
type PreferenceReader interface {
	Preferences(target any) error
}

// PreferenceWriter extends PreferenceReader with write operations for
// preferences and provider credentials.
type PreferenceWriter interface {
	PreferenceReader
	SavePreferences(target any) error
	SaveProviderKey(providerName, apiKey string) error
}

// ExtensionConfigWriter optionally lets privileged extensions persist scoped
// configuration to the active settings layer.
type ExtensionConfigWriter interface {
	SaveExtensionConfig(scope, name string, target any) error
}

// PreferenceStore is the full interface for backward compatibility.
type PreferenceStore interface {
	PreferenceWriter
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
func (NoopPreferenceStore) SaveExtensionConfig(_, _ string, _ any) error {
	return nil
}

// FilePathConfig is a Config that returns the given path from FilePath().
type FilePathConfig string

func (f FilePathConfig) FilePath() string                         { return string(f) }
func (f FilePathConfig) ProjectDir() string                       { return "" }
func (f FilePathConfig) ExtensionConfig(_, _ string, _ any) error { return nil }
func (f FilePathConfig) IsHeadless() bool                         { return true }
func (f FilePathConfig) RespectGitignore() bool                   { return true }

func ConfigOrDefault(cfg Config) Config {
	if cfg != nil {
		return cfg
	}

	return NoopConfig{}
}

// configOnly wraps a Config so it cannot be type-asserted to PreferenceWriter.
// This prevents tool and extension factories from bypassing the read-only
// PreferenceReader boundary via the Config parameter.
type configOnly struct {
	cfg Config
}

func (c configOnly) FilePath() string   { return c.cfg.FilePath() }
func (c configOnly) ProjectDir() string { return c.cfg.ProjectDir() }
func (c configOnly) ExtensionConfig(s, n string, t any) error {
	return c.cfg.ExtensionConfig(s, n, t) //nolint:wrapcheck // transparent delegation
}
func (c configOnly) IsHeadless() bool       { return c.cfg.IsHeadless() }
func (c configOnly) RespectGitignore() bool { return c.cfg.RespectGitignore() }

// ConfigReadOnly returns a Config that delegates to the given config but does
// not expose PreferenceWriter methods, preventing type-assertion to write-capable
// interfaces.
func ConfigReadOnly(cfg Config) Config {
	if cfg == nil {
		return NoopConfig{}
	}

	return configOnly{cfg: cfg}
}

// preferenceReaderOnly wraps a PreferenceReader so it cannot be type-asserted
// back to PreferenceWriter. This enforces the capability boundary passed to
// tool and extension factories.
type preferenceReaderOnly struct {
	pr PreferenceReader
}

func (w preferenceReaderOnly) Preferences(target any) error {
	return w.pr.Preferences(target) //nolint:wrapcheck // transparent delegation
}

func PreferenceStoreFrom(cfg Config) PreferenceReader {
	if ps, ok := cfg.(PreferenceStore); ok {
		return preferenceReaderOnly{pr: ps}
	}

	return NoopPreferenceStore{}
}

// asPreferenceWriter extracts the underlying PreferenceWriter from a
// PreferenceReader when the reader wraps a PreferenceStore. Returns false if
// no writer is available. Used internally by privileged registration paths
// (RegisterExtensionWithScopeAndWriter) so that only explicitly declared
// extensions receive write access.
func asPreferenceWriter(pr PreferenceReader) (PreferenceWriter, bool) {
	if wrapped, ok := pr.(preferenceReaderOnly); ok {
		if pw, ok := wrapped.pr.(PreferenceWriter); ok {
			return pw, true
		}
	}

	pw, ok := pr.(PreferenceWriter)

	return pw, ok
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

func (h HeadlessConfig) SaveExtensionConfig(scope, name string, target any) error {
	if writer, ok := h.Config.(ExtensionConfigWriter); ok {
		return writer.SaveExtensionConfig(scope, name, target) //nolint:wrapcheck // transparent delegation
	}

	return NoopPreferenceStore{}.SaveExtensionConfig(scope, name, target)
}
