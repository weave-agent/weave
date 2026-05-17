package subagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/weave-agent/weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendMessageTool_NameAndDefinition(t *testing.T) {
	tool := &sendMessageTool{}
	assert.Equal(t, "send_message", tool.Name())

	def := tool.Definition()
	assert.Equal(t, "send_message", def.Name)
	assert.NotEmpty(t, def.Description)

	params, ok := def.Parameters.(map[string]any)
	require.True(t, ok)
	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "to")
	assert.Contains(t, props, "content")
}

func TestSendMessageTool_Execute(t *testing.T) {
	var stdoutBuf strings.Builder

	oldStdout := stdoutWriter

	stdoutWriter = &stdoutBuf
	defer func() { stdoutWriter = oldStdout }()

	tool := &sendMessageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"to":      "agent2",
		"content": "hello from agent1",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "agent2")

	// Verify JSON written to stdout.
	line := strings.TrimSpace(stdoutBuf.String())
	require.NotEmpty(t, line)

	var msg brokerMessage
	require.NoError(t, json.Unmarshal([]byte(line), &msg))
	assert.Equal(t, "send", msg.Type)
	assert.Equal(t, "agent2", msg.To)
	assert.Equal(t, "hello from agent1", msg.Content)
}

func TestSendMessageTool_MissingTo(t *testing.T) {
	tool := &sendMessageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"content": "hello",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "missing required parameter: to")
}

func TestSendMessageTool_EmptyTo(t *testing.T) {
	tool := &sendMessageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"to":      "",
		"content": "hello",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "non-empty string")
}

func TestSendMessageTool_MissingContent(t *testing.T) {
	tool := &sendMessageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"to": "agent2",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "missing required parameter: content")
}

func TestBroadcastMessageTool_NameAndDefinition(t *testing.T) {
	tool := &broadcastMessageTool{}
	assert.Equal(t, "broadcast_message", tool.Name())

	def := tool.Definition()
	assert.Equal(t, "broadcast_message", def.Name)
	assert.NotEmpty(t, def.Description)

	params, ok := def.Parameters.(map[string]any)
	require.True(t, ok)
	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "content")
}

func TestBroadcastMessageTool_Execute(t *testing.T) {
	var stdoutBuf strings.Builder

	oldStdout := stdoutWriter

	stdoutWriter = &stdoutBuf
	defer func() { stdoutWriter = oldStdout }()

	tool := &broadcastMessageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"content": "hello all",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "broadcast")

	// Verify JSON written to stdout.
	line := strings.TrimSpace(stdoutBuf.String())
	require.NotEmpty(t, line)

	var msg brokerMessage
	require.NoError(t, json.Unmarshal([]byte(line), &msg))
	assert.Equal(t, "broadcast", msg.Type)
	assert.Equal(t, "hello all", msg.Content)
}

func TestBroadcastMessageTool_MissingContent(t *testing.T) {
	tool := &broadcastMessageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "missing required parameter: content")
}

func TestListAgentsTool_NameAndDefinition(t *testing.T) {
	tool := &listAgentsTool{}
	assert.Equal(t, "list_agents", tool.Name())

	def := tool.Definition()
	assert.Equal(t, "list_agents", def.Name)
	assert.NotEmpty(t, def.Description)
}

func TestListAgentsTool_Execute(t *testing.T) {
	var stdoutBuf strings.Builder

	oldStdout := stdoutWriter

	stdoutWriter = &stdoutBuf
	defer func() { stdoutWriter = oldStdout }()

	// Pre-populate stdin with the response.
	response := brokerMessage{
		Type: "list_agents_response",
		Agents: []agentInfo{
			{ID: "agent1", Name: "explore", Status: "running"},
			{ID: "agent2", Name: "coder", Status: "running"},
		},
	}
	respData, _ := json.Marshal(response)

	oldStdin := stdinReader

	stdinReader = strings.NewReader(string(respData) + "\n")
	defer func() { stdinReader = oldStdin }()

	tool := &listAgentsTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "agent1")
	assert.Contains(t, result.Content, "agent2")
	assert.Contains(t, result.Content, "explore")
	assert.Contains(t, result.Content, "coder")

	// Verify request written to stdout.
	line := strings.TrimSpace(stdoutBuf.String())
	require.NotEmpty(t, line)

	var msg brokerMessage
	require.NoError(t, json.Unmarshal([]byte(line), &msg))
	assert.Equal(t, "list_agents", msg.Type)
}

