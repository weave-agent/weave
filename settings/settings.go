package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// JSONLConfig holds jsonl store configuration from the config file.
type JSONLConfig struct {
	Dir string `json:"dir" description:"Session directory (default: ~/.weave/sessions)"`
}

// Settings holds all configuration — project-level settings and user preferences
// unified into a single struct.
type Settings struct {
	// Project-level config fields.
	AgentLoop         string            `json:"agent_loop,omitempty" default:"agent" env:"AGENT_LOOP" description:"Agent loop extension name"`
	UIExtension       string            `json:"ui_extension,omitempty" default:"tui" env:"UI_EXTENSION" description:"UI extension name"`
	Providers         map[string]any    `json:"providers,omitempty" description:"Per-provider configuration"`
	ExcludeExtensions []string          `json:"exclude_extensions,omitempty" env:"EXCLUDE_EXTENSIONS" description:"Extensions to exclude from auto-discovery"`
	Sandbox           SandboxFileConfig `json:"sandbox" description:"Sandbox configuration"`
	Extensions        map[string]any    `json:"extensions,omitempty" description:"Per-extension configuration"`
	UIExtensions      map[string]any    `json:"ui_extensions,omitempty" description:"Per-UI-extension configuration"`

	// User preference fields.
	Provider         string         `json:"provider,omitempty" env:"PROVIDER"`
	Model            string         `json:"model,omitempty" env:"MODEL"`
	ThinkingLevel    string         `json:"thinking_level,omitempty" env:"THINKING_LEVEL"`
	RespectGitignore *bool          `json:"respect_gitignore,omitempty" env:"RESPECT_GITIGNORE"`
	UI               map[string]any `json:"ui,omitempty"`
	Tools            map[string]any `json:"tools,omitempty"`
	JSONL            JSONLConfig    `json:"jsonl"`

	// CLI-only flags (not persisted).
	Prompt      string `short:"p" json:"-" description:"Prompt to pass to the agent"`
	Output      string `flag:"output" json:"-" description:"Output format: text or json"`
	ToolsFlag   string `flag:"tools" json:"-" description:"Comma-separated tool allowlist"`
	ToolsSet    bool   `json:"-" description:"True when --tools= was explicitly passed"`
	SubagentID  string `flag:"subagent-id" json:"-" description:"Subagent ID for inter-agent communication"`
	SandboxMode string `flag:"sandbox" json:"-" description:"Sandbox mode override"`
	ModelFlag   string `flag:"model" json:"-" description:"Model override for this session"`
	Debug       bool   `flag:"debug" json:"-" description:"Enable debug logging"`
	Continue    bool   `flag:"continue" short:"c" json:"-" description:"Resume most recent session"`
	Resume      string `flag:"resume" short:"r" json:"-" description:"Resume specific session by ID"`
}

// SettingsLayer identifies which settings file to read or write.
type SettingsLayer string

const (
	// SettingsGlobal is ~/.weave/settings.json.
	SettingsGlobal SettingsLayer = "global"
	// SettingsProject is .weave/settings.json relative to the project.
	SettingsProject SettingsLayer = "project"
	// SettingsLocal is .weave/settings.local.json (gitignored, per-developer overrides).
	SettingsLocal SettingsLayer = "local"
)

var (
	settingsMu   sync.RWMutex
	settingsPath string // override for tests
)

// SetSettingsPath overrides the settings file path. For testing only.
func SetSettingsPath(p string) {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	settingsPath = p
}

// SettingsPath returns the path to the settings file (~/.weave/settings.json).
func SettingsPath() (string, error) {
	settingsMu.RLock()

	if settingsPath != "" {
		p := settingsPath

		settingsMu.RUnlock()

		return p, nil
	}

	settingsMu.RUnlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("settings path: %w", err)
	}

	return filepath.Join(home, ".weave", "settings.json"), nil
}

// LoadSettings reads and parses the settings file.
// Returns an empty Settings if not found.
func LoadSettings() (*Settings, error) {
	path, err := SettingsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Settings{}, nil
		}

		return nil, fmt.Errorf("read settings: %w", err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}

	return &s, nil
}

