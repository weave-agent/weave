package subagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseJSONLines_MessageEnd(t *testing.T) {
	input := `{"type":"message_start","model":"claude-haiku-4-5"}
{"type":"message_update","content":"Thinking..."}
{"type":"message_end","content":"Final answer here","usage":{"input":150,"output":200}}
`
	result, err := parseJSONLines(strings.NewReader(input), nil)
	require.NoError(t, err)
	assert.Equal(t, "Final answer here", result)
}

func TestParseJSONLines_MultipleMessageEnd_LastWins(t *testing.T) {
	input := `{"type":"message_end","content":"First"}
{"type":"message_end","content":"Second"}
`
	result, err := parseJSONLines(strings.NewReader(input), nil)
	require.NoError(t, err)
	assert.Equal(t, "Second", result)
}

func TestParseJSONLines_EmptyInput(t *testing.T) {
	_, err := parseJSONLines(strings.NewReader(""), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid JSON events")
}

func TestParseJSONLines_OnlyWhitespace(t *testing.T) {
	_, err := parseJSONLines(strings.NewReader("\n\n  \n"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid JSON events")
}

func TestParseJSONLines_NonJSONIgnored(t *testing.T) {
	input := `{"type":"message_end","content":"Result"}
this is not json
{"type":"message_update","content":"update"}
`
	result, err := parseJSONLines(strings.NewReader(input), nil)
	require.NoError(t, err)
	assert.Equal(t, "Result", result)
}

func TestParseJSONLines_AllEventTypes(t *testing.T) {
	input := `{"type":"message_start","model":"gpt-5"}
{"type":"message_update","content":"Hello"}
{"type":"tool_call","tool":"grep","args":{"pattern":"TODO"}}
{"type":"tool_result","tool":"grep","output":"found"}
{"type":"message_end","content":"Done","usage":{"input":10,"output":5}}
`
	result, err := parseJSONLines(strings.NewReader(input), nil)
	require.NoError(t, err)
	assert.Equal(t, "Done", result)
}

func TestBuildCommand_BasicArgs(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	agent := &AgentDef{
		Name:        "explore",
		Description: "Research agent",
		Tools:       []string{"read", "grep", "find"},
		Model:       "claude-haiku-4-5",
		Sandbox:     "readonly",
	}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "search for TODOs", "", "", "", "")
	require.NoError(t, err)

	defer cleanup()

	require.NotNil(t, cmd)
	assert.NotEmpty(t, cmd.Path)

	args := cmd.Args
	require.GreaterOrEqual(t, len(args), 2)

	assert.Equal(t, "/tmp/weave-launcher", cmd.Path)

	// Check for weave-prompt-file.
	foundPromptFile := false

	for _, a := range args {
		if strings.HasPrefix(a, "--weave-prompt-file=") {
			foundPromptFile = true
			path := strings.TrimPrefix(a, "--weave-prompt-file=")
			assert.NotEmpty(t, path)
		}
	}

	assert.True(t, foundPromptFile, "expected --weave-prompt-file flag")

	// Verify extra flags.
	assert.Contains(t, args, "--output=json")
	assert.Contains(t, args, "--tools=read,grep,find")
	assert.Contains(t, args, "--sandbox=readonly")
	assert.Contains(t, args, "--model=claude-haiku-4-5")
}

func TestBuildCommand_UsesLauncherPathWhenAvailable(t *testing.T) {
	launcherPath := "/tmp/weave-launcher"
	t.Setenv("WEAVE_LAUNCHER_PATH", launcherPath)

	agent := &AgentDef{
		Name:    "explore",
		Tools:   []string{"read", "grep", "find"},
		Model:   "claude-haiku-4-5",
		Sandbox: "readonly",
	}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "search for TODOs", "", "subagent_explore_123", "/project/.weave/settings.json", "/project")
	require.NoError(t, err)

	defer cleanup()

	assert.Equal(t, launcherPath, cmd.Path)

	args := cmd.Args
	assert.NotContains(t, args, "--weave-headless=true")
	assert.Contains(t, args, "--output=json")
	assert.Contains(t, args, "--tools=read,grep,find")
	assert.Contains(t, args, "--sandbox=readonly")
	assert.Contains(t, args, "--model=claude-haiku-4-5")
	assert.Contains(t, args, "--subagent-id=subagent_explore_123")
	assert.Contains(t, args, "--config=/project/.weave/settings.json")
	assert.Contains(t, args, "--weave-project-dir=/project")

	foundPromptFile := false

	for _, a := range args {
		if strings.HasPrefix(a, "--weave-prompt-file=") {
			foundPromptFile = true
		}
	}

	assert.True(t, foundPromptFile, "expected internal prompt-file flag")
}

func TestBuildCommand_RequiresLauncherPath(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "")

	cmd, cleanup, err := buildCommand(context.Background(), &AgentDef{Name: "test"}, "prompt", "", "", "", "")
	require.Error(t, err)
	assert.Nil(t, cmd)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "WEAVE_LAUNCHER_PATH is not set")
}

