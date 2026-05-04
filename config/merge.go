package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
)

// MergeSettings deep-merges multiple Settings layers. Later layers override
// earlier ones. Nested objects merge recursively; primitive values and slices
// are replaced entirely by the later layer.
func MergeSettings(layers ...*Settings) *Settings {
	result := &Settings{}

	for _, layer := range layers {
		if layer == nil {
			continue
		}

		if layer.Provider != "" {
			result.Provider = layer.Provider
		}

		if layer.Model != "" {
			result.Model = layer.Model
		}

		if layer.ThinkingLevel != "" {
			result.ThinkingLevel = layer.ThinkingLevel
		}

		if layer.UI != nil {
			if result.UI == nil {
				result.UI = &UISettings{}
			}

			if layer.UI.Theme != "" {
				result.UI.Theme = layer.UI.Theme
			}

			if layer.UI.EditorMaxLines != 0 {
				result.UI.EditorMaxLines = layer.UI.EditorMaxLines
			}
		}

		if layer.Tools != nil {
			if result.Tools == nil {
				result.Tools = make(map[string]any, len(layer.Tools))
			}

			maps.Copy(result.Tools, layer.Tools)
		}
	}

	return result
}

// LoadLayeredSettings loads settings from global, project, and local files,
// then merges them in order (global → project → local).
// Missing files are silently skipped.
func LoadLayeredSettings(projectDir string) (*Settings, error) {
	globalPath, err := SettingsPath()
	if err != nil {
		return nil, fmt.Errorf("global settings path: %w", err)
	}

	global, err := loadSettingsFile(globalPath)
	if err != nil {
		return nil, fmt.Errorf("load global settings: %w", err)
	}

	project, projectPath, err := loadProjectSettings(projectDir)
	if err != nil {
		return nil, fmt.Errorf("load project settings: %w", err)
	}

	local, err := loadLocalSettings(projectPath)
	if err != nil {
		return nil, fmt.Errorf("load local settings: %w", err)
	}

	return MergeSettings(global, project, local), nil
}

// loadSettingsFile reads and parses a single settings file.
// Returns an empty Settings if the file does not exist.
func loadSettingsFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Settings{}, nil
		}

		return nil, err
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return &s, nil
}

// loadProjectSettings walks up from projectDir looking for .weave/settings.json.
// Returns the settings and the directory where it was found (empty string if not found).
func loadProjectSettings(projectDir string) (*Settings, string, error) {
	dir := projectDir

	for {
		candidate := filepath.Join(dir, ".weave", "settings.json")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			s, err := loadSettingsFile(candidate)
			if err != nil {
				return nil, "", err
			}

			return s, dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return &Settings{}, "", nil
}

// loadLocalSettings loads .weave/settings.local.json from the same directory
// where project settings were found. projectDir is empty when no project
// settings were found; in that case, local settings are also skipped.
func loadLocalSettings(projectDir string) (*Settings, error) {
	if projectDir == "" {
		return &Settings{}, nil
	}

	path := filepath.Join(projectDir, ".weave", "settings.local.json")
	return loadSettingsFile(path)
}