func TestListAgentsTool_ContextCancellation(t *testing.T) {
	var stdoutBuf strings.Builder

	oldStdout := stdoutWriter

	stdoutWriter = &stdoutBuf
	defer func() { stdoutWriter = oldStdout }()

	// Use a pipe that will never receive data.
	r, _ := io.Pipe()
	oldStdin := stdinReader

	stdinReader = r
	defer func() { stdinReader = oldStdin }()

	tool := &listAgentsTool{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	result, err := tool.Execute(ctx, map[string]any{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "context canceled")
}

func TestListAgentsTool_SkipsNonResponseLines(t *testing.T) {
	var stdoutBuf strings.Builder

	oldStdout := stdoutWriter

	stdoutWriter = &stdoutBuf
	defer func() { stdoutWriter = oldStdout }()

	response := brokerMessage{
		Type: "list_agents_response",
		Agents: []agentInfo{
			{ID: "agent1", Name: "explore", Status: "running"},
		},
	}
	respData, _ := json.Marshal(response)

	// Inject some noise before the real response.
	stdinInput := `{"type":"agent_msg","from":"broker","content":"injected context"}
` + string(respData) + "\n"

	oldStdin := stdinReader

	stdinReader = strings.NewReader(stdinInput)
	defer func() { stdinReader = oldStdin }()

	tool := &listAgentsTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "agent1")
}

func TestWriteBrokerMessage(t *testing.T) {
	var buf strings.Builder

	msg := brokerMessage{Type: "send", To: "target", Content: "hello"}

	err := writeBrokerMessage(&buf, msg)
	require.NoError(t, err)

	line := strings.TrimSpace(buf.String())

	var parsed brokerMessage
	require.NoError(t, json.Unmarshal([]byte(line), &parsed))
	assert.Equal(t, "send", parsed.Type)
	assert.Equal(t, "target", parsed.To)
	assert.Equal(t, "hello", parsed.Content)
}

func TestReadListAgentsResponse_Success(t *testing.T) {
	response := brokerMessage{
		Type: "list_agents_response",
		Agents: []agentInfo{
			{ID: "a1", Name: "explore", Status: "running"},
		},
	}
	data, _ := json.Marshal(response)

	result, err := readListAgentsResponse(context.Background(), strings.NewReader(string(data)+"\n"))
	require.NoError(t, err)
	assert.Contains(t, result, "a1")
	assert.Contains(t, result, "explore")
}

func TestReadListAgentsResponse_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readListAgentsResponse(ctx, strings.NewReader(""))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRegisterMessagingTools_WhenSubagentIDSet(t *testing.T) {
	t.Setenv("WEAVE_SUBAGENT_ID", "subagent_test_abc123")
	t.Setenv("WEAVE_MESSAGING", "true")

	sdk.ResetToolRegistry()
	registerMessagingTools()

	assert.True(t, sdk.ToolRegistered("send_message"))
	assert.True(t, sdk.ToolRegistered("broadcast_message"))
	assert.True(t, sdk.ToolRegistered("list_agents"))
}

func TestRegisterMessagingTools_WhenSubagentIDNotSet(t *testing.T) {
	t.Setenv("WEAVE_SUBAGENT_ID", "")

	sdk.ResetToolRegistry()
	registerMessagingTools()

	assert.False(t, sdk.ToolRegistered("send_message"))
	assert.False(t, sdk.ToolRegistered("broadcast_message"))
	assert.False(t, sdk.ToolRegistered("list_agents"))
}

func TestMessagingToolFactories(t *testing.T) {
	t.Setenv("WEAVE_SUBAGENT_ID", "subagent_test_abc123")
	t.Setenv("WEAVE_MESSAGING", "true")

	sdk.ResetToolRegistry()
	registerMessagingTools()

	tool, err := sdk.GetTool("send_message", nil)
	require.NoError(t, err)
	assert.Equal(t, "send_message", tool.Name())

	tool, err = sdk.GetTool("broadcast_message", nil)
	require.NoError(t, err)
	assert.Equal(t, "broadcast_message", tool.Name())

	tool, err = sdk.GetTool("list_agents", nil)
	require.NoError(t, err)
	assert.Equal(t, "list_agents", tool.Name())
}

func TestSendMessageTool_IntegrationWithStdoutCapture(t *testing.T) {
	// Use a pipe to simulate stdout.
	pr, pw := io.Pipe()
	oldStdout := stdoutWriter

	stdoutWriter = pw
	defer func() { stdoutWriter = oldStdout }()

	tool := &sendMessageTool{}

	var captured brokerMessage

	done := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(pr)
		if scanner.Scan() {
			line := scanner.Text()
			_ = json.Unmarshal([]byte(line), &captured)
		}

		_ = pr.Close()

		close(done)
	}()

	result, err := tool.Execute(context.Background(), map[string]any{
		"to":      "agent_target",
		"content": "test content",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	_ = pw.Close()

	<-done

	assert.Equal(t, "send", captured.Type)
	assert.Equal(t, "agent_target", captured.To)
	assert.Equal(t, "test content", captured.Content)
}

func TestBroadcastMessageTool_IntegrationWithStdoutCapture(t *testing.T) {
	pr, pw := io.Pipe()
	oldStdout := stdoutWriter

	stdoutWriter = pw
	defer func() { stdoutWriter = oldStdout }()

	tool := &broadcastMessageTool{}

	var captured brokerMessage

	done := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(pr)
		if scanner.Scan() {
			line := scanner.Text()
			_ = json.Unmarshal([]byte(line), &captured)
		}

		_ = pr.Close()

		close(done)
	}()

	result, err := tool.Execute(context.Background(), map[string]any{
		"content": "broadcast test",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	_ = pw.Close()

	<-done

	assert.Equal(t, "broadcast", captured.Type)
	assert.Equal(t, "broadcast test", captured.Content)
}

func TestListAgentsTool_IntegrationWithStdinStdout(t *testing.T) {
	// Capture stdout.
	var stdoutBuf strings.Builder

	oldStdout := stdoutWriter

	stdoutWriter = &stdoutBuf
	defer func() { stdoutWriter = oldStdout }()

	// Prepare stdin response.
	response := brokerMessage{
		Type: "list_agents_response",
		Agents: []agentInfo{
			{ID: "subagent_explore_abc", Name: "explore", Status: "running"},
			{ID: "subagent_plan_def", Name: "plan", Status: "running"},
		},
	}
	respData, _ := json.Marshal(response)

	oldStdin := stdinReader

	stdinReader = strings.NewReader(string(respData) + "\n")
	defer func() { stdinReader = oldStdin }()

	tool := &listAgentsTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Verify stdout contains the request.
	stdoutLine := strings.TrimSpace(stdoutBuf.String())

	var reqMsg brokerMessage
	require.NoError(t, json.Unmarshal([]byte(stdoutLine), &reqMsg))
	assert.Equal(t, "list_agents", reqMsg.Type)

	// Verify result contains the roster.
	assert.Contains(t, result.Content, "explore")
	assert.Contains(t, result.Content, "plan")
	assert.Contains(t, result.Content, "subagent_explore_abc")
	assert.Contains(t, result.Content, "subagent_plan_def")
}

func TestSendMessageTool_InvalidToType(t *testing.T) {
	tool := &sendMessageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"to":      123,
		"content": "hello",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "non-empty string")
}

func TestSendMessageTool_InvalidContentType(t *testing.T) {
	tool := &sendMessageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"to":      "agent2",
		"content": 456,
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "string")
}

func TestBroadcastMessageTool_InvalidContentType(t *testing.T) {
	tool := &broadcastMessageTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"content": 789,
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "string")
}

