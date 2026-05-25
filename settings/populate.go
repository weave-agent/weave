package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"

	"github.com/weave-agent/weave/sdk"
)

func populateExtensionDefaults(sourcePath, scope, name string, effective map[string]any) error {
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

	defaults = pruneExistingDefaults(defaults, effective)
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

	return nil
}

func pruneExistingDefaults(defaults, existing map[string]any) map[string]any {
	pruned := make(map[string]any, len(defaults))

	for key, defaultValue := range defaults {
		existingValue, ok := existing[key]
		if !ok {
			pruned[key] = defaultValue

			continue
		}

		defaultMap, defaultOK := defaultValue.(map[string]any)

		existingMap, existingOK := existingValue.(map[string]any)
		if defaultOK && existingOK {
			nested := pruneExistingDefaults(defaultMap, existingMap)
			if len(nested) > 0 {
				pruned[key] = nested
			}
		}
	}

	return pruned
}

func buildDefaultsMap(schemaInfo *sdk.SchemaInfo) (map[string]any, error) {
	if schemaInfo == nil {
		return nil, errors.New("schema info is nil")
	}

	typ := schemaInfo.Type
	if typ == nil {
		return nil, errors.New("schema type is nil")
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

	defaults, err := defaultFieldsMap(reflect.ValueOf(instance).Elem())
	if err != nil {
		return nil, err
	}

	return defaults, nil
}

func defaultFieldsMap(v reflect.Value) (map[string]any, error) {
	defaults := map[string]any{}
	t := v.Type()

	for i := range v.NumField() {
		field := v.Field(i)
		if !field.CanInterface() {
			continue
		}

		ft := t.Field(i)

		jsonTag := ft.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name := sdk.JSONFieldName(jsonTag, ft.Name)
		if name == "" || name == "-" {
			continue
		}

		fieldValue := field
		for fieldValue.Kind() == reflect.Pointer {
			if fieldValue.IsNil() {
				break
			}

			fieldValue = fieldValue.Elem()
		}

		if fieldValue.Kind() == reflect.Struct && !implementsJSONMarshaler(fieldValue.Type()) {
			nested, err := defaultFieldsMap(fieldValue)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", ft.Name, err)
			}

			if len(nested) > 0 {
				defaults[name] = nested
			}

			continue
		}

		if ft.Tag.Get("default") == "" {
			continue
		}

		value, err := jsonValue(field.Interface())
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", ft.Name, err)
		}

		defaults[name] = value
	}

	return defaults, nil
}

func implementsJSONMarshaler(t reflect.Type) bool {
	marshaler := reflect.TypeFor[json.Marshaler]()

	return t.Implements(marshaler) || reflect.PointerTo(t).Implements(marshaler)
}

func jsonValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal default: %w", err)
	}

	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal default: %w", err)
	}

	return out, nil
}

func mergeMissing(defaults, existing map[string]any) map[string]any {
	result := make(map[string]any, len(existing)+len(defaults))
	maps.Copy(result, existing)

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

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}

	if root == nil {
		root = map[string]any{}
	}

	return root, nil
}

func mergeDefaultsForScope(root map[string]any, scope, name string, defaults map[string]any) (bool, error) {
	switch scope {
	case configScopeUI, configScopeGuardian, configScopeSandbox, configScopeJSONL:
		existing, err := mapForKey(root, scope)
		if err != nil {
			return false, err
		}

		merged := mergeMissing(defaults, existing)
		if reflect.DeepEqual(existing, merged) {
			return false, nil
		}

		root[scope] = merged

		return true, nil
	case configScopeTools, configScopeProviders, configScopeExtensions, configScopeUIExtensions:
		scopeMap, err := mapForKey(root, scope)
		if err != nil {
			return false, err
		}

		existing, err := mapForKey(scopeMap, name)
		if err != nil {
			return false, err
		}

		merged := mergeMissing(defaults, existing)
		if reflect.DeepEqual(existing, merged) {
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
