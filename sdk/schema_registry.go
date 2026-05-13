package sdk

import (
	"sort"
	"strings"
	"sync"
)

var (
	schemaMu sync.RWMutex
	schemas  = make(map[string]Schema)
)

// storeSchema stores the schema for a named extension under the given scope.
func storeSchema(scope, name string, schema Schema) {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	key := scopeKey(scope, name)
	if _, exists := schemas[key]; exists {
		return
	}

	schemas[key] = schema
}

// GetSchema returns the schema for a named extension within the given scope.
// The second return value reports whether it was found.
func GetSchema(scope, name string) (Schema, bool) {
	schemaMu.RLock()
	defer schemaMu.RUnlock()

	key := scopeKey(scope, name)
	schema, ok := schemas[key]

	return schema, ok
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
