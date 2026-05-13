package sdk

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreSchemaAndGetSchema(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	schema := Schema{
		Fields: []SchemaField{
			{Name: "Timeout", JSONName: "timeout", Type: "int", Default: "120"},
		},
	}

	storeSchema("tools", "bash", schema)

	got, ok := GetSchema("tools", "bash")
	require.True(t, ok)
	assert.Equal(t, schema, got)
}

func TestGetSchema_NotFound(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	_, ok := GetSchema("tools", "nonexistent")
	assert.False(t, ok)
}

func TestListSchemas(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout"}}})
	storeSchema("providers", "kimi", Schema{Fields: []SchemaField{{Name: "Model", JSONName: "model"}}})

	all := ListSchemas()
	require.Len(t, all, 2)

	byName := make(map[string]SchemaEntry)
	for _, e := range all {
		byName[e.Name] = e
	}

	assert.Equal(t, "tools", byName["bash"].Scope)
	assert.Equal(t, "providers", byName["kimi"].Scope)
}

func TestResetSchemas(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout"}}})
	ResetSchemas()

	_, ok := GetSchema("tools", "bash")
	assert.False(t, ok)
}

func TestStoreSchema_FirstWins(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout", Default: "60"}}})
	storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout", Default: "120"}}})

	got, ok := GetSchema("tools", "bash")
	require.True(t, ok)
	assert.Equal(t, "60", got.Fields[0].Default)
}

func TestStoreSchema_SameNameDifferentScope(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	storeSchema("tools", "test", Schema{Fields: []SchemaField{{Name: "A", JSONName: "a"}}})
	storeSchema("extensions", "test", Schema{Fields: []SchemaField{{Name: "B", JSONName: "b"}}})

	// Both schemas are independently retrievable by scope+name.
	toolsSchema, ok := GetSchema("tools", "test")
	require.True(t, ok)
	assert.Equal(t, "A", toolsSchema.Fields[0].Name)

	extSchema, ok := GetSchema("extensions", "test")
	require.True(t, ok)
	assert.Equal(t, "B", extSchema.Fields[0].Name)
}

func TestSchemaRegistry_ConcurrentAccess(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout", Default: "120"}}})
			GetSchema("tools", "bash")
			ListSchemas()
		})
	}

	wg.Wait()

	// After concurrent writes, schema should still be retrievable.
	_, ok := GetSchema("tools", "bash")
	assert.True(t, ok)
}
