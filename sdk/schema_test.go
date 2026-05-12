package sdk

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type schemaTestConfig struct {
	Timeout  int     `json:"timeout" default:"120" env:"TIMEOUT" flag:"timeout" short:"t" validate:"gt=0" description:"Command timeout"`
	Shell    string  `json:"shell" default:"bash" env:"SHELL" flag:"shell" description:"Shell to use"`
	Verbose  bool    `json:"verbose" default:"false" env:"VERBOSE" flag:"verbose" short:"v" description:"Enable verbose output"`
	Ratio    float64 `json:"ratio" default:"0.5" validate:"gt=0,lt=1"`
	Internal string  // no tags
}

type nestedSchemaConfig struct {
	Name  string            `json:"name" default:"default-name"`
	Inner nestedInnerConfig `json:"inner"`
}

type nestedInnerConfig struct {
	Key   string `json:"key" default:"inner-default"`
	Count int    `json:"count" default:"5" validate:"gt=0"`
}

func TestExtractSchema(t *testing.T) {
	schema := extractSchema(reflect.TypeFor[schemaTestConfig]())

	requireLen := func(t *testing.T, expected int) {
		t.Helper()
		assert.Len(t, schema.Fields, expected)
	}
	requireLen(t, 5)

	fieldMap := make(map[string]SchemaField)
	for _, f := range schema.Fields {
		fieldMap[f.JSONName] = f
	}

	assert.Equal(t, "Timeout", fieldMap["timeout"].Name)
	assert.Equal(t, "int", fieldMap["timeout"].Type)
	assert.Equal(t, "120", fieldMap["timeout"].Default)
	assert.Equal(t, "TIMEOUT", fieldMap["timeout"].Env)
	assert.Equal(t, "timeout", fieldMap["timeout"].Flag)
	assert.Equal(t, "t", fieldMap["timeout"].Short)
	assert.Equal(t, "gt=0", fieldMap["timeout"].Validate)
	assert.Equal(t, "Command timeout", fieldMap["timeout"].Description)

	assert.Equal(t, "Shell", fieldMap["shell"].Name)
	assert.Equal(t, "string", fieldMap["shell"].Type)
	assert.Equal(t, "bash", fieldMap["shell"].Default)

	assert.Equal(t, "Verbose", fieldMap["verbose"].Name)
	assert.Equal(t, "bool", fieldMap["verbose"].Type)
	assert.Equal(t, "false", fieldMap["verbose"].Default)

	assert.Equal(t, "Ratio", fieldMap["ratio"].Name)
	assert.Equal(t, "float64", fieldMap["ratio"].Type)
	assert.Equal(t, "0.5", fieldMap["ratio"].Default)

	assert.Equal(t, "Internal", fieldMap["Internal"].Name)
	assert.Empty(t, fieldMap["Internal"].Default)
}

func TestExtractSchema_Nested(t *testing.T) {
	schema := extractSchema(reflect.TypeFor[nestedSchemaConfig]())

	assert.Len(t, schema.Fields, 3)

	fieldMap := make(map[string]SchemaField)
	for _, f := range schema.Fields {
		fieldMap[f.JSONName] = f
	}

	assert.Equal(t, "Name", fieldMap["name"].Name)
	assert.Equal(t, "default-name", fieldMap["name"].Default)

	assert.Equal(t, "Inner.Key", fieldMap["inner.key"].Name)
	assert.Equal(t, "string", fieldMap["inner.key"].Type)
	assert.Equal(t, "inner-default", fieldMap["inner.key"].Default)

	assert.Equal(t, "Inner.Count", fieldMap["inner.count"].Name)
	assert.Equal(t, "int", fieldMap["inner.count"].Type)
	assert.Equal(t, "5", fieldMap["inner.count"].Default)
	assert.Equal(t, "gt=0", fieldMap["inner.count"].Validate)
}

func TestExtractSchema_EmptyStruct(t *testing.T) {
	schema := extractSchema(reflect.TypeFor[struct{}]())
	assert.Empty(t, schema.Fields)
}

func TestExtractSchema_NonStruct(t *testing.T) {
	schema := extractSchema(reflect.TypeFor[string]())
	assert.Empty(t, schema.Fields)
}

func TestExtractSchema_Nil(t *testing.T) {
	schema := extractSchema(nil)
	assert.Empty(t, schema.Fields)
}

func TestExtractSchema_SkipsJSONIgnore(t *testing.T) {
	type skipConfig struct {
		Visible string `json:"visible"`
		Hidden  string `json:"-"`
	}

	schema := extractSchema(reflect.TypeFor[skipConfig]())
	assert.Len(t, schema.Fields, 1)
	assert.Equal(t, "visible", schema.Fields[0].JSONName)
}

func TestJsonFieldName(t *testing.T) {
	assert.Equal(t, "name", jsonFieldName("name", "Fallback"))
	assert.Equal(t, "name", jsonFieldName("name,omitempty", "Fallback"))
	assert.Equal(t, "Fallback", jsonFieldName("", "Fallback"))
	assert.Equal(t, "-", jsonFieldName("-", "Fallback"))
}
