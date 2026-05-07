package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/nniel-ape/gonfig"

	"weave/sdk"
)

// String constants used across config validation and defaults.
const (
	UIValueTUI  = "tui"
	UIValueNone = "none"

	// DefaultAgentLoop is the default agent loop extension name.
	DefaultAgentLoop = "loop"
	// DefaultProvider is the default provider name.
	DefaultProvider = "anthropic"
	// ExtBash is the bash tool extension name.
	ExtBash = "bash"
)

type CoreConfig struct {
	AgentLoop string   `default:"loop" description:"Agent loop extension name"`
	Providers []string `default:"anthropic" description:"Provider extension names"`
}

// ProviderEntry holds per-provider configuration from the config file.
type ProviderEntry struct {
	APIKey    string `json:"api_key" description:"API key (literal, env var name, or !command)"`
	Model     string `json:"model" description:"Default model for this provider"`
	MaxTokens int64  `json:"max_tokens" description:"Max tokens for this provider"`
	BaseURL   string `json:"base_url" description:"Custom base URL (for OpenAI-compat providers)"`
}

type File struct {
	Extensions   []string       `short:"e" description:"List of optional extensions to load"`
	UIExtensions []string       `yaml:"ui_extensions" description:"List of UI-specific extensions to load when TUI is active"`
	Prompt       string         `short:"p" description:"Prompt to pass to the agent"`
	UI           string         `default:"tui" description:"UI extension name (tui for interactive, none for headless)"`
	Core         CoreConfig     `description:"Core agent configuration"`
	Providers    map[string]any `description:"Per-provider configuration"`
}

// CoreExts returns (coreExts, optionalExts) where coreExts contains the agent-loop
// and provider names, and optionalExts contains the user-specified extensions.
func (f *File) CoreExts() ([]string, []string) {
	core := make([]string, 0, 1+len(f.Core.Providers))
	core = append(core, f.Core.AgentLoop)
	core = append(core, f.Core.Providers...)

	return core, f.Extensions
}

// AllExtensions returns the complete merged list of all extensions: core
// (agent-loop + providers), optional extensions, and UI extensions (only
// when UI is "tui"). Duplicates are removed.
func (f *File) AllExtensions() []string {
	core, opt := f.CoreExts()

	totalLen := len(core) + len(opt)
	if f.UI == UIValueTUI {
		totalLen += len(f.UIExtensions)
	}

	seen := make(map[string]bool, totalLen)
	result := make([]string, 0, totalLen)

	for _, e := range core {
		if !seen[e] {
			seen[e] = true
			result = append(result, e)
		}
	}

	for _, e := range opt {
		if !seen[e] {
			seen[e] = true
			result = append(result, e)
		}
	}

	if f.UI == UIValueTUI {
		for _, e := range f.UIExtensions {
			if !seen[e] {
				seen[e] = true
				result = append(result, e)
			}
		}
	}

	return result
}

// TypedProviders converts the Providers map[string]any to map[string]ProviderEntry
// via JSON round-trip (gonfig supports map[string]any, not map[string]ProviderEntry).
func (f *File) TypedProviders() map[string]ProviderEntry {
	result := make(map[string]ProviderEntry, len(f.Providers))

	for name, raw := range f.Providers {
		jsonBytes, err := json.Marshal(raw)
		if err != nil {
			continue
		}

		var entry ProviderEntry
		if err := json.Unmarshal(jsonBytes, &entry); err != nil {
			continue
		}

		result[name] = entry
	}

	return result
}

// ProviderConfig returns the typed config entry for a named provider, or nil.
func (f *File) ProviderConfig(name string) *ProviderEntry {
	tp := f.TypedProviders()

	e, ok := tp[name]
	if !ok {
		return nil
	}

	return &e
}

// DefaultFile returns a File with all built-in extensions and sensible defaults.
func DefaultFile() *File {
	return &File{
		UI: UIValueTUI,
		Core: CoreConfig{
			AgentLoop: DefaultAgentLoop,
			Providers: []string{DefaultProvider},
		},
		Extensions: []string{
			"jsonl",
			"instructions",
			ExtBash,
			"edit",
			"find",
			"grep",
			"ls",
			"read",
			"write",
		},
		UIExtensions: []string{},
	}
}

// DefaultConfigJSON returns the default config as formatted JSON
// with all built-in extensions and providers.
func DefaultConfigJSON() string {
	return `{
  "core": {
    "agent_loop": "loop",
    "providers": ["anthropic", "openai", "zai"]
  },
  "ui": "tui",
  "extensions": [
    "jsonl",
    "instructions",
    "bash",
    "edit",
    "find",
    "grep",
    "ls",
    "read",
    "write"
  ],
  "ui_extensions": []
}`
}

