package sdk

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndRetrieveTool(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("bash", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{
			NameFunc: func() string { return "bash" },
			DefinitionFunc: func() ToolDef {
				return ToolDef{Name: "bash", Description: "run commands"}
			},
		}, nil
	})

	got, err := GetTool("bash", nil)
	require.NoError(t, err, "GetTool")
	assert.Equal(t, "bash", got.Name())
}

func TestDuplicateToolRegistration(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("dup", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "first" }}, nil
	})

	// Second registration should be a no-op with a warning (no panic).
	RegisterTool[struct{}]("dup", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "second" }}, nil
	})

	// First registration wins.
	got, err := GetTool("dup", nil)
	require.NoError(t, err)
	assert.Equal(t, "first", got.Name())
}

func TestMissingTool(t *testing.T) {
	ResetToolRegistry()

	_, err := GetTool("nonexistent", nil)
	require.Error(t, err, "expected error for missing tool")
	assert.ErrorIs(t, err, ErrNotRegistered)
}

func TestGetTool_FactoryError(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("fail", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return nil, errors.New("factory error")
	})

	_, err := GetTool("fail", nil)
	require.Error(t, err, "expected error from failing factory")
	assert.Equal(t, "factory error", err.Error())
}

func TestListTools(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("bash", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "bash" }}, nil
	})
	RegisterTool[struct{}]("file", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "file" }}, nil
	})

	names := ListTools()
	sort.Strings(names)

	assert.Equal(t, []string{"bash", "file"}, names)
}

func TestResetToolRegistry(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("temp", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "temp" }}, nil
	})

	ResetToolRegistry()

	assert.Empty(t, ListTools())
}

func TestSetToolFilter(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("bash", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "bash" }}, nil
	})
	RegisterTool[struct{}]("read", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "read" }}, nil
	})
	RegisterTool[struct{}]("edit", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "edit" }}, nil
	})

	SetToolFilter([]string{"bash", "read"})
	defer func() { SetToolFilter(nil) }()

	names := ListTools()
	sort.Strings(names)
	assert.Equal(t, []string{"bash", "read"}, names)
}

func TestSetToolFilter_EmptyBlocksAll(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("bash", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "bash" }}, nil
	})
	RegisterTool[struct{}]("read", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "read" }}, nil
	})

	SetToolFilter([]string{})
	defer func() { SetToolFilter(nil) }()

	names := ListTools()
	assert.Empty(t, names)

	_, err := GetTool("bash", nil)
	require.Error(t, err)
}

func TestSetToolFilter_UnknownToolIgnored(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("bash", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "bash" }}, nil
	})

	SetToolFilter([]string{"bash", "nonexistent"})
	defer func() { SetToolFilter(nil) }()

	names := ListTools()
	assert.Equal(t, []string{"bash"}, names)
}

func TestSetToolFilter_NilClearsFilter(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("bash", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "bash" }}, nil
	})
	RegisterTool[struct{}]("read", func(Config, PreferenceStore, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "read" }}, nil
	})

	SetToolFilter([]string{"bash"})

	names := ListTools()
	assert.Equal(t, []string{"bash"}, names)

	SetToolFilter(nil)

	names = ListTools()
	assert.Equal(t, []string{"bash", "read"}, names)
}

// Test config population via ExtensionConfig in generic registration.

type testToolConfig struct {
	Timeout int    `json:"timeout" default:"120"`
	Shell   string `json:"shell" default:"bash"`
}

func TestRegisterTool_ConfigPopulation(t *testing.T) {
	ResetToolRegistry()
	ResetSchemas()

	var receivedConfig testToolConfig

	RegisterTool("bash", func(cfg Config, _ PreferenceStore, bc testToolConfig) (Tool, error) {
		receivedConfig = bc
		return &ToolMock{NameFunc: func() string { return "bash" }}, nil
	})

	mock := &ConfigMock{
		ExtensionConfigFunc: func(scope, name string, target any) error {
			// Simulate populating the config
			if tc, ok := target.(*testToolConfig); ok {
				tc.Timeout = 60
				tc.Shell = "zsh"
			}

			return nil
		},
	}

	got, err := GetTool("bash", mock)
	require.NoError(t, err)
	assert.Equal(t, "bash", got.Name())
	assert.Equal(t, 60, receivedConfig.Timeout)
	assert.Equal(t, "zsh", receivedConfig.Shell)
}

func TestRegisterTool_ConfigPopulationError(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("bash", func(cfg Config, _ PreferenceStore, bc testToolConfig) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "bash" }}, nil
	})

	mock := &ConfigMock{
		ExtensionConfigFunc: func(scope, name string, target any) error {
			return errors.New("config error")
		},
	}

	_, err := GetTool("bash", mock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config error")
}

func TestRegisterTool_SchemaExtraction(t *testing.T) {
	ResetToolRegistry()
	ResetSchemas()

	RegisterTool("bash", func(cfg Config, _ PreferenceStore, bc testToolConfig) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "bash" }}, nil
	})

	schema, ok := GetSchema("tools", "bash")
	require.True(t, ok)
	require.Len(t, schema.Fields, 2)

	fieldMap := make(map[string]SchemaField)
	for _, f := range schema.Fields {
		fieldMap[f.JSONName] = f
	}

	assert.Equal(t, "int", fieldMap["timeout"].Type)
	assert.Equal(t, "120", fieldMap["timeout"].Default)
	assert.Equal(t, "string", fieldMap["shell"].Type)
	assert.Equal(t, "bash", fieldMap["shell"].Default)
}

func TestGetToolFilter(t *testing.T) {
	ResetToolRegistry()

	// No filter set
	assert.Nil(t, GetToolFilter())

	// Set a filter
	SetToolFilter([]string{"read", "bash"})
	defer SetToolFilter(nil)

	filter := GetToolFilter()
	assert.Equal(t, []string{"bash", "read"}, filter)

	// Clear filter
	SetToolFilter(nil)
	assert.Nil(t, GetToolFilter())
}
