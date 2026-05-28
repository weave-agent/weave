package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/weave-agent/weave/internal/auth"
	"github.com/weave-agent/weave/sdk"
)

// DefaultEnvPrefix is the standard environment variable prefix for weave settings.
const DefaultEnvPrefix = "WEAVE"

const (
	configScopeTools        = "tools"
	configScopeProviders    = "providers"
	configScopeUI           = "ui"
	configScopeGuardian     = "guardian"
	configScopeSandbox      = "sandbox"
	configScopeJSONL        = "jsonl"
	configScopeExtensions   = "extensions"
	configScopeUIExtensions = "ui_extensions"
	removedSandboxModeKey   = "mode"
)

// GuardianFileConfig holds guardian policy configuration from the config file.
type GuardianFileConfig struct {
	Profile     string                         `json:"profile" env:"GUARDIAN_PROFILE" description:"Active guardian profile: ask, auto, yolo, or custom profile name"`
	AskFallback *bool                          `json:"ask_fallback" env:"GUARDIAN_ASK_FALLBACK" description:"Ask instead of blocking when no guardian policy matches"`
	Profiles    map[string]sdk.GuardianProfile `json:"profiles,omitempty" description:"Custom guardian profile definitions"`
}

// SandboxFilesystemConfig holds initial filesystem containment boundaries.
type SandboxFilesystemConfig struct {
	ReadOnly  []string `json:"read_only,omitempty" description:"Paths mounted read-only inside the sandbox"`
	ReadWrite []string `json:"read_write,omitempty" description:"Paths mounted read-write inside the sandbox"`
	Blocked   []string `json:"blocked,omitempty" description:"Paths blocked inside the sandbox"`
}

// SandboxNetworkConfig holds initial network containment boundaries.
type SandboxNetworkConfig struct {
	Enabled     *bool    `json:"enabled" description:"Allow network access from sandboxed processes"`
	AllowHosts  []string `json:"allow_hosts,omitempty" description:"Hosts allowed from sandboxed processes"`
	AllowPorts  []string `json:"allow_ports,omitempty" description:"Ports allowed from sandboxed processes"`
	BlockHosts  []string `json:"block_hosts,omitempty" description:"Hosts blocked from sandboxed processes"`
	AllowListen *bool    `json:"allow_listen,omitempty" description:"Allow sandboxed processes to listen on local ports"`
}

// SandboxFileConfig holds sandbox containment configuration from the config file.
type SandboxFileConfig struct {
	Enabled                  *bool                   `json:"enabled" env:"SANDBOX_ENABLED" description:"Enable OS-level containment for approved commands"`
	FailIfUnavailable        *bool                   `json:"fail_if_unavailable" env:"SANDBOX_FAIL_IF_UNAVAILABLE" description:"Fail commands when sandbox containment is unavailable"`
	AllowUnsandboxedFallback *bool                   `json:"allow_unsandboxed_fallback" env:"SANDBOX_ALLOW_UNSANDBOXED_FALLBACK" description:"Allow approved commands to run without containment when sandboxing is unavailable"`
	Filesystem               SandboxFilesystemConfig `json:"filesystem" description:"Filesystem containment boundaries"`
	Network                  SandboxNetworkConfig    `json:"network" description:"Network containment boundaries"`
}

// DefaultSettings returns a Settings with sensible defaults.
func DefaultSettings() *Settings {
	return &Settings{
		UIExtension: "tui",
		AgentLoop:   "agent",
	}
}

