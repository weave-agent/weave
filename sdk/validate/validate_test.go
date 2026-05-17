package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArgs_RequiredFields(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "number"},
		},
		"required": []any{"name"},
	}

	err := Args(map[string]any{"name": "alice"}, schema)
	require.NoError(t, err)

	err = Args(map[string]any{"age": 30.0}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `missing required field: "name"`)
}

func TestArgs_UnknownFields(t *testing.T) {
	schema := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"name": map[string]any{"type": "string"}},
		"additionalProperties": false,
	}

	err := Args(map[string]any{"name": "alice"}, schema)
	require.NoError(t, err)

	err = Args(map[string]any{"name": "alice", "extra": "value"}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field: "extra"`)
}

func TestArgs_UnknownFieldsAllowed(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
		// additionalProperties omitted — defaults to true (allow unknowns)
	}

	err := Args(map[string]any{"name": "alice", "extra": "value"}, schema)
	require.NoError(t, err)
}

func TestArgs_TypeValidation(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"str":  map[string]any{"type": "string"},
			"num":  map[string]any{"type": "number"},
			"int":  map[string]any{"type": "integer"},
			"bool": map[string]any{"type": "boolean"},
			"arr":  map[string]any{"type": "array"},
			"obj":  map[string]any{"type": "object"},
		},
	}

	err := Args(map[string]any{
		"str":  "hello",
		"num":  3.14,
		"int":  42.0,
		"bool": true,
		"arr":  []any{1, 2, 3},
		"obj":  map[string]any{"key": "val"},
	}, schema)
	require.NoError(t, err)
}

func TestArgs_TypeMismatch(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	err := Args(map[string]any{"name": 3.14}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `field "name"`)
	assert.Contains(t, err.Error(), `expected type "string", got "number"`)
}

func TestArgs_ArrayItems(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	}

	err := Args(map[string]any{"tags": []any{"a", "b"}}, schema)
	require.NoError(t, err)

	err = Args(map[string]any{"tags": []any{"a", 42}}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `item 1`)
	assert.Contains(t, err.Error(), `expected type "string", got "integer"`)
}

func TestArgs_NilSchema(t *testing.T) {
	err := Args(map[string]any{"key": "val"}, nil)
	require.NoError(t, err)
}

func TestArgs_EmptyArgs(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
	}

	err := Args(map[string]any{}, schema)
	require.NoError(t, err)
}
