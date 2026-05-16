package subagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionRegistration(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetToolRegistry()

	// Re-register the extension by calling init logic manually.
	// Since init() already ran at package load, we need to reset and re-register.
	sdk.RegisterExtension[struct{}]("subagent", func(cfg sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		projectDir := dirFromConfig(cfg)

		agents, err := DiscoverAgents(projectDir)
		if err != nil {
			return nil, err
		}

		for _, agent := range agents {
			a := agent
			toolName := "subagent_" + a.Name
			sdk.RegisterTool[struct{}](toolName, func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
				return newSubagentTool(a, nil, nil, "", ""), nil
			})
		}

		mgr := newBackgroundManager(nil, "", "")

		sdk.RegisterTool[struct{}]("check_agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
			return &checkAgentTool{mgr: mgr}, nil
		})
		sdk.RegisterTool[struct{}]("await_agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
			return &awaitAgentTool{mgr: mgr}, nil
		})

		return sdk.NewExtensionFunc("subagent", nil), nil
	})

	// Verify extension is registered.
	ext, err := sdk.GetExtension("subagent", nil)
	require.NoError(t, err)
	assert.Equal(t, "subagent", ext.Name())
}

func TestExtensionFactoryRegistersTools(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetToolRegistry()

	// Register and instantiate the extension.
	sdk.RegisterExtension[struct{}]("subagent", func(cfg sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		projectDir := dirFromConfig(cfg)

		agents, err := DiscoverAgents(projectDir)
		if err != nil {
			return nil, err
		}

		mgr := newBackgroundManager(nil, "", "")

		for _, agent := range agents {
			a := agent
			toolName := "subagent_" + a.Name
			sdk.RegisterTool[struct{}](toolName, func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
				return newSubagentTool(a, mgr, nil, "", ""), nil
			})
		}

		sdk.RegisterTool[struct{}]("check_agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
			return &checkAgentTool{mgr: mgr}, nil
		})
		sdk.RegisterTool[struct{}]("await_agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
			return &awaitAgentTool{mgr: mgr}, nil
		})

		return sdk.NewExtensionFunc("subagent", nil), nil
	})

	_, err := sdk.GetExtension("subagent", nil)
	require.NoError(t, err)

	// Built-in agents should have registered tools.
	assert.True(t, sdk.ToolRegistered("subagent_general"))
	assert.True(t, sdk.ToolRegistered("subagent_explore"))
	assert.True(t, sdk.ToolRegistered("subagent_plan"))
	assert.True(t, sdk.ToolRegistered("check_agent"))
	assert.True(t, sdk.ToolRegistered("await_agent"))

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
	tool := newSubagentTool(agent, nil, nil, "", "")

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
	tool := newSubagentTool(agent, nil, nil, "", "")

	ctx := context.Background()
	args := map[string]any{"prompt": "hello", "tasks": []any{"task1"}}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "mutually exclusive")
}

func TestExecute_MissingMode(t *testing.T) {
	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, nil, nil, "", "")

	ctx := context.Background()
	args := map[string]any{}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "exactly one of prompt, tasks, or chain is required")
}

func TestExecute_PromptMode(t *testing.T) {
	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "result: " + prompt, nil
	}

	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, nil, nil, "", "")

	ctx := context.Background()
	args := map[string]any{"prompt": "hello"}
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, "result: hello", result.Content)
}

func TestExecute_ParallelMode(t *testing.T) {
	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "result: " + prompt, nil
	}

	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, nil, nil, "", "")

	ctx := context.Background()
	args := map[string]any{
		"tasks": []any{
			map[string]any{"prompt": "task 1"},
			map[string]any{"prompt": "task 2"},
		},
	}

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "result: task 1")
	assert.Contains(t, result.Content, "result: task 2")
}

func TestExecute_ChainMode(t *testing.T) {
	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "result: " + prompt, nil
	}

	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, nil, nil, "", "")

	ctx := context.Background()
	args := map[string]any{
		"chain": []any{
			map[string]any{"prompt": "step 1"},
			map[string]any{"prompt": "step 2 with {previous}"},
		},
	}

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, "result: step 2 with result: step 1", result.Content)
}

func TestExecute_PromptMode_WithCWD(t *testing.T) {
	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	var receivedCWD string

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		receivedCWD = cwd
		return "done", nil
	}

	agent := &AgentDef{Name: "test"}
	tool := newSubagentTool(agent, nil, nil, "", "")

	ctx := context.Background()
	args := map[string]any{
		"prompt": "hello",
		"cwd":    "testdata",
	}

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, receivedCWD, "testdata")
}

func TestResolveCWD_Valid(t *testing.T) {
	parent, err := os.Getwd()
	require.NoError(t, err)

	resolved, err := resolveCWD("testdata")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(parent, "testdata"), resolved)
}

func TestResolveCWD_Escapes(t *testing.T) {
	_, err := resolveCWD("../..")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes working directory")
}

func TestResolveCWD_AbsoluteEscapes(t *testing.T) {
	_, err := resolveCWD("/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes working directory")
}

func TestResolveCWD_SymlinkInside(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	linkPath := filepath.Join(tmpDir, "link")
	require.NoError(t, os.Symlink(subDir, linkPath))

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tmpDir))

	resolved, err := resolveCWD("link")
	require.NoError(t, err)
	// resolveCWD returns the symlink-resolved path for safety.
	// EvalSymlinks on subDir to handle macOS /var -> /private/var.
	expectedSubDir, err := filepath.EvalSymlinks(subDir)
	require.NoError(t, err)
	assert.Equal(t, expectedSubDir, resolved)
}

func TestResolveCWD_SymlinkEscapes(t *testing.T) {
	tmpDir := t.TempDir()
	linkPath := filepath.Join(tmpDir, "link")
	require.NoError(t, os.Symlink("/", linkPath))

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tmpDir))

	_, err = resolveCWD("link")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes working directory")
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

func TestDirFromConfig_PlainPath(t *testing.T) {
	cfg := sdk.FilePathConfig("/project/settings.json")
	dir := dirFromConfig(cfg)
	assert.Equal(t, "/project", dir)
}

func TestDirFromConfig_WeaveSettingsJson(t *testing.T) {
	cfg := sdk.FilePathConfig("/project/.weave/settings.json")
	dir := dirFromConfig(cfg)
	assert.Equal(t, "/project", dir)
}
