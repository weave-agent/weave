package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Settings holds user preferences that persist across sessions.
type Settings struct {
	Provider      string                 `json:"provider,omitempty"`
	Model         string                 `json:"model,omitempty"`
	ThinkingLevel string                 `json:"thinking_level,omitempty"`
	UI            *UISettings            `json:"ui,omitempty"`
	Tools         map[string]interface{} `json:"tools,omitempty"`
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
			return "", fmt.Errorf("project settings require projectDir")
		}

		return filepath.Join(projectDir, ".weave", "settings.json"), nil
	case SettingsLocal:
		if projectDir == "" {
			return "", fmt.Errorf("local settings require projectDir")
		}

		return filepath.Join(projectDir, ".weave", "settings.local.json"), nil
	default:
		return "", fmt.Errorf("unknown settings layer: %q", layer)
	}
}
