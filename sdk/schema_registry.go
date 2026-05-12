package sdk

import (
	"strings"
	"sync"
)

var (
	schemaMu sync.RWMutex
	schemas  = make(map[string]Schema)
	scopeMap = make(map[string]string) // name -> scope (tools, providers, extensions)
)

// storeSchema stores the schema for a named extension under the given scope.
func storeSchema(scope, name string, schema Schema) {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	key := scopeKey(scope, name)
	schemas[key] = schema
	scopeMap[name] = scope
}

// GetSchema returns the schema for a named extension and its scope.
// When multiple scopes share a name, the last-registered scope wins.
// The second return value reports whether it was found.
func GetSchema(name string) (Schema, string, bool) {
	schemaMu.RLock()
	defer schemaMu.RUnlock()

	scope, ok := scopeMap[name]
	if !ok {
		return Schema{}, "", false
	}

	key := scopeKey(scope, name)
	schema, ok := schemas[key]

	return schema, scope, ok
}

// SchemaEntry pairs a schema with its registered name and scope.
type SchemaEntry struct {
	Name   string
	Scope  string
	Schema Schema
}

// ListSchemas returns all registered schemas. Entries are ordered by
// registration order within each scope, but overall order is not guaranteed.
func ListSchemas() []SchemaEntry {
	schemaMu.RLock()
	defer schemaMu.RUnlock()

	result := make([]SchemaEntry, 0, len(schemas))
	for key, schema := range schemas {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			continue
		}

		result = append(result, SchemaEntry{
			Name:   parts[1],
			Scope:  parts[0],
			Schema: schema,
		})
	}

	return result
}

// ResetSchemas clears all stored schemas.
func ResetSchemas() {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	clear(schemas)
	clear(scopeMap)
}

func scopeKey(scope, name string) string {
	return scope + "/" + name
}