// SaveSettings writes the settings file. When layer is SettingsProject, the
// projectDir is used to locate the .weave directory. For SettingsGlobal (the
// zero value), it writes to the global path.
func SaveSettings(s *Settings, layer SettingsLayer, projectDir string) error {
	path, err := settingsPathForLayer(layer, projectDir)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)

	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return fmt.Errorf("create settings dir: %w", mkdirErr)
	}

	data, marshalErr := json.MarshalIndent(s, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshal settings: %w", marshalErr)
	}

	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return fmt.Errorf("write settings: %w", writeErr)
	}

	return nil
}

// SaveSettingsGlobal persists settings to the global file.
// Convenience wrapper for SaveSettings with SettingsGlobal layer.
func SaveSettingsGlobal(s *Settings) error {
	return SaveSettings(s, SettingsGlobal, "")
}

// settingsPathForLayer resolves the file path for a given layer.
func settingsPathForLayer(layer SettingsLayer, projectDir string) (string, error) {
	switch layer {
	case SettingsGlobal, "":
		return SettingsPath()
	case SettingsProject:
		if projectDir == "" {
			return "", errors.New("project settings require projectDir")
		}

		return filepath.Join(projectDir, ".weave", "settings.json"), nil
	case SettingsLocal:
		if projectDir == "" {
			return "", errors.New("local settings require projectDir")
		}

		return filepath.Join(projectDir, ".weave", "settings.local.json"), nil
	default:
		return "", fmt.Errorf("unknown settings layer: %q", layer)
	}
}

// WeaveFlags returns CLI flags formatted for the generated binary.
// It translates parsed user-facing flags into --weave-* prefixed flags.
func (s *Settings) WeaveFlags() []string {
	var flags []string

	if s.Debug {
		flags = append(flags, "--weave-debug=true")
	}

	if s.Output != "" {
		flags = append(flags, "--weave-output="+s.Output)
	}

	if s.ToolsSet || s.ToolsFlag != "" {
		flags = append(flags, "--weave-tools="+s.ToolsFlag)
	}

	if s.SubagentID != "" {
		flags = append(flags, "--weave-subagent-id="+s.SubagentID)
	}

	if s.SandboxMode != "" {
		flags = append(flags, "--weave-sandbox-mode="+s.SandboxMode)
	}

	if s.ModelFlag != "" {
		flags = append(flags, "--weave-model="+s.ModelFlag)
	}

	return flags
}

// EnsureLocalSettingsExcluded adds the local settings file to the project's
// .git/info/exclude so it is never accidentally committed. Walks up from
// configDir to find the nearest .git directory and computes the correct
// relative path. Silently skips if not a git repo.
func EnsureLocalSettingsExcluded(configDir string) {
	dir := configDir

	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			gitRoot := dir

			rel, err := filepath.Rel(gitRoot, configDir)
			if err != nil {
				return
			}

			// configDir is the directory containing the config file.
			// For .weave/settings.json at project root: configDir = project root, local settings at .weave/settings.local.json
			// For .weave/settings.json inside .weave/: configDir = .weave/ dir, local settings at settings.local.json
			var entry string
			if filepath.Base(configDir) == ".weave" {
				entry = filepath.Join(rel, "settings.local.json")
			} else {
				entry = filepath.Join(rel, ".weave", "settings.local.json")
			}

			ensureExcludeEntry(filepath.Join(gitDir, "info", "exclude"), entry)

			return
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}

		dir = parent
	}
}

// ensureExcludeEntry appends the exclusion line to the git exclude file if
// it is not already present.
func ensureExcludeEntry(excludePath, entry string) {
	data, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return
	}

	content := string(data)

	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == entry {
			return
		}
	}

	_ = os.MkdirAll(filepath.Dir(excludePath), 0o750)

	f, err := os.OpenFile(excludePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	if content != "" && !strings.HasSuffix(content, "\n") {
		_, _ = f.WriteString("\n")
	}

	_, _ = fmt.Fprintf(f, "%s\n", entry)
}
