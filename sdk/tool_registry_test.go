package sdk

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndRetrieveTool(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("bash", func(Config) (Tool, error) {
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

	RegisterTool("dup", func(Config) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "first" }}, nil
	})

	// Second registration should be a no-op with a warning (no panic).
	RegisterTool("dup", func(Config) (Tool, error) {
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
}

func TestGetTool_FactoryError(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("fail", func(Config) (Tool, error) {
		return nil, errors.New("factory error")
	})

	_, err := GetTool("fail", nil)
	require.Error(t, err, "expected error from failing factory")
	assert.Equal(t, "factory error", err.Error())
}

func TestListTools(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("bash", func(Config) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "bash" }}, nil
	})
	RegisterTool("file", func(Config) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "file" }}, nil
	})

	names := ListTools()
	sort.Strings(names)

	assert.Equal(t, []string{"bash", "file"}, names)
}

func TestResetToolRegistry(t *testing.T) {
	ResetToolRegistry()

	RegisterTool("temp", func(Config) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "temp" }}, nil
	})

	ResetToolRegistry()

	assert.Empty(t, ListTools())
}

// Suppress unused import warning.
var _ = context.Background
