package settings

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/weave-agent/weave/sdk"
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

		if layer.ExcludeExtensions != nil {
			result.ExcludeExtensions = layer.ExcludeExtensions
		}

		mergeProviders(result, layer)
		mergeTools(result, layer)
		mergeGuardian(result, layer)
		mergeSandbox(result, layer)
		mergeJSONL(result, layer)
		mergeExtensions(result, layer)
		mergeUIExtensions(result, layer)
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
	if layer.UI == nil {
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
	if layer.Providers == nil {
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

func mergeGuardian(result, layer *Settings) {
	if layer.Guardian.Profile != "" {
		result.Guardian.Profile = layer.Guardian.Profile
	}

	if layer.Guardian.AskFallback != nil {
		result.Guardian.AskFallback = layer.Guardian.AskFallback
	}

	if layer.Guardian.Profiles != nil {
		if result.Guardian.Profiles == nil {
			result.Guardian.Profiles = make(map[string]sdk.GuardianProfile, len(layer.Guardian.Profiles))
		}

		maps.Copy(result.Guardian.Profiles, layer.Guardian.Profiles)
	}
}

func mergeSandbox(result, layer *Settings) {
	if layer.Sandbox.Enabled != nil {
		result.Sandbox.Enabled = layer.Sandbox.Enabled
	}

	if layer.Sandbox.FailIfUnavailable != nil {
		result.Sandbox.FailIfUnavailable = layer.Sandbox.FailIfUnavailable
	}

	if layer.Sandbox.AllowUnsandboxedFallback != nil {
		result.Sandbox.AllowUnsandboxedFallback = layer.Sandbox.AllowUnsandboxedFallback
	}

	if layer.Sandbox.Filesystem.ReadOnly != nil {
		result.Sandbox.Filesystem.ReadOnly = layer.Sandbox.Filesystem.ReadOnly
	}

	if layer.Sandbox.Filesystem.ReadWrite != nil {
		result.Sandbox.Filesystem.ReadWrite = layer.Sandbox.Filesystem.ReadWrite
	}

	if layer.Sandbox.Filesystem.Blocked != nil {
		result.Sandbox.Filesystem.Blocked = layer.Sandbox.Filesystem.Blocked
	}

	if layer.Sandbox.Network.Enabled != nil {
		result.Sandbox.Network.Enabled = layer.Sandbox.Network.Enabled
	}

	if layer.Sandbox.Network.AllowHosts != nil {
		result.Sandbox.Network.AllowHosts = layer.Sandbox.Network.AllowHosts
	}

	if layer.Sandbox.Network.AllowPorts != nil {
		result.Sandbox.Network.AllowPorts = layer.Sandbox.Network.AllowPorts
	}

	if layer.Sandbox.Network.BlockHosts != nil {
		result.Sandbox.Network.BlockHosts = layer.Sandbox.Network.BlockHosts
	}

	if layer.Sandbox.Network.AllowListen != nil {
		result.Sandbox.Network.AllowListen = layer.Sandbox.Network.AllowListen
	}
}

func mergeJSONL(result, layer *Settings) {
	if layer.JSONL.Dir != "" {
		result.JSONL.Dir = layer.JSONL.Dir
	}
}

func mergeExtensions(result, layer *Settings) {
	if layer.Extensions == nil {
		return
	}

	if result.Extensions == nil {
		result.Extensions = make(map[string]any, len(layer.Extensions))
	}

	for k, v := range layer.Extensions {
		if existing, ok := result.Extensions[k]; ok {
			result.Extensions[k] = deepMergeValues(existing, v)
		} else {
			result.Extensions[k] = v
		}
	}
}

func mergeUIExtensions(result, layer *Settings) {
	if layer.UIExtensions == nil {
		return
	}

	if result.UIExtensions == nil {
		result.UIExtensions = make(map[string]any, len(layer.UIExtensions))
	}

	for k, v := range layer.UIExtensions {
		if existing, ok := result.UIExtensions[k]; ok {
			result.UIExtensions[k] = deepMergeValues(existing, v)
		} else {
			result.UIExtensions[k] = v
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

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
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

// loadLocalSettings walks up from dir looking for .weave/settings.local.json.
// Missing files are silently skipped.
func loadLocalSettings(startDir string) (*Settings, error) {
	path, found, err := findLocalSettingsPath(startDir)
	if err != nil {
		return nil, err
	}

	if !found {
		return &Settings{}, nil
	}

	return loadSettingsFile(path)
}

func findLocalSettingsPath(startDir string) (string, bool, error) {
	if startDir == "" {
		return "", false, nil
	}

	home, _ := os.UserHomeDir()

	dir := startDir

	for home == "" || dir != home {
		path := filepath.Join(dir, ".weave", "settings.local.json")
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, true, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", false, fmt.Errorf("stat local settings %s: %w", path, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return "", false, nil
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