// GlobalConfigDir returns ~/.weave.
func GlobalConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("global config dir: %w", err)
	}

	return filepath.Join(home, ".weave"), nil
}

// EnsureGlobalConfig writes a default config.json to ~/.weave/ if no global
// config exists and no project-local config is found.
// Returns the path to the newly created file, or "" if a config already exists.
func EnsureGlobalConfig(projectDir string) (string, error) {
	// If a project-local config exists, skip.
	if _, found := findAnyConfigPath(projectDir); found {
		return "", nil
	}

	// If a global config already exists, skip.
	globalDir, err := GlobalConfigDir()
	if err != nil {
		return "", err
	}

	globalPath := filepath.Join(globalDir, "config.json")
	if _, statErr := os.Stat(globalPath); statErr == nil {
		return "", nil
	}

	if err := os.MkdirAll(globalDir, 0o750); err != nil {
		return "", fmt.Errorf("create global config dir: %w", err)
	}

	if err := os.WriteFile(globalPath, []byte(DefaultConfigJSON()), 0o600); err != nil {
		return "", fmt.Errorf("write default config: %w", err)
	}

	return globalPath, nil
}

func FindConfigPath(startDir string) (string, error) {
	path, found := findAnyConfigPath(startDir)
	if found {
		return path, nil
	}

	// Also check global config.
	if globalPath, globalFound := findGlobalConfig(); globalFound {
		return globalPath, nil
	}

	return "", errors.New("no .weave.yaml, .weave/config.yaml, or .weave/config.json found")
}

func findGlobalConfig() (string, bool) {
	globalDir, err := GlobalConfigDir()
	if err != nil {
		return "", false
	}

	candidate := filepath.Join(globalDir, "config.json")
	if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
		return candidate, true
	}

	return "", false
}

func findAnyConfigPath(startDir string) (string, bool) {
	dir := startDir

	for {
		for _, name := range []string{".weave.yaml", ".weave/config.yaml", ".weave/config.json"} {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, true
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}

		dir = parent
	}
}

func Load(args []string) (string, *File, []string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, nil, fmt.Errorf("get working dir: %w", err)
	}

	return LoadFromDir(cwd, args)
}

// parseConfigFlag extracts -c/--config from args, returning the config path
// and remaining args.
func parseConfigFlag(args []string) (configPath string, rest []string) {
	for i := range args {
		if args[i] == "-c" || args[i] == "--config" {
			if i+1 < len(args) {
				return args[i+1], append(args[:i:i], args[i+2:]...)
			}
		} else if cfg, ok := strings.CutPrefix(args[i], "-c="); ok {
			return cfg, append(args[:i:i], args[i+1:]...)
		} else if cfg, ok := strings.CutPrefix(args[i], "--config="); ok {
			return cfg, append(args[:i:i], args[i+1:]...)
		}
	}

	return "", args
}

func LoadFromDir(dir string, args []string) (string, *File, []string, error) {
	configPath, args := parseConfigFlag(args)

	path := configPath

	if path == "" {
		foundPath, found := findAnyConfigPath(dir)
		if found {
			path = foundPath
		}
	}

	// Fall back to global config (~/.weave/config.json).
	if path == "" {
		if globalPath, globalFound := findGlobalConfig(); globalFound {
			path = globalPath
		}
	}

	// No config anywhere — generate a default global config.
	if path == "" {
		generatedPath, genErr := EnsureGlobalConfig(dir)
		if genErr != nil {
			return "", nil, nil, fmt.Errorf("generate default config: %w", genErr)
		}

		path = generatedPath
	}

	// Still nothing (shouldn't happen) — use in-memory defaults.
	if path == "" {
		return "", DefaultFile(), args, nil
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	var (
		f    File
		rest []string
	)

	if err := gonfig.Load(&f,
		gonfig.WithFile(path),
		gonfig.WithEnvPrefix("WEAVE"),
		gonfig.WithFlags(args),
		gonfig.WithRemainingArgs(&rest),
	); err != nil {
		return "", nil, nil, fmt.Errorf("load config: %w", err)
	}

	configDir := filepath.Dir(path)
	if err := ValidateWithConfigDir(&f, configDir); err != nil {
		return "", nil, nil, fmt.Errorf("validate config: %w", err)
	}

	return path, &f, rest, nil
}

// LoadFromFile loads a config file from the given path without discovery or generation.
func LoadFromFile(path string) (*File, error) {
	var f File
	if err := gonfig.Load(&f, gonfig.WithFile(path)); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}

	return &f, nil
}