func TestReadListAgentsResponse_EmptyInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := readListAgentsResponse(ctx, strings.NewReader(""))
	require.Error(t, err)
}

func TestReadListAgentsResponse_NonJSONLines(t *testing.T) {
	response := brokerMessage{
		Type: "list_agents_response",
		Agents: []agentInfo{
			{ID: "a1", Name: "explore", Status: "running"},
		},
	}
	respData, _ := json.Marshal(response)

	input := fmt.Sprintf("log: starting\nnot json\n%s\n", string(respData))

	result, err := readListAgentsResponse(context.Background(), strings.NewReader(input))
	require.NoError(t, err)
	assert.Contains(t, result, "a1")
}

func TestListAgentsTool_Execute_NoRaceWithListener(t *testing.T) {
	// Set up stdin listener.
	pr, pw := io.Pipe()
	oldStdin := stdinReader

	stdinReader = pr
	defer func() { stdinReader = oldStdin }()

	t.Setenv("WEAVE_SUBAGENT_ID", "subagent_test_123")

	bus := newMockBus()
	startStdinListener(bus)

	defer stopStdinListener()

	// Set up stdout pipe to capture the request and immediately respond.
	stdoutPR, stdoutPW := io.Pipe()
	oldStdout := stdoutWriter

	stdoutWriter = stdoutPW
	defer func() { stdoutWriter = oldStdout }()

	response := brokerMessage{
		Type: "list_agents_response",
		Agents: []agentInfo{
			{ID: "agent1", Name: "explore", Status: "running"},
		},
	}
	respData, _ := json.Marshal(response)

	done := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(stdoutPR)
		if scanner.Scan() {
			_, _ = fmt.Fprintln(pw, string(respData))
		}

		close(done)
	}()

	tool := &listAgentsTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "agent1")
	assert.Contains(t, result.Content, "explore")

	_ = stdoutPW.Close()

	<-done

	_ = pw.Close()
}
