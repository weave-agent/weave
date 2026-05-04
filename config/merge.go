package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
)

// MergeSettings deep-merges multiple Settings layers. Later layers override
// earlier ones. Nested objects (UI fields, tool config maps) merge recursively;
// primitive values and slices are replaced entirely by the later layer.
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

			for k, v := range layer.Tools {
				if existing, ok := result.Tools[k]; ok {
					result.Tools[k] = deepMergeValues(existing, v)
				} else {
					result.Tools[k] = v
				}
			}
		}
	}

	return result
}

// LoadLayeredSettings loads settings from global, project, and local files,
// then merges them in order (global → project → local).
// Missing files are silently skipped. Local settings are loaded from the same
// directory where project settings were found, or from projectDir directly if
// no project settings file exists.
func LoadLayeredSettings(projectDir string) (*Settings, error) {
	globalPath, err := SettingsPath()
	if err != nil {
		return nil, fmt.Errorf("global settings path: %w", err)
	}

	global, err := loadSettingsFile(globalPath)
	if err != nil {
		return nil, fmt.Errorf("load global settings: %w", err)
	}

	project, localDir, err := loadProjectSettings(projectDir)
	if err != nil {
		return nil, fmt.Errorf("load project settings: %w", err)
	}

	if localDir == "" {
		localDir = projectDir
	}

	local, err := loadLocalSettings(localDir)
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

		return nil, fmt.Errorf("read settings file %s: %w", path, err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return &s, nil
}

// loadProjectSettings walks up from projectDir looking for .weave/settings.json.
// Returns the settings and the directory where it was found (empty string if not found).
// Stops before reaching the user's home directory to avoid treating global settings
// (~/.weave/settings.json) as project-layer settings.
func loadProjectSettings(projectDir string) (*Settings, string, error) {
	home, _ := os.UserHomeDir()

	dir := projectDir

	for home == "" || dir != home {
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

// loadLocalSettings loads .weave/settings.local.json from the given directory.
// Missing files are silently skipped.
func loadLocalSettings(dir string) (*Settings, error) {
	if dir == "" {
		return &Settings{}, nil
	}

	path := filepath.Join(dir, ".weave", "settings.local.json")

	return loadSettingsFile(path)
}

// deepMergeValues recursively merges two values. When both are map[string]any,
// keys from incoming override existing keys, and nested maps are merged
// recursively. For all other types, incoming replaces existing entirely.
func deepMergeValues(existing, incoming any) any {
	existingMap, ok1 := existing.(map[string]any)

	incomingMap, ok2 := incoming.(map[string]any)
	if !ok1 || !ok2 {
		return incoming
	}

	result := make(map[string]any, len(existingMap))
	maps.Copy(result, existingMap)

	for k, v := range incomingMap {
		if prev, ok := result[k]; ok {
			result[k] = deepMergeValues(prev, v)
		} else {
			result[k] = v
		}
	}

	return result
}
