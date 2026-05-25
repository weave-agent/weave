package sdk

import (
	"reflect"
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

	type config struct {
		Timeout int `json:"timeout"`
	}

	storeSchema("tools", "bash", schema, reflect.TypeFor[config]())

	got, ok := GetSchema("tools", "bash")
	require.True(t, ok)
	assert.Equal(t, schema, got)
}

func TestGetSchemaInfo(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	type config struct {
		Model string `json:"model"`
	}

	schema := Schema{
		Fields: []SchemaField{
			{Name: "Model", JSONName: "model", Type: "string", Default: "gpt-5"},
		},
	}
	typ := reflect.TypeFor[config]()

	storeSchema("providers", "openai", schema, typ)

	info := GetSchemaInfo("providers", "openai")
	require.NotNil(t, info)
	assert.Equal(t, schema, info.Schema)
	assert.Equal(t, typ, info.Type)
}

func TestGetSchemaInfo_NotFound(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	assert.Nil(t, GetSchemaInfo("tools", "missing"))
}

func TestRegisterExtensionSchemaStoresSchemaInfo(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	type config struct {
		Enabled bool `json:"enabled" default:"true"`
	}

	RegisterExtensionSchema("extensions", "test", &config{})

	info := GetSchemaInfo("extensions", "test")
	require.NotNil(t, info)
	assert.Equal(t, reflect.TypeFor[config](), info.Type)
	require.Len(t, info.Schema.Fields, 1)
	assert.Equal(t, "enabled", info.Schema.Fields[0].JSONName)
	assert.Equal(t, "true", info.Schema.Fields[0].Default)
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

	storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout"}}}, reflect.TypeOf(struct{}{}))
	storeSchema("providers", "kimi", Schema{Fields: []SchemaField{{Name: "Model", JSONName: "model"}}}, reflect.TypeOf(struct{}{}))

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

	storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout"}}}, reflect.TypeOf(struct{}{}))
	ResetSchemas()

	_, ok := GetSchema("tools", "bash")
	assert.False(t, ok)
	assert.Nil(t, GetSchemaInfo("tools", "bash"))
}

func TestStoreSchema_FirstWins(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	type firstConfig struct {
		Timeout int `json:"timeout"`
	}

	type secondConfig struct {
		Timeout int `json:"timeout"`
	}

	storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout", Default: "60"}}}, reflect.TypeFor[firstConfig]())
	storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout", Default: "120"}}}, reflect.TypeFor[secondConfig]())

	got, ok := GetSchema("tools", "bash")
	require.True(t, ok)
	assert.Equal(t, "60", got.Fields[0].Default)

	info := GetSchemaInfo("tools", "bash")
	require.NotNil(t, info)
	assert.Equal(t, reflect.TypeFor[firstConfig](), info.Type)
}

func TestStoreSchema_SameNameDifferentScope(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	storeSchema("tools", "test", Schema{Fields: []SchemaField{{Name: "A", JSONName: "a"}}}, reflect.TypeOf(struct{}{}))
	storeSchema("extensions", "test", Schema{Fields: []SchemaField{{Name: "B", JSONName: "b"}}}, reflect.TypeOf(struct{}{}))

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
			storeSchema("tools", "bash", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout", Default: "120"}}}, reflect.TypeOf(struct{}{}))
			GetSchema("tools", "bash")
			GetSchemaInfo("tools", "bash")
			ListSchemas()
		})
	}

	wg.Wait()

	// After concurrent writes, schema should still be retrievable.
	_, ok := GetSchema("tools", "bash")
	assert.True(t, ok)
}

func TestResetSchemasForScopeClearsSchemaInfo(t *testing.T) {
	ResetSchemas()
	defer ResetSchemas()

	type toolConfig struct {
		Timeout int `json:"timeout"`
	}

	type extensionConfig struct {
		Name string `json:"name"`
	}

	storeSchema("tools", "test", Schema{Fields: []SchemaField{{Name: "Timeout", JSONName: "timeout"}}}, reflect.TypeFor[toolConfig]())
	storeSchema("extensions", "test", Schema{Fields: []SchemaField{{Name: "Name", JSONName: "name"}}}, reflect.TypeFor[extensionConfig]())

	ResetSchemasForScope("tools")

	_, ok := GetSchema("tools", "test")
	assert.False(t, ok)
	assert.Nil(t, GetSchemaInfo("tools", "test"))

	info := GetSchemaInfo("extensions", "test")
	require.NotNil(t, info)
	assert.Equal(t, reflect.TypeFor[extensionConfig](), info.Type)
}