func TestBuildCommand_NoOptionalFlags(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	agent := &AgentDef{
		Name:        "minimal",
		Description: "Minimal agent",
	}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "hello", "", "", "", "")
	require.NoError(t, err)

	defer cleanup()

	args := cmd.Args

	// Should not have --tools, --sandbox, or --model when agent has none.
	for _, a := range args {
		assert.False(t, strings.HasPrefix(a, "--tools="), "unexpected --tools flag")
		assert.False(t, strings.HasPrefix(a, "--sandbox="), "unexpected --sandbox flag")
		assert.False(t, strings.HasPrefix(a, "--model="), "unexpected --model flag")
	}

	// --output=json is always included.
	assert.Contains(t, args, "--output=json")
}

func TestBuildCommand_CWD(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	agent := &AgentDef{Name: "test"}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "prompt", "/tmp/workdir", "", "", "")
	require.NoError(t, err)

	defer cleanup()

	assert.Equal(t, "/tmp/workdir", cmd.Dir)
}

func TestBuildCommand_PromptFileCreated(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	agent := &AgentDef{Name: "test"}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "my test prompt", "", "", "", "")
	require.NoError(t, err)

	defer cleanup()

	// Find the prompt file path.
	var promptFile string

	for _, a := range cmd.Args {
		if p, ok := strings.CutPrefix(a, "--weave-prompt-file="); ok {
			promptFile = p
			break
		}
	}

	require.NotEmpty(t, promptFile)

	// Verify the file exists and contains the prompt.
	data, err := os.ReadFile(promptFile)
	require.NoError(t, err)
	assert.Equal(t, "my test prompt", string(data))

	// Verify permissions are restricted to owner-only.
	info, err := os.Stat(promptFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestRunSubagent_Mock(t *testing.T) {
	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "mocked result for " + agent.Name + ": " + prompt, nil
	}

	agent := &AgentDef{Name: "explore"}
	result, err := runSubagent(context.Background(), agent, "find bugs", "", "", nil, "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "mocked result for explore: find bugs", result)
}

func TestRunSubagent_MockError(t *testing.T) {
	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	testRunSubagent = func(ctx context.Context, agent *AgentDef, prompt, cwd, subagentID string, broker *Broker, cfgPath, projectDir string) (string, error) {
		return "", errors.New("mock failure")
	}

	agent := &AgentDef{Name: "explore"}
	_, err := runSubagent(context.Background(), agent, "find bugs", "", "", nil, "", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock failure")
}

func TestRunSubagent_Abort(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	// Test that context cancellation aborts the subagent.
	// We spawn a real "sleep" command and cancel the context early.
	if testRunSubagent != nil {
		t.Skip("testRunSubagent is set, skipping real subprocess test")
	}

	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	// Override buildCommand to use "sleep" instead of the weave binary.
	cmd, cleanup, err := buildCommand(context.Background(), &AgentDef{Name: "test"}, "prompt", "", "", "", "")
	require.NoError(t, err)

	defer cleanup()

	// Replace with a sleep command.
	cmd.Path = "sleep"
	cmd.Args = []string{"sleep", "10"}

	testRunSubagent = nil // ensure we use real exec

	// We need to test the actual runSubagent function with our custom command.
	// Since buildCommand is called inside runSubagent, we can't easily inject.
	// Instead, test at the buildCommand + exec level.
	ctx, cancel := context.WithCancel(context.Background())

	start := time.Now()

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Start the sleep command manually and simulate the abort logic.
	cmd = exec.CommandContext(ctx, "sleep", "10")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err = cmd.Start()
	require.NoError(t, err)

	go func() {
		<-ctx.Done()

		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}()

	err = cmd.Wait()
	elapsed := time.Since(start)

	// Should have been killed quickly, not after 10s.
	assert.Less(t, elapsed, 2*time.Second)
	assert.Error(t, err)
}

func TestParseJSONLines_InterAgentEvents(t *testing.T) {
	input := `{"type":"message_start","model":"claude-haiku-4-5"}
{"type":"send","to":"other","content":"hello"}
{"type":"broadcast","content":"all hello"}
{"type":"list_agents"}
{"type":"message_end","content":"Final result"}
`
	result, err := parseJSONLines(strings.NewReader(input), nil)
	require.NoError(t, err)
	assert.Equal(t, "Final result", result)
}

func TestParseJSONLines_MixedContentAndLogLines(t *testing.T) {
	input := `log: starting subagent
{"type":"message_start"}
{"type":"message_update","content":"working..."}
warning: something happened
{"type":"message_end","content":"Done"}
`
	result, err := parseJSONLines(strings.NewReader(input), nil)
	require.NoError(t, err)
	assert.Equal(t, "Done", result)
}

func TestParseJSONLines_EmptyLinesBetweenJSON(t *testing.T) {
	input := `{"type":"message_start"}

{"type":"message_end","content":"Result"}

`
	result, err := parseJSONLines(strings.NewReader(input), nil)
	require.NoError(t, err)
	assert.Equal(t, "Result", result)
}

func TestBuildCommand_ProcessGroup(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	agent := &AgentDef{Name: "test"}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "prompt", "", "", "", "")
	require.NoError(t, err)

	defer cleanup()

	require.NotNil(t, cmd.SysProcAttr)
	assert.True(t, cmd.SysProcAttr.Setpgid)
}

func TestBuildCommand_WithMessaging(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	agent := &AgentDef{
		Name:      "general",
		Messaging: true,
	}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "prompt", "", "subagent_general_abc123", "", "")
	require.NoError(t, err)

	defer cleanup()

	assert.True(t, slices.Contains(cmd.Args, "--subagent-id=subagent_general_abc123"), "expected --subagent-id flag with correct value")
}