// DefaultConfigJSON returns the default config as formatted JSON.
func DefaultConfigJSON() string {
	return `{
  "agent_loop": "agent",
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
		if args[i] == "--config" {
			if i+1 < len(args) {
				return args[i+1], append(args[:i:i], args[i+2:]...)
			}
		} else if cfg, ok := strings.CutPrefix(args[i], "--config="); ok {
			return cfg, append(args[:i:i], args[i+1:]...)
		}
	}

	return "", args
}

// flagSet holds CLI-only flags parsed separately from the settings file.
type flagSet struct {
	Prompt          string `flag:"prompt" short:"p" description:"Prompt to pass to the agent"`
	UI              string `flag:"ui" description:"UI extension name"`
	Output          string `flag:"output" description:"Output format: text or json"`
	Tools           string `flag:"tools" description:"Comma-separated tool allowlist"`
	SubagentID      string `flag:"subagent-id" description:"Subagent ID for inter-agent communication"`
	GuardianProfile string `flag:"guardian-profile" description:"Guardian profile override"`
	Model           string `flag:"model" description:"Model override for this session"`
	Debug           bool   `flag:"debug" description:"Enable debug logging"`
	Continue        bool   `flag:"continue" short:"c" description:"Resume most recent session"`
	Resume          string `flag:"resume" short:"r" description:"Resume specific session by ID"`
	SkipBootstrap   bool   `flag:"skip-bootstrap" description:"Skip auto-install of core extensions on first run"`
}

func loadSettingsFromFile(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return Settings{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := rejectRemovedSandboxKeys(raw); err != nil {
		return Settings{}, err
	}

	var s Settings

	loader := Loader{
		Data:      raw,
		EnvPrefix: DefaultEnvPrefix,
	}
	if err := loader.Load(&s); err != nil {
		return Settings{}, fmt.Errorf("load config: %w", err)
	}

	return s, nil
}

func rejectRemovedSandboxKeys(raw map[string]any) error {
	sandboxRaw, ok := raw[configScopeSandbox]
	if !ok || sandboxRaw == nil {
		return nil
	}

	sandbox, ok := sandboxRaw.(map[string]any)
	if !ok {
		return nil
	}

	for _, key := range []string{removedSandboxModeKey, "writable", "deny_read", "deny_write"} {
		if _, ok := sandbox[key]; ok {
			return fmt.Errorf("unsupported sandbox config key %q: the sandbox mode API was removed; use guardian.profile and sandbox containment settings", key)
		}
	}

	return nil
}

func resolveConfigPath(dir, configPath string) (string, error) {
	path := configPath

	if path == "" {
		if foundPath, found := findAnyConfigPath(dir); found {
			path = foundPath
		}
	}

	if path == "" {
		if globalPath, globalFound := findGlobalConfig(); globalFound {
			path = globalPath
		}
	}

	if path == "" {
		generatedPath, err := EnsureGlobalConfig(dir)
		if err != nil {
			return "", fmt.Errorf("generate default config: %w", err)
		}

		path = generatedPath
	}

	return path, nil
}

func LoadFromDir(dir string, args []string) (string, *Settings, []string, error) {
	configPath, args := parseConfigFlag(args)
	if err := rejectRemovedSandboxRuntimeInputs(args); err != nil {
		return "", nil, nil, err
	}

	path, err := resolveConfigPath(dir, configPath)
	if err != nil {
		return "", nil, nil, err
	}

	if path == "" {
		return "", DefaultSettings(), args, nil
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	var flags flagSet

	rest, err := applyFlags(&flags, args)
	if err != nil {
		return "", nil, nil, fmt.Errorf("load config: %w", err)
	}

	var s Settings

	if path != "" {
		s, err = loadSettingsFromFile(path)
		if err != nil {
			return "", nil, nil, err
		}
	} else {
		s = *DefaultSettings()
	}

	// Apply parsed CLI flags.
	s.Prompt = flags.Prompt
	if flags.UI != "" {
		s.UIExtension = flags.UI
	}

	s.Output = flags.Output
	s.ToolsFlag = flags.Tools
	s.SubagentID = flags.SubagentID
	s.GuardianProfile = flags.GuardianProfile
	s.ModelFlag = flags.Model
	s.Debug = flags.Debug
	s.Continue = flags.Continue
	s.Resume = flags.Resume
	s.SkipBootstrap = flags.SkipBootstrap

	if s.Continue && s.Resume != "" {
		return "", nil, nil, errors.New("--continue and --resume are mutually exclusive")
	}

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

func rejectRemovedSandboxRuntimeInputs(args []string) error {
	if _, ok := os.LookupEnv("WEAVE_SANDBOX_MODE"); ok {
		return errors.New("unsupported WEAVE_SANDBOX_MODE: the sandbox mode API was removed; use WEAVE_GUARDIAN_PROFILE and sandbox containment settings")
	}

	removedFlags := []string{
		"--sandbox",
		"--weave-sandbox-mode",
		"--sandbox-mode",
		"--sandbox-writable",
		"--sandbox-deny_read",
		"--sandbox-deny_write",
		"--sandbox-network",
	}

	for _, arg := range args {
		if arg == "--" {
			break
		}

		for _, flag := range removedFlags {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return fmt.Errorf("unsupported %s: the sandbox mode API was removed; use --guardian-profile and sandbox containment settings", flag)
			}
		}
	}

	return nil
}

// LoadFromFile loads a settings file from the given path without discovery or generation.
func LoadFromFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := rejectRemovedSandboxKeys(raw); err != nil {
		return nil, err
	}

	var s Settings

	loader := Loader{
		Data:      raw,
		EnvPrefix: DefaultEnvPrefix,
	}
	if err := loader.Load(&s); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return &s, nil
}

// LoadFullConfig loads the config file, returning a FullConfig
// that implements sdk.Config with full key resolution.
func LoadFullConfig(path string) (*FullConfig, error) {
	if path == "" {
		return &FullConfig{settings: &Settings{}}, nil
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
	args        []string
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

// SetArgs stores remaining CLI args for extension-specific flag parsing.
func (c *FullConfig) SetArgs(args []string) {
	c.args = args
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

var (
	_ sdk.Config                = (*FullConfig)(nil)
	_ sdk.ExtensionConfigWriter = (*FullConfig)(nil)
)

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

func (c *FullConfig) resolveSourcePath() (string, error) {
	projectDir := c.effectiveProjectDir()

	if path, found, err := findLocalSettingsPath(projectDir); err != nil {
		return "", err
	} else if found {
		return path, nil
	}

	if c.filePath == "" {
		path, err := SettingsPath()
		if err != nil {
			return "", err
		}

		return path, nil
	}

	return c.filePath, nil
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

// ExtensionConfig loads scoped extension configuration into target and writes
// any missing schema-declared defaults back to the active settings file.
func (c *FullConfig) ExtensionConfig(scope, name string, target any) error {
	layered, err := c.getLayeredSettings()
	if err != nil {
		return fmt.Errorf("load settings for %s.%s: %w", scope, name, err)
	}

	var raw any

	switch scope {
	case configScopeTools:
		if layered.Tools != nil {
			raw = layered.Tools[name]
		}
	case configScopeProviders:
		if layered.Providers != nil {
			raw = layered.Providers[name]
		}
	case configScopeUI:
		raw = layered.UI
	case configScopeGuardian:
		raw = layered.Guardian
	case configScopeSandbox:
		raw = layered.Sandbox
	case configScopeJSONL:
		raw = layered.JSONL
	case configScopeExtensions:
		if layered.Extensions != nil {
			raw = layered.Extensions[name]
		}
	case configScopeUIExtensions:
		if layered.UIExtensions != nil {
			raw = layered.UIExtensions[name]
		}
	default:
		return fmt.Errorf("unknown config scope %q", scope)
	}

	data, err := toMapAny(raw)
	if err != nil {
		return fmt.Errorf("convert config for %s.%s: %w", scope, name, err)
	}

	envPrefix := extensionEnvPrefix(scope, name)

	loader := Loader{
		Data:      data,
		EnvPrefix: envPrefix,
		Args:      filterExtensionArgs(c.args, extensionFlagPrefixName(scope, name)),
	}

	if loadErr := loader.Load(target); loadErr != nil {
		return loadErr
	}

	sourcePath, err := c.resolveSourcePath()
	if err != nil {
		slog.Warn("populate extension defaults: resolve source settings failed", "scope", scope, "name", name, "error", err)

		return nil
	}

	existing, err := c.existingConfigForPopulation(scope, name)
	if err != nil {
		slog.Warn("populate extension defaults: read existing settings failed", "scope", scope, "name", name, "error", err)
	}

	if err := populateExtensionDefaults(sourcePath, scope, name, existing); err != nil {
		slog.Warn("populate extension defaults failed", "scope", scope, "name", name, "path", sourcePath, "error", err)
	}

	return nil
}

func (c *FullConfig) existingConfigForPopulation(scope, name string) (map[string]any, error) {
	paths := []string{}

	globalPath, err := SettingsPath()
	if err != nil {
		return nil, fmt.Errorf("global settings path: %w", err)
	}

	paths = append(paths, globalPath)

	if c.filePath != "" && c.filePath != globalPath {
		paths = append(paths, c.filePath)
	}

	if localPath, found, err := findLocalSettingsPath(c.effectiveProjectDir()); err != nil {
		return nil, err
	} else if found && localPath != c.filePath {
		paths = append(paths, localPath)
	}

	merged := map[string]any{}
	seen := map[string]bool{}

	for _, path := range paths {
		if seen[path] {
			continue
		}

		seen[path] = true

		root, err := readSettingsMap(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}

		merged = deepMergeValues(merged, root).(map[string]any)
	}

	return configMapForScope(merged, scope, name)
}

func extensionFlagPrefixName(scope, name string) string {
	if name != "" {
		return name
	}

	return scope
}

func extensionEnvPrefix(scope, name string) string {
	switch scope {
	case configScopeProviders:
		return ""
	case configScopeTools, configScopeExtensions, configScopeUIExtensions:
		return "WEAVE_" + strings.ReplaceAll(strings.ToUpper(name), "-", "_")
	default:
		return DefaultEnvPrefix
	}
}

// filterExtensionArgs strips extension-specific prefixes from CLI args.
// Flags like --bash-timeout are transformed to --timeout for the extension's loader.
func filterExtensionArgs(args []string, name string) []string {
	if len(args) == 0 {
		return nil
	}

	prefix := "--" + name + "-"

	var result []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Handle --ext-flag=value form.
		if eqIdx := strings.Index(arg, "="); eqIdx > 0 {
			if strings.HasPrefix(arg[:eqIdx], prefix) {
				result = append(result, "--"+arg[len(prefix):])
			}

			continue
		}

		// Handle --ext-flag value form.
		if strings.HasPrefix(arg, prefix) {
			result = append(result, "--"+arg[len(prefix):])
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result = append(result, args[i+1])
				i++
			}

			continue
		}
	}

	return result
}

func toMapAny(raw any) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}

	m, ok := raw.(map[string]any)
	if ok {
		return m, nil
	}

	// If it's not already a map, try JSON round-trip.
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	dec.UseNumber()

	var result map[string]any
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return result, nil
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

	data, err := toMapAny(layered)
	if err != nil {
		return fmt.Errorf("convert preferences: %w", err)
	}

	loader := Loader{
		Data: data,
	}

	return loader.Load(target)
}

func (c *FullConfig) SaveProviderKey(providerName, apiKey string) error {
	if err := auth.SetProviderKey(providerName, apiKey); err != nil {
		return fmt.Errorf("save provider key: %w", err)
	}

	return nil
}

// SaveExtensionConfig persists a scoped config subtree to the active settings
// layer using the same source selection as ExtensionConfig default population.
func (c *FullConfig) SaveExtensionConfig(scope, name string, target any) error {
	sourcePath, err := c.resolveSourcePath()
	if err != nil {
		return fmt.Errorf("resolve source settings: %w", err)
	}

	incoming, err := toMapAny(target)
	if err != nil {
		return fmt.Errorf("convert config for %s.%s: %w", scope, name, err)
	}

	saveSettingsMu.Lock()
	defer saveSettingsMu.Unlock()

	root, err := readSettingsMap(sourcePath)
	if err != nil {
		return fmt.Errorf("read source settings: %w", err)
	}

	if saveErr := saveConfigForScope(root, scope, name, incoming); saveErr != nil {
		return saveErr
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(sourcePath), 0o700); mkdirErr != nil {
		return fmt.Errorf("create settings dir: %w", mkdirErr)
	}

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if err := os.WriteFile(sourcePath, data, 0o600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	if sourcePath == c.filePath {
		settings, err := loadSettingsFile(sourcePath)
		if err != nil {
			return fmt.Errorf("reload settings: %w", err)
		}

		c.settings = settings
	}

	c.layeredOnce = sync.Once{}
	c.layered = nil
	c.layeredErr = nil

	return nil
}

func saveConfigForScope(root map[string]any, scope, name string, incoming map[string]any) error {
	switch scope {
	case configScopeUI, configScopeGuardian, configScopeSandbox, configScopeJSONL:
		existing, err := mapForKey(root, scope)
		if err != nil {
			return err
		}

		root[scope] = deepMergeValues(existing, incoming)

		return nil
	case configScopeTools, configScopeProviders, configScopeExtensions, configScopeUIExtensions:
		scopeMap, err := mapForKey(root, scope)
		if err != nil {
			return err
		}

		if existing, ok := scopeMap[name]; ok {
			scopeMap[name] = deepMergeValues(existing, incoming)
		} else {
			scopeMap[name] = incoming
		}

		root[scope] = scopeMap

		return nil
	default:
		return fmt.Errorf("unknown config scope %q", scope)
	}
}

func (c *FullConfig) SavePreferences(target any) error {
	saveSettingsMu.Lock()
	defer saveSettingsMu.Unlock()

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

	return saveSettings(&final, SettingsGlobal, "")
}
