package sdk

import (
	"reflect"
	"sort"
	"strings"
	"sync"
)

var (
	schemaMu sync.RWMutex
	schemas  = make(map[string]SchemaInfo)
)

// SchemaInfo contains a registered config schema and the config struct type
// used to build it.
type SchemaInfo struct {
	Schema Schema
	Type   reflect.Type
}

// storeSchema stores the schema for a named extension under the given scope.
func storeSchema(scope, name string, schema Schema, typ reflect.Type) {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	key := scopeKey(scope, name)
	if _, exists := schemas[key]; exists {
		return
	}

	schemas[key] = SchemaInfo{
		Schema: schema,
		Type:   typ,
	}
}

// RegisterExtensionSchema extracts the schema from the provided config value and
// stores it under the given scope and name. It is a no-op if a schema for that
// key is already registered.
func RegisterExtensionSchema(scope, name string, config any) {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	key := scopeKey(scope, name)
	if _, exists := schemas[key]; exists {
		return
	}

	configType := reflect.TypeOf(config)
	if configType == nil {
		return
	}

	typ := configType
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return
	}

	schemas[key] = SchemaInfo{
		Schema: extractSchema(typ),
		Type:   typ,
	}
}

// GetSchema returns the schema for a named extension within the given scope.
// The second return value reports whether it was found.
func GetSchema(scope, name string) (Schema, bool) {
	schemaMu.RLock()
	defer schemaMu.RUnlock()

	key := scopeKey(scope, name)

	info, ok := schemas[key]
	if !ok {
		return Schema{}, false
	}

	return info.Schema, true
}

// GetSchemaInfo returns the schema metadata for a named extension within the
// given scope, or nil when the schema is not registered.
func GetSchemaInfo(scope, name string) *SchemaInfo {
	schemaMu.RLock()
	defer schemaMu.RUnlock()

	key := scopeKey(scope, name)

	info, ok := schemas[key]
	if !ok {
		return nil
	}

	return &info
}

// SchemaEntry pairs a schema with its registered name and scope.
type SchemaEntry struct {
	Name   string
	Scope  string
	Schema Schema
}

// ListSchemas returns all registered schemas sorted by scope then name.
func ListSchemas() []SchemaEntry {
	schemaMu.RLock()
	defer schemaMu.RUnlock()

	result := make([]SchemaEntry, 0, len(schemas))
	for key, info := range schemas {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			continue
		}

		result = append(result, SchemaEntry{
			Name:   parts[1],
			Scope:  parts[0],
			Schema: info.Schema,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Scope != result[j].Scope {
			return result[i].Scope < result[j].Scope
		}

		return result[i].Name < result[j].Name
	})

	return result
}

// ResetSchemas clears all stored schemas.
func ResetSchemas() {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	clear(schemas)
}

// ResetSchemasForScope clears all schemas for the given scope.
func ResetSchemasForScope(scope string) {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	prefix := scope + "/"
	for key := range schemas {
		if strings.HasPrefix(key, prefix) {
			delete(schemas, key)
		}
	}
}

func scopeKey(scope, name string) string {
	return scope + "/" + name
}
