package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Settings holds user preferences that persist across sessions.
type Settings struct {
	Provider      string         `json:"provider,omitempty"`
	Model         string         `json:"model,omitempty"`
	ThinkingLevel string         `json:"thinking_level,omitempty"`
	UI            *UISettings    `json:"ui,omitempty"`
	Tools         map[string]any `json:"tools,omitempty"`
}

// UISettings holds UI-specific preferences.
type UISettings struct {
	Theme          string `json:"theme,omitempty"`
	EditorMaxLines int    `json:"editor_max_lines,omitempty"`
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
			// For .weave.yaml: configDir = project root, local settings at .weave/settings.local.json
			// For .weave/config.yaml: configDir = .weave/ dir, local settings at settings.local.json
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