// LoadFullConfig loads the config file and auth file, returning a FullConfig
// that implements sdk.Config with full key resolution.
func LoadFullConfig(path string) (*FullConfig, error) {
	auth, err := LoadAuth()
	if err != nil {
		return nil, fmt.Errorf("load auth: %w", err)
	}

	if path == "" {
		return &FullConfig{file: DefaultFile(), auth: auth}, nil
	}

	var f File
	if err := gonfig.Load(&f, gonfig.WithFile(path), gonfig.WithEnvPrefix("WEAVE")); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return &FullConfig{filePath: path, file: &f, auth: auth}, nil
}

// FullConfig implements sdk.Config with auth resolution and provider config lookup.
type FullConfig struct {
	filePath string
	file     *File
	auth     *AuthFile
}

var _ sdk.Config = (*FullConfig)(nil)

func (c *FullConfig) FilePath() string { return c.filePath }

func (c *FullConfig) ProviderConfig(name string) *sdk.ProviderConfigEntry {
	e := c.file.ProviderConfig(name)
	if e == nil {
		return nil
	}

	return &sdk.ProviderConfigEntry{
		Model:     e.Model,
		MaxTokens: e.MaxTokens,
		BaseURL:   e.BaseURL,
		APIKey:    e.APIKey,
	}
}

func (c *FullConfig) ResolveKey(providerName, envVar string) (string, error) {
	// Re-read auth file so keys set via /providers after startup are visible.
	auth, err := LoadAuth()
	if err != nil {
		auth = c.auth // fall back to cached snapshot
	}

	return ResolveProviderKey(providerName, envVar, c.file.ProviderConfig(providerName), auth)
}

// projectDirFromConfig returns the project root directory for a config file path.
// If the config file is inside .weave/ (e.g. .weave/config.yaml), returns the
// parent directory so that layered settings look in the right place.
func projectDirFromConfig(configPath string) string {
	dir := filepath.Dir(configPath)
	if filepath.Base(dir) == ".weave" {
		return filepath.Dir(dir)
	}

	return dir
}

func (c *FullConfig) ToolConfig(name string, target any) error {
	applyDefaults(target)

	configDir := projectDirFromConfig(c.filePath)

	settings, err := LoadLayeredSettings(configDir)
	if err != nil {
		return fmt.Errorf("load settings for tool config: %w", err)
	}

	if settings.Tools == nil {
		return nil
	}

	raw, ok := settings.Tools[name]
	if !ok {
		return nil
	}

	return populateConfig(raw, target)
}

func (c *FullConfig) UIConfig(target any) error {
	applyDefaults(target)

	configDir := projectDirFromConfig(c.filePath)

	settings, err := LoadLayeredSettings(configDir)
	if err != nil {
		return fmt.Errorf("load settings for UI config: %w", err)
	}

	if settings.UI == nil {
		return nil
	}

	return populateConfig(settings.UI, target)
}

func (c *FullConfig) IsHeadless() bool { return true }

func (c *FullConfig) Preferences(any) error               { return nil }
func (c *FullConfig) SavePreferences(any) error           { return nil }
func (c *FullConfig) ProviderHasKey(string) bool          { return false }
func (c *FullConfig) SetProviderKey(string, string) error { return nil }

// populateConfig JSON round-trips a map/struct into target and applies default
// struct tags for zero-value fields.
func populateConfig(raw, target any) error {
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, target); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	return nil
}

// applyDefaults sets zero-value fields from their `default` struct tags.
func applyDefaults(target any) {
	v := reflect.ValueOf(target).Elem()

	t := v.Type()

	for i := range v.NumField() {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}

		defTag := t.Field(i).Tag.Get("default")
		if defTag == "" {
			continue
		}

		if !field.IsZero() {
			continue
		}

		switch field.Kind() {
		case reflect.String:
			field.SetString(defTag)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if n, err := strconv.ParseInt(defTag, 10, 64); err == nil {
				field.SetInt(n)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if n, err := strconv.ParseUint(defTag, 10, 64); err == nil {
				field.SetUint(n)
			}
		case reflect.Float32, reflect.Float64:
			if f, err := strconv.ParseFloat(defTag, 64); err == nil {
				field.SetFloat(f)
			}
		case reflect.Bool:
			if b, err := strconv.ParseBool(defTag); err == nil {
				field.SetBool(b)
			}
		default:
			// Unsupported kind for default tag; skip.
		}
	}
}
