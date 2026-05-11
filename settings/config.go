package settings

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

type CoreConfig struct {
	AgentLoop string `default:"loop" description:"Agent loop extension name"`
}

// ProviderEntry holds per-provider configuration from the config file.
type ProviderEntry struct {
	APIKey    string `json:"api_key" description:"API key (literal, env var name, or !command)"`
	Model     string `json:"model" description:"Default model for this provider"`
	MaxTokens int64  `json:"max_tokens" description:"Max tokens for this provider"`
	BaseURL   string `json:"base_url" description:"Custom base URL (for OpenAI-compat providers)"`
}

// SandboxFileConfig holds sandbox configuration from the config file.
type SandboxFileConfig struct {
	Mode      string   `json:"mode" description:"Sandbox mode: off, readonly, ask, auto"`
	Writable  []string `json:"writable" description:"Paths allowed for writes (default: CWD)"`
	DenyWrite []string `json:"deny_write" description:"Additional paths to block from writes"`
	DenyRead  []string `json:"deny_read" description:"Paths to block from reading"`
	Network   *bool    `json:"network" description:"Allow network access in sandbox"`
}

// TypedProviders converts the Providers map[string]any to map[string]ProviderEntry
// via JSON round-trip (gonfig supports map[string]any, not map[string]ProviderEntry).
func (s *Settings) TypedProviders() map[string]ProviderEntry {
	result := make(map[string]ProviderEntry, len(s.Providers))

	for name, raw := range s.Providers {
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
func (s *Settings) ProviderConfig(name string) *ProviderEntry {
	tp := s.TypedProviders()

	e, ok := tp[name]
	if !ok {
		return nil
	}

	return &e
}

// DefaultSettings returns a Settings with sensible defaults.
func DefaultSettings() *Settings {
	return &Settings{
		UIExtension: "tui",
		AgentLoop:   "loop",
	}
}

// DefaultConfigJSON returns the default config as formatted JSON.
func DefaultConfigJSON() string {
	return `{
  "agent_loop": "loop",
  "ui_extension": "tui"
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

// EnsureGlobalConfig writes a default settings.json to ~/.weave/ if no global
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

	globalPath := filepath.Join(globalDir, "settings.json")
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

	return "", errors.New("no .weave/settings.json found")
}

func findGlobalConfig() (string, bool) {
	globalDir, err := GlobalConfigDir()
	if err != nil {
		return "", false
	}

	candidate := filepath.Join(globalDir, "settings.json")
	if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
		return candidate, true
	}

	return "", false
}

func findAnyConfigPath(startDir string) (string, bool) {
	dir := startDir

	for {
		candidate := filepath.Join(dir, ".weave", "settings.json")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}

		dir = parent
	}
}

func Load(args []string) (string, *Settings, []string, error) {
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

// flagSet holds CLI-only flags parsed separately from the settings file.
type flagSet struct {
	Prompt      string `short:"p" description:"Prompt to pass to the agent"`
	UI          string `flag:"ui" description:"UI extension name"`
	Output      string `flag:"output" description:"Output format: text or json"`
	Tools       string `flag:"tools" description:"Comma-separated tool allowlist"`
	SubagentID  string `flag:"subagent-id" description:"Subagent ID for inter-agent communication"`
	SandboxMode string `flag:"sandbox" description:"Sandbox mode override"`
	Model       string `flag:"model" description:"Model override for this session"`
}

func LoadFromDir(dir string, args []string) (string, *Settings, []string, error) {
	configPath, args := parseConfigFlag(args)

	path := configPath

	if path == "" {
		foundPath, found := findAnyConfigPath(dir)
		if found {
			path = foundPath
		}
	}

	// Fall back to global config (~/.weave/settings.json).
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
		return "", DefaultSettings(), args, nil
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	// Parse CLI flags separately to avoid conflicts with Settings JSON tags.
	var (
		flags flagSet
		rest  []string
	)
	if err := gonfig.Load(
		&flags,
		gonfig.WithFlags(args),
		gonfig.WithRemainingArgs(&rest),
	); err != nil {
		return "", nil, nil, fmt.Errorf("load config: %w", err)
	}

	// Load settings from file (env vars and defaults applied by gonfig).
	var s Settings
	if err := gonfig.Load(
		&s,
		gonfig.WithFile(path),
		gonfig.WithEnvPrefix("WEAVE"),
	); err != nil {
		return "", nil, nil, fmt.Errorf("load config: %w", err)
	}

	// Apply parsed CLI flags.
	s.Prompt = flags.Prompt
	if flags.UI != "" {
		s.UIExtension = flags.UI
	}

	s.Output = flags.Output
	s.ToolsFlag = flags.Tools
	s.SubagentID = flags.SubagentID
	s.SandboxMode = flags.SandboxMode
	s.ModelFlag = flags.Model

	// Detect explicitly empty --tools= so the launcher can forward it.
	for _, a := range args {
		if strings.HasPrefix(a, "--tools=") || a == "--tools" {
			s.ToolsSet = true
			break
		}
	}

	configDir := filepath.Dir(path)
	if err := ValidateWithConfigDir(&s, configDir); err != nil {
		return "", nil, nil, fmt.Errorf("validate config: %w", err)
	}

	return path, &s, rest, nil
}

// LoadFromFile loads a settings file from the given path without discovery or generation.
func LoadFromFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return &s, nil
}

// LoadFullConfig loads the config file, returning a FullConfig
// that implements sdk.Config with full key resolution.
func LoadFullConfig(path string) (*FullConfig, error) {
	if path == "" {
		return &FullConfig{settings: DefaultSettings()}, nil
	}

	s, err := LoadFromFile(path)
	if err != nil {
		return nil, err
	}

	return &FullConfig{filePath: path, settings: s}, nil
}

// FullConfig implements sdk.Config with auth resolution and provider config lookup.
type FullConfig struct {
	filePath   string
	settings   *Settings
	projectDir string
}

// SetProjectDir overrides the project directory used for layered settings resolution.
// When set, it takes precedence over the directory derived from the config file path.
func (c *FullConfig) SetProjectDir(dir string) {
	c.projectDir = dir
}

var _ sdk.Config = (*FullConfig)(nil)

func (c *FullConfig) FilePath() string { return c.filePath }

func (c *FullConfig) ProjectDir() string { return c.effectiveProjectDir() }

func (c *FullConfig) ProviderConfig(name string) *sdk.ProviderConfigEntry {
	e := c.settings.ProviderConfig(name)
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

// effectiveProjectDir returns the override project dir if set, otherwise derives
// it from the config file path.
func (c *FullConfig) effectiveProjectDir() string {
	if c.projectDir != "" {
		return c.projectDir
	}

	return ProjectDirFromConfig(c.filePath)
}

func (c *FullConfig) ResolveKey(providerName, envVar string) (string, error) {
	return ResolveProviderKey(providerName, envVar, c.settings.ProviderConfig(providerName))
}

// ProjectDirFromConfig returns the project root directory for a config file path.
// If the config file is inside .weave/ (e.g. .weave/settings.json), returns the
// parent directory so that layered settings look in the right place.
func ProjectDirFromConfig(configPath string) string {
	dir := filepath.Dir(configPath)
	if filepath.Base(dir) == ".weave" {
		return filepath.Dir(dir)
	}

	return dir
}

func (c *FullConfig) ToolConfig(name string, target any) error {
	applyDefaults(target)

	configDir := c.effectiveProjectDir()

	layered, err := LoadLayeredSettings(configDir)
	if err != nil {
		return fmt.Errorf("load settings for tool config: %w", err)
	}

	if layered.Tools == nil {
		return nil
	}

	raw, ok := layered.Tools[name]
	if !ok {
		return nil
	}

	return populateConfig(raw, target)
}

func (c *FullConfig) UIConfig(target any) error {
	applyDefaults(target)

	configDir := c.effectiveProjectDir()

	layered, err := LoadLayeredSettings(configDir)
	if err != nil {
		return fmt.Errorf("load settings for UI config: %w", err)
	}

	if len(layered.UI) == 0 {
		return nil
	}

	return populateConfig(layered.UI, target)
}

func (c *FullConfig) IsHeadless() bool { return false }

func (c *FullConfig) RespectGitignore() bool {
	configDir := c.effectiveProjectDir()

	layered, err := LoadLayeredSettings(configDir)
	if err != nil {
		return true
	}

	if layered.RespectGitignore == nil {
		return true
	}

	return *layered.RespectGitignore
}

func (c *FullConfig) Preferences(target any) error {
	applyDefaults(target)

	configDir := c.effectiveProjectDir()

	layered, err := LoadLayeredSettings(configDir)
	if err != nil {
		return fmt.Errorf("load preferences: %w", err)
	}

	return populateConfig(layered, target)
}

func (c *FullConfig) SavePreferences(target any) error {
	existing, err := LoadSettings()
	if err != nil {
		return fmt.Errorf("load existing settings: %w", err)
	}

	raw, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("marshal preferences: %w", err)
	}

	var incoming map[string]any
	if unErr := json.Unmarshal(raw, &incoming); unErr != nil {
		return fmt.Errorf("unmarshal preferences: %w", unErr)
	}

	existingRaw, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal existing settings: %w", err)
	}

	var merged map[string]any
	if unErr := json.Unmarshal(existingRaw, &merged); unErr != nil {
		return fmt.Errorf("unmarshal existing settings: %w", unErr)
	}

	for k, v := range incoming {
		if prev, ok := merged[k]; ok {
			merged[k] = deepMergeValues(prev, v)
		} else {
			merged[k] = v
		}
	}

	finalRaw, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal merged settings: %w", err)
	}

	var final Settings
	if unErr := json.Unmarshal(finalRaw, &final); unErr != nil {
		return fmt.Errorf("unmarshal merged settings: %w", unErr)
	}

	return SaveSettingsGlobal(&final)
}

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
	if target == nil {
		return
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return
	}

	v = v.Elem()

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
