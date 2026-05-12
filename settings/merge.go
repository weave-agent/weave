package settings

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

		mergeStringFields(result, layer)

		if layer.RespectGitignore != nil {
			result.RespectGitignore = layer.RespectGitignore
		}

		mergeUI(result, layer)

		if len(layer.ExcludeExtensions) > 0 {
			result.ExcludeExtensions = layer.ExcludeExtensions
		}

		mergeProviders(result, layer)
		mergeTools(result, layer)
	}

	return result
}

func mergeStringFields(result, layer *Settings) {
	if layer.AgentLoop != "" {
		result.AgentLoop = layer.AgentLoop
	}

	if layer.UIExtension != "" {
		result.UIExtension = layer.UIExtension
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
}

func mergeUI(result, layer *Settings) {
	if len(layer.UI) == 0 {
		return
	}

	if result.UI == nil {
		result.UI = make(map[string]any, len(layer.UI))
	}

	for k, v := range layer.UI {
		if existing, ok := result.UI[k]; ok {
			result.UI[k] = deepMergeValues(existing, v)
		} else {
			result.UI[k] = v
		}
	}
}

func mergeProviders(result, layer *Settings) {
	if len(layer.Providers) == 0 {
		return
	}

	if result.Providers == nil {
		result.Providers = make(map[string]any, len(layer.Providers))
	}

	for k, v := range layer.Providers {
		if existing, ok := result.Providers[k]; ok {
			result.Providers[k] = deepMergeValues(existing, v)
		} else {
			result.Providers[k] = v
		}
	}
}

func mergeTools(result, layer *Settings) {
	if layer.Tools == nil {
		return
	}

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

// LoadLayeredSettings loads settings from global and local files,
// then merges them in order (global → local).
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

	local, err := loadLocalSettings(projectDir)
	if err != nil {
		return nil, fmt.Errorf("load local settings: %w", err)
	}

	return MergeSettings(global, local), nil
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

// loadLocalSettings walks up from dir looking for .weave/settings.local.json.
// Missing files are silently skipped.
func loadLocalSettings(startDir string) (*Settings, error) {
	if startDir == "" {
		return &Settings{}, nil
	}

	home, _ := os.UserHomeDir()

	dir := startDir

	for home == "" || dir != home {
		path := filepath.Join(dir, ".weave", "settings.local.json")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return loadSettingsFile(path)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return &Settings{}, nil
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
