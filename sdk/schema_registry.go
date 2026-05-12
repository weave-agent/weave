package sdk

import (
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

// ListSchemas returns all registered names with their scopes and schemas.
func ListSchemas() map[string]struct {
	Scope  string
	Schema Schema
} {
	schemaMu.RLock()
	defer schemaMu.RUnlock()

	result := make(map[string]struct {
		Scope  string
		Schema Schema
	}, len(scopeMap))
	for name, scope := range scopeMap {
		key := scopeKey(scope, name)
		result[name] = struct {
			Scope  string
			Schema Schema
		}{
			Scope:  scope,
			Schema: schemas[key],
		}
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