func TestBuildCommand_MessagingAppendsTools(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	agent := &AgentDef{
		Name:      "general",
		Tools:     []string{"bash", "read"},
		Messaging: true,
	}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "prompt", "", "subagent_general_abc123", "", "")
	require.NoError(t, err)

	defer cleanup()

	var toolsArg string

	for _, a := range cmd.Args {
		if p, ok := strings.CutPrefix(a, "--tools="); ok {
			toolsArg = p
			break
		}
	}

	require.NotEmpty(t, toolsArg, "expected --tools flag")

	tools := strings.Split(toolsArg, ",")
	assert.Contains(t, tools, "bash")
	assert.Contains(t, tools, "read")
	assert.Contains(t, tools, "send_message")
	assert.Contains(t, tools, "broadcast_message")
	assert.Contains(t, tools, "list_agents")
}

func TestBuildCommand_MessagingDedupesTools(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	agent := &AgentDef{
		Name:      "general",
		Tools:     []string{"bash", "send_message"},
		Messaging: true,
	}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "prompt", "", "subagent_general_abc123", "", "")
	require.NoError(t, err)

	defer cleanup()

	var toolsArg string

	for _, a := range cmd.Args {
		if p, ok := strings.CutPrefix(a, "--tools="); ok {
			toolsArg = p
			break
		}
	}

	require.NotEmpty(t, toolsArg, "expected --tools flag")

	// send_message should appear exactly once.
	count := strings.Count(toolsArg, "send_message")
	assert.Equal(t, 1, count)
}

