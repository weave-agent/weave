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
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	ThinkingLevel string `json:"thinking_level,omitempty"`
}

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

// SaveSettings writes the settings file.
func SaveSettings(s *Settings) error {
	path, err := SettingsPath()
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
		return fmt.Errorf("write settings: %w", err)
	}

	return nil
}
