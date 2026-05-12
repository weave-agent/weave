package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nniel-ape/gonfig"

	"weave/internal/auth"
	"weave/sdk"
)

// SandboxFileConfig holds sandbox configuration from the config file.
type SandboxFileConfig struct {
	Mode      string   `json:"mode" description:"Sandbox mode: off, readonly, ask, auto"`
	Writable  []string `json:"writable" description:"Paths allowed for writes (default: CWD)"`
	DenyWrite []string `json:"deny_write" description:"Additional paths to block from writes"`
	DenyRead  []string `json:"deny_read" description:"Paths to block from reading"`
	Network   *bool    `json:"network" description:"Allow network access in sandbox"`
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
	filePath    string
	settings    *Settings
	projectDir  string
	layeredOnce sync.Once
	layered     *Settings
	layeredErr  error
}

// SetProjectDir overrides the project directory used for layered settings resolution.
// When set, it takes precedence over the directory derived from the config file path.
func (c *FullConfig) SetProjectDir(dir string) {
	c.projectDir = dir
	c.layeredOnce = sync.Once{}
	c.layered = nil
	c.layeredErr = nil
}

func (c *FullConfig) getLayeredSettings() (*Settings, error) {
	c.layeredOnce.Do(func() {
		projectDir := c.effectiveProjectDir()

		globalPath, err := SettingsPath()
		if err != nil {
			c.layered, c.layeredErr = nil, fmt.Errorf("global settings path: %w", err)
			return
		}

		global, err := loadSettingsFile(globalPath)
		if err != nil {
			c.layered, c.layeredErr = nil, fmt.Errorf("load global settings: %w", err)
			return
		}

		local, err := loadLocalSettings(projectDir)
		if err != nil {
			c.layered, c.layeredErr = nil, fmt.Errorf("load local settings: %w", err)
			return
		}

		c.layered = MergeSettings(global, c.settings, local)
	})

	return c.layered, c.layeredErr
}

var _ sdk.Config = (*FullConfig)(nil)

func (c *FullConfig) FilePath() string { return c.filePath }

func (c *FullConfig) ProjectDir() string { return c.effectiveProjectDir() }

// effectiveProjectDir returns the override project dir if set, otherwise derives
// it from the config file path.
func (c *FullConfig) effectiveProjectDir() string {
	if c.projectDir != "" {
		return c.projectDir
	}

	return ProjectDirFromConfig(c.filePath)
}

func (c *FullConfig) ResolveKey(providerName, envVar string) (string, error) {
	apiKey := c.resolveProviderAPIKey(providerName)

	return ResolveProviderKey(providerName, envVar, apiKey)
}

func (c *FullConfig) resolveProviderAPIKey(providerName string) string {
	layered, err := c.getLayeredSettings()
	if err == nil {
		if key := extractAPIKey(layered.Providers, providerName); key != "" {
			return key
		}
	}

	return extractAPIKey(c.settings.Providers, providerName)
}

func extractAPIKey(providers map[string]any, name string) string {
	if providers == nil {
		return ""
	}

	raw, ok := providers[name]
	if !ok {
		return ""
	}

	m, ok := raw.(map[string]any)
	if !ok {
		return ""
	}

	key, _ := m["api_key"].(string)

	return key
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

func (c *FullConfig) ExtensionConfig(scope, name string, target any, envPrefix string) error {
	layered, err := c.getLayeredSettings()
	if err != nil {
		return fmt.Errorf("load settings for %s.%s: %w", scope, name, err)
	}

	var raw any

	switch scope {
	case "tools":
		if layered.Tools != nil {
			raw = layered.Tools[name]
		}
	case "providers":
		if layered.Providers != nil {
			raw = layered.Providers[name]
		}
	case "ui":
		raw = layered.UI
	case "extensions":
		// Extensions config not yet supported in layered settings.
		return nil
	default:
		return nil
	}

	loader := Loader{
		Data:      toMapAny(raw),
		EnvPrefix: envPrefix,
	}

	return loader.Load(target)
}

func toMapAny(raw any) map[string]any {
	if raw == nil {
		return nil
	}

	m, ok := raw.(map[string]any)
	if ok {
		return m
	}

	// If it's not already a map, try JSON round-trip.
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil
	}

	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil
	}

	return result
}

func (c *FullConfig) IsHeadless() bool { return false }

func (c *FullConfig) RespectGitignore() bool {
	layered, err := c.getLayeredSettings()
	if err != nil {
		return true
	}

	if layered.RespectGitignore == nil {
		return true
	}

	return *layered.RespectGitignore
}

func (c *FullConfig) Preferences(target any) error {
	layered, err := c.getLayeredSettings()
	if err != nil {
		return fmt.Errorf("load preferences: %w", err)
	}

	loader := Loader{
		Data: toMapAny(layered),
	}

	return loader.Load(target)
}

func (c *FullConfig) SaveProviderKey(providerName, apiKey string) error {
	if err := auth.SetProviderKey(providerName, apiKey); err != nil {
		return fmt.Errorf("save provider key: %w", err)
	}

	return nil
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