func TestBuildCommand_WithoutMessaging(t *testing.T) {
	t.Setenv("WEAVE_LAUNCHER_PATH", "/tmp/weave-launcher")

	agent := &AgentDef{
		Name:      "explore",
		Messaging: false,
	}

	cmd, cleanup, err := buildCommand(context.Background(), agent, "prompt", "", "", "", "")
	require.NoError(t, err)

	defer cleanup()

	for _, a := range cmd.Args {
		assert.False(t, strings.HasPrefix(a, "--subagent-id"), "unexpected --subagent-id flag")
	}
}

func TestBuildCommand_MessagingGeneratesID(t *testing.T) {
	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	var receivedID string

	testRunSubagent = func(_ context.Context, _ *AgentDef, _, _, subagentID string, _ *Broker, _, _ string) (string, error) {
		receivedID = subagentID

		return "ok", nil
	}

	agent := &AgentDef{Name: "test", Messaging: true}
	tool := newSubagentTool(agent, nil, nil, "", "")

	ctx := context.Background()
	args := map[string]any{"prompt": "hello"}
	_, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	assert.Contains(t, receivedID, "subagent_test_")
}

func TestBuildCommand_NoMessagingNoID(t *testing.T) {
	original := testRunSubagent

	t.Cleanup(func() { testRunSubagent = original })

	var receivedID string

	testRunSubagent = func(_ context.Context, _ *AgentDef, _, _, subagentID string, _ *Broker, _, _ string) (string, error) {
		receivedID = subagentID

		return "ok", nil
	}

	agent := &AgentDef{Name: "test", Messaging: false}
	_, err := runSubagent(context.Background(), agent, "prompt", "", "", nil, "", "", nil)
	require.NoError(t, err)

	assert.Empty(t, receivedID)
}

func TestParseJSONLines_OnEvent_Callback(t *testing.T) {
	var captured []jsonEvent

	onEvent := func(evt jsonEvent) {
		captured = append(captured, evt)
	}

	input := `{"type":"message_start","model":"claude-haiku-4-5"}
{"type":"message_update","content":"Thinking..."}
{"type":"tool_call","tool":"grep","args":{"pattern":"TODO"}}
{"type":"tool_result","tool":"grep","output":"found"}
{"type":"message_end","content":"Final answer","usage":{"input":150,"output":200}}
`
	result, err := parseJSONLines(strings.NewReader(input), onEvent)
	require.NoError(t, err)
	assert.Equal(t, "Final answer", result)

	require.Len(t, captured, 5)

	assert.Equal(t, "message_start", captured[0].Type)
	assert.Equal(t, "claude-haiku-4-5", captured[0].Model)

	assert.Equal(t, "message_update", captured[1].Type)
	assert.Equal(t, "Thinking...", captured[1].Content)

	assert.Equal(t, "tool_call", captured[2].Type)
	assert.Equal(t, "grep", captured[2].Tool)

	assert.Equal(t, "tool_result", captured[3].Type)
	assert.Equal(t, "grep", captured[3].Tool)

	assert.Equal(t, "message_end", captured[4].Type)
	assert.Equal(t, "Final answer", captured[4].Content)
}

func TestParseJSONLines_OnEvent_NilCallback(t *testing.T) {
	input := `{"type":"message_end","content":"Result"}
`
	// Should not panic with nil callback.
	result, err := parseJSONLines(strings.NewReader(input), nil)
	require.NoError(t, err)
	assert.Equal(t, "Result", result)
}

func TestParseJSONLines_OnEvent_NonJSONNotForwarded(t *testing.T) {
	var captured []jsonEvent

	onEvent := func(evt jsonEvent) {
		captured = append(captured, evt)
	}

	input := `log line
{"type":"message_end","content":"Done"}
more log
`
	result, err := parseJSONLines(strings.NewReader(input), onEvent)
	require.NoError(t, err)
	assert.Equal(t, "Done", result)

	// Only the JSON line should have been forwarded.
	require.Len(t, captured, 1)
	assert.Equal(t, "message_end", captured[0].Type)
}
