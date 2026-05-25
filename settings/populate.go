package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/weave-agent/weave/sdk"
)

func populateExtensionDefaults(sourcePath, scope, name string) error {
	info := sdk.GetSchemaInfo(scope, name)
	if info == nil {
		return nil
	}

	defaults, err := buildDefaultsMap(info)
	if err != nil {
		return err
	}

	if len(defaults) == 0 {
		return nil
	}

	saveSettingsMu.Lock()
	defer saveSettingsMu.Unlock()

	root, err := readSettingsMap(sourcePath)
	if err != nil {
		return err
	}

	changed, err := mergeDefaultsForScope(root, scope, name, defaults)
	if err != nil {
		return err
	}

	if !changed {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if err := os.WriteFile(sourcePath, data, 0o600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	return nil
}

func buildDefaultsMap(schemaInfo *sdk.SchemaInfo) (map[string]any, error) {
	if schemaInfo == nil {
		return nil, fmt.Errorf("schema info is nil")
	}

	typ := schemaInfo.Type
	if typ == nil {
		return nil, fmt.Errorf("schema type is nil")
	}

	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("schema type must be a struct, got %s", typ.Kind())
	}

	instance := reflect.New(typ).Interface()
	if err := applyDefaults(instance); err != nil {
		return nil, fmt.Errorf("apply defaults: %w", err)
	}

	data, err := json.Marshal(instance)
	if err != nil {
		return nil, fmt.Errorf("marshal defaults: %w", err)
	}

	var defaults map[string]any
	if err := json.Unmarshal(data, &defaults); err != nil {
		return nil, fmt.Errorf("unmarshal defaults: %w", err)
	}

	return defaults, nil
}

func mergeMissing(defaults, existing map[string]any) map[string]any {
	result := make(map[string]any, len(existing)+len(defaults))
	for k, v := range existing {
		result[k] = v
	}

	for k, defaultValue := range defaults {
		existingValue, ok := result[k]
		if !ok {
			result[k] = defaultValue

			continue
		}

		defaultMap, defaultOK := defaultValue.(map[string]any)
		existingMap, existingOK := existingValue.(map[string]any)
		if defaultOK && existingOK {
			result[k] = mergeMissing(defaultMap, existingMap)
		}
	}

	return result
}

func mapsEqual(a, b map[string]any) bool {
	return reflect.DeepEqual(a, b)
}

func readSettingsMap(sourcePath string) (map[string]any, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}

		return nil, fmt.Errorf("read settings: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}

	if root == nil {
		root = map[string]any{}
	}

	return root, nil
}

func mergeDefaultsForScope(root map[string]any, scope, name string, defaults map[string]any) (bool, error) {
	switch scope {
	case "ui", "guardian", "sandbox", "jsonl":
		existing, err := mapForKey(root, scope)
		if err != nil {
			return false, err
		}

		merged := mergeMissing(defaults, existing)
		if mapsEqual(existing, merged) {
			return false, nil
		}

		root[scope] = merged

		return true, nil
	case "tools", "providers", "extensions", "ui_extensions":
		scopeMap, err := mapForKey(root, scope)
		if err != nil {
			return false, err
		}

		existing, err := mapForKey(scopeMap, name)
		if err != nil {
			return false, err
		}

		merged := mergeMissing(defaults, existing)
		if mapsEqual(existing, merged) {
			return false, nil
		}

		scopeMap[name] = merged
		root[scope] = scopeMap

		return true, nil
	default:
		return false, fmt.Errorf("unknown config scope %q", scope)
	}
}

func mapForKey(parent map[string]any, key string) (map[string]any, error) {
	value, ok := parent[key]
	if !ok || value == nil {
		return map[string]any{}, nil
	}

	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config key %q must be an object, got %T", key, value)
	}

	return m, nil
}
