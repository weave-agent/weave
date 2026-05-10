package subagent

import (
	"context"
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionRegistration(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetToolRegistry()

	// Re-register the extension by calling init logic manually.
	// Since init() already ran at package load, we need to reset and re-register.
	sdk.RegisterExtension("subagent", func(cfg sdk.Config) (sdk.Extension, error) {
		projectDir := dirFromConfig(cfg)

		agents, err := DiscoverAgents(projectDir)
		if err != nil {
			return nil, err
		}

		for _, agent := range agents {
			a := agent
			toolName := "subagent_" + a.Name
			sdk.RegisterTool(toolName, func(sdk.Config) (sdk.Tool, error) {
				return newSubagentTool(a), nil
			})
		}

		return sdk.NewExtensionFunc("subagent", nil), nil
	})

	// Verify extension is registered.
	ext, err := sdk.GetExtension("subagent", nil)
	require.NoError(t, err)
	assert.Equal(t, "subagent", ext.Name())
}

func TestExtensionFactoryRegistersTools(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetToolRegistry()

	// Register and instantiate the extension.
	sdk.RegisterExtension("subagent", func(cfg sdk.Config) (sdk.Extension, error) {
		projectDir := dirFromConfig(cfg)

		agents, err := DiscoverAgents(projectDir)
		if err != nil {
			return nil, err
		}

		for _, agent := range agents {
			a := agent
			toolName := "subagent_" + a.Name
			sdk.RegisterTool(toolName, func(sdk.Config) (sdk.Tool, error) {
				return newSubagentTool(a), nil
			})
		}

		return sdk.NewExtensionFunc("subagent", nil), nil
	})

	_, err := sdk.GetExtension("subagent", nil)
	require.NoError(t, err)

	// Built-in agents should have registered tools.
	assert.True(t, sdk.ToolRegistered("subagent_general"))
	assert.True(t, sdk.ToolRegistered("subagent_explore"))
	assert.True(t, sdk.ToolRegistered("subagent_plan"))

	// Verify tool names and definitions.
	tool, err := sdk.GetTool("subagent_explore", nil)
	require.NoError(t, err)
	assert.Equal(t, "subagent_explore", tool.Name())

	def := tool.Definition()
	assert.Equal(t, "subagent_explore", def.Name)
	assert.NotEmpty(t, def.Description)
	assert.NotNil(t, def.Parameters)
}

func TestSubagentTool_Definition(t *testing.T) {
	agent := &AgentDef{
		Name:        "test",
		Description: "A test agent",
	}
	tool := newSubagentTool(agent)

	def := tool.Definition()
	assert.Equal(t, "subagent_test", def.Name)
	assert.Equal(t, "A test agent", def.Description)

	params, ok := def.Parameters.(map[string]any)
	require.True(t, ok)

	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)

	expected := []string{"prompt", "tasks", "chain", "background", "cwd"}
	for _, name := range expected {
		assert.Contains(t, props, name, "expected parameter %q", name)
	}
}

func TestValidateMode_MissingAll(t *testing.T) {
	args := map[string]any{}
	m, err := validateMode(args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of prompt, tasks, or chain is required")
	assert.Empty(t, m)
}

func TestValidateMode_PromptOnly(t *testing.T) {
	args := map[string]any{"prompt": "do something"}
	m, err := validateMode(args)
	require.NoError(t, err)
	assert.Equal(t, modePrompt, m)
}

func TestValidateMode_TasksOnly(t *testing.T) {
	args := map[string]any{"tasks": []any{"task1", "task2"}}
	m, err := validateMode(args)
	require.NoError(t, err)
	assert.Equal(t, modeParallel, m)
}

func TestValidateMode_ChainOnly(t *testing.T) {
	args := map[string]any{"chain": []any{"step1", "step2"}}
	m, err := validateMode(args)
	require.NoError(t, err)
	assert.Equal(t, modeChain, m)
}

func TestValidateMode_MutuallyExclusive(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
	}{
		{
			name: "prompt and tasks",
			args: map[string]any{"prompt": "do something", "tasks": []any{"task1"}},
		},
		{
			name: "prompt and chain",
			args: map[string]any{"prompt": "do something", "chain": []any{"step1"}},
		},
		{
			name: "tasks and chain",
			args: map[string]any{"tasks": []any{"task1"}, "chain": []any{"step1"}},
		},
		{
			name: "all three",
			args: map[string]any{"prompt": "do something", "tasks": []any{"task1"}, "chain": []any{"step1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := validateMode(tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "mutually exclusive")
			assert.Empty(t, m)
		})
	}
}

func TestValidateMode_EmptyPrompt(t *testing.T) {
	args := map[string]any{"prompt": ""}
	m, err := validateMode(args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of prompt, tasks, or chain is required")
	assert.Empty(t, m)
}

func TestValidateMode_EmptyTasks(t *testing.T) {
	args := map[string]any{"tasks": []any{}}
	m, err := validateMode(args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of prompt, tasks, or chain is required")
	assert.Empty(t, m)
}

func TestValidateMode_EmptyChain(t *testing.T) {
	args := map[string]any{"chain": []any{}}
	m, err := validateMode(args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of prompt, tasks, or chain is required")
	assert.Empty(t, m)
}

func TestValidateMode_TypedArrays(t *testing.T) {
	// JSON decoding may produce []map[string]any for object arrays.
	args := map[string]any{"tasks": []map[string]any{{"key": "value"}}}
	m, err := validateMode(args)
	require.NoError(t, err)
	assert.Equal(t, modeParallel, m)

	args = map[string]any{"chain": []map[string]any{{"key": "value"}}}
	m, err = validateMode(args)
	require.NoError(t, err)
	assert.Equal(t, modeChain, m)
}

func TestExecute_ValidationError(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent)

	ctx := context.Background()
	args := map[string]any{"prompt": "hello", "tasks": []any{"task1"}}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "mutually exclusive")
}

func TestExecute_MissingMode(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent)

	ctx := context.Background()
	args := map[string]any{}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "exactly one of prompt, tasks, or chain is required")
}

func TestExecute_PromptMode(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent)

	ctx := context.Background()
	args := map[string]any{"prompt": "hello"}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "subagent test:")
	assert.Contains(t, result.Content, "prompt")
}

func TestDirFromConfig_Nil(t *testing.T) {
	dir := dirFromConfig(nil)
	assert.NotEmpty(t, dir)
}

func TestDirFromConfig_EmptyFilePath(t *testing.T) {
	cfg := sdk.FilePathConfig("")
	dir := dirFromConfig(cfg)
	assert.NotEmpty(t, dir)
}

func TestDirFromConfig_WeaveYaml(t *testing.T) {
	cfg := sdk.FilePathConfig("/project/.weave.yaml")
	dir := dirFromConfig(cfg)
	assert.Equal(t, "/project", dir)
}

func TestDirFromConfig_WeaveConfigYaml(t *testing.T) {
	cfg := sdk.FilePathConfig("/project/.weave/config.yaml")
	dir := dirFromConfig(cfg)
	assert.Equal(t, "/project", dir)
}
