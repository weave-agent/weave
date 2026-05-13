package kimi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave/sdk"
	"weave/sdk/model"
)

type sseEvent struct {
	EventType string
	Data      string
}

func writeSSE(w http.ResponseWriter, events []sseEvent) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	for _, evt := range events {
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.EventType, evt.Data)

		flusher.Flush()
	}
}

func textStreamEvents(text string) []sseEvent {
	return []sseEvent{
		{EventType: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"kimi-for-coding","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{EventType: "content_block_delta", Data: fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`, text)},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{EventType: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		{EventType: "message_stop", Data: `{"type":"message_stop"}`},
	}
}

func toolCallEvents(toolID, toolName, inputJSON string) []sseEvent {
	return []sseEvent{
		{EventType: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"kimi-for-coding","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":20,"output_tokens":1}}}`},
		{EventType: "content_block_start", Data: fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":%q,"name":%q,"input":{}}}`, toolID, toolName)},
		{EventType: "content_block_delta", Data: fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%q}}`, inputJSON)},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{EventType: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":50}}`},
		{EventType: "message_stop", Data: `{"type":"message_stop"}`},
	}
}

func textAndToolEvents(text, toolID, toolName, inputJSON string) []sseEvent {
	return []sseEvent{
		{EventType: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"kimi-for-coding","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":30,"output_tokens":1}}}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{EventType: "content_block_delta", Data: fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`, text)},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{EventType: "content_block_start", Data: fmt.Sprintf(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":%q,"name":%q,"input":{}}}`, toolID, toolName)},
		{EventType: "content_block_delta", Data: fmt.Sprintf(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":%q}}`, inputJSON)},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":1}`},
		{EventType: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":60}}`},
		{EventType: "message_stop", Data: `{"type":"message_stop"}`},
	}
}

func newTestProvider(server *httptest.Server) sdk.Provider {
	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	return NewProviderWithClient(client, "kimi-for-coding")
}

func collectEvents(t *testing.T, ch <-chan sdk.ProviderEvent) []sdk.ProviderEvent {
	t.Helper()

	var events []sdk.ProviderEvent

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return events
			}

			events = append(events, evt)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for events")
		}
	}
}

func TestStream_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, textStreamEvents("Hello, world!"))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{
			sdk.NewUserMessage("Say hello"),
		},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var textDeltas []string

	for _, evt := range events {
		if evt.Type == sdk.ProviderEventTextDelta {
			textDeltas = append(textDeltas, evt.Content.(string))
		}
	}

	assert.Equal(t, []string{"Hello, world!"}, textDeltas)
}

func TestStream_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, toolCallEvents("toolu_123", "bash", `{"command":"ls -la"}`))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{
			sdk.NewUserMessage("List files"),
		},
		Tools: []sdk.ToolDef{
			{
				Name:        "bash",
				Description: "Run a bash command",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string"},
					},
					"required": []string{"command"},
				},
			},
		},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var toolCalls []sdk.ToolCall

	for _, evt := range events {
		if evt.Type == sdk.ProviderEventToolCall {
			toolCalls = append(toolCalls, evt.Content.(sdk.ToolCall))
		}
	}

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "toolu_123", toolCalls[0].ID)
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Equal(t, map[string]any{"command": "ls -la"}, toolCalls[0].Arguments)
}

func TestStream_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"invalid model"}}`)
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{
			sdk.NewUserMessage("Hello"),
		},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var errorMsgs []string

	for _, evt := range events {
		if evt.Type == sdk.ProviderEventError {
			errorMsgs = append(errorMsgs, evt.Content.(string))
		}
	}

	require.Len(t, errorMsgs, 1)
	assert.Contains(t, errorMsgs[0], "invalid model")
}

func TestStream_WithSystemPrompt(t *testing.T) {
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		receivedBody = string(buf)

		writeSSE(w, textStreamEvents("response"))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		SystemPrompt: "You are a helpful assistant.",
		Messages: []sdk.Message{
			sdk.NewUserMessage("Hello"),
		},
	})
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Contains(t, receivedBody, "You are a helpful assistant.")
}

func TestStream_WithThinkingLevel(t *testing.T) {
	model.ResetModelRegistry()
	defer model.ResetModelRegistry()

	RegisterModels()

	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		receivedBody = string(buf)

		writeSSE(w, textStreamEvents("thinking response"))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("think")},
	}, model.WithThinkingLevel(model.ThinkingHigh))
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Contains(t, receivedBody, `"thinking"`)
	assert.Contains(t, receivedBody, `"adaptive"`)
	assert.Contains(t, receivedBody, `"output_config"`)
	assert.Contains(t, receivedBody, `"effort"`)
	assert.Contains(t, receivedBody, `"high"`)
}

func TestStream_ThinkingOff_NoThinkingParam(t *testing.T) {
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		receivedBody = string(buf)

		writeSSE(w, textStreamEvents("no thinking"))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hello")},
	}, model.WithThinkingLevel(model.ThinkingOff))
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.NotContains(t, receivedBody, `"thinking"`)
}

func TestStream_ThinkingContentEmitted(t *testing.T) {
	events := []sseEvent{
		{EventType: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"kimi-for-coding","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"let me think"}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":1}`},
		{EventType: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		{EventType: "message_stop", Data: `{"type":"message_stop"}`},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, events)
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("think")},
	}, model.WithThinkingLevel(model.ThinkingMedium))
	require.NoError(t, err)

	evts := collectEvents(t, ch)

	var (
		thinkingDeltas []string
		textDeltas     []string
	)

	for _, evt := range evts {
		switch evt.Type {
		case sdk.ProviderEventThinking:
			thinkingDeltas = append(thinkingDeltas, evt.Content.(string))
		case sdk.ProviderEventTextDelta:
			textDeltas = append(textDeltas, evt.Content.(string))
		}
	}

	assert.Equal(t, []string{"let me think"}, thinkingDeltas)
	assert.Equal(t, []string{"answer"}, textDeltas)
}

func TestStream_TextAndToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, textAndToolEvents("I'll list the files.", "toolu_456", "bash", `{"command":"ls"}`))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{
			sdk.NewUserMessage("List files"),
		},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var (
		textDeltas []string
		toolCalls  []sdk.ToolCall
	)

	for _, evt := range events {
		switch evt.Type {
		case sdk.ProviderEventTextDelta:
			textDeltas = append(textDeltas, evt.Content.(string))
		case sdk.ProviderEventToolCall:
			toolCalls = append(toolCalls, evt.Content.(sdk.ToolCall))
		}
	}

	assert.Equal(t, []string{"I'll list the files."}, textDeltas)
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "bash", toolCalls[0].Name)
}

func TestStream_EmptyMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, textStreamEvents("Hi"))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var textDeltas []string

	for _, evt := range events {
		if evt.Type == sdk.ProviderEventTextDelta {
			textDeltas = append(textDeltas, evt.Content.(string))
		}
	}

	assert.Equal(t, []string{"Hi"}, textDeltas)
}

func TestStream_MultipleToolCalls(t *testing.T) {
	events := []sseEvent{
		{EventType: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"kimi-for-coding","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":20,"output_tokens":1}}}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_2","name":"read","input":{}}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"/tmp/test\"}"}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":1}`},
		{EventType: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":80}}`},
		{EventType: "message_stop", Data: `{"type":"message_stop"}`},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, events)
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{
			sdk.NewUserMessage("Run ls then read a file"),
		},
	})
	require.NoError(t, err)

	collected := collectEvents(t, ch)

	var toolCalls []sdk.ToolCall

	for _, evt := range collected {
		if evt.Type == sdk.ProviderEventToolCall {
			toolCalls = append(toolCalls, evt.Content.(sdk.ToolCall))
		}
	}

	require.Len(t, toolCalls, 2)
	assert.Equal(t, "toolu_1", toolCalls[0].ID)
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Equal(t, "toolu_2", toolCalls[1].ID)
	assert.Equal(t, "read", toolCalls[1].Name)
}

func TestStream_SplitToolInputJSON(t *testing.T) {
	events := []sseEvent{
		{EventType: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"kimi-for-coding","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":20,"output_tokens":1}}}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_split","name":"bash","input":{}}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"com"}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"mand\":\"ls\"}"}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{EventType: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":30}}`},
		{EventType: "message_stop", Data: `{"type":"message_stop"}`},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, events)
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{
			sdk.NewUserMessage("Run ls"),
		},
	})
	require.NoError(t, err)

	collected := collectEvents(t, ch)

	var toolCalls []sdk.ToolCall

	for _, evt := range collected {
		if evt.Type == sdk.ProviderEventToolCall {
			toolCalls = append(toolCalls, evt.Content.(sdk.ToolCall))
		}
	}

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Equal(t, map[string]any{"command": "ls"}, toolCalls[0].Arguments)
}

func TestStream_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`)
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{
			sdk.NewUserMessage("Hello"),
		},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var errorMsgs []string

	for _, evt := range events {
		if evt.Type == sdk.ProviderEventError {
			errorMsgs = append(errorMsgs, evt.Content.(string))
		}
	}

	require.Len(t, errorMsgs, 1)
	assert.Contains(t, errorMsgs[0], "invalid api key")
}

func TestStream_NetworkError(t *testing.T) {
	handlerErr := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			handlerErr <- errors.New("streaming not supported")
			return
		}

		// Write partial SSE then close connection by hijacking
		_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")

		flusher.Flush()

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			handlerErr <- errors.New("server must support hijacking")
			return
		}

		conn, _, err := hijacker.Hijack()
		if err != nil {
			handlerErr <- fmt.Errorf("hijack failed: %w", err)
			return
		}

		_ = conn.Close()
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{
			sdk.NewUserMessage("Hello"),
		},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	select {
	case err := <-handlerErr:
		t.Fatalf("handler error: %v", err)
	default:
	}

	var errorMsgs []string

	for _, evt := range events {
		if evt.Type == sdk.ProviderEventError {
			errorMsgs = append(errorMsgs, evt.Content.(string))
		}
	}

	require.Len(t, errorMsgs, 1)
	assert.NotEmpty(t, errorMsgs[0])
}

func TestConvertMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []sdk.Message
		wantLen  int
		check    func(t *testing.T, params []anthropic.MessageParam)
	}{
		{
			name:     "empty",
			messages: []sdk.Message{},
			wantLen:  0,
			check:    func(t *testing.T, params []anthropic.MessageParam) {},
		},
		{
			name: "user message",
			messages: []sdk.Message{
				sdk.NewUserMessage("Hello"),
			},
			wantLen: 1,
			check: func(t *testing.T, params []anthropic.MessageParam) {
				assert.Equal(t, anthropic.MessageParamRoleUser, params[0].Role)
			},
		},
		{
			name: "user and assistant",
			messages: []sdk.Message{
				sdk.NewUserMessage("Hello"),
				sdk.NewAssistantMessage("Hi there"),
			},
			wantLen: 2,
			check: func(t *testing.T, params []anthropic.MessageParam) {
				assert.Equal(t, anthropic.MessageParamRoleUser, params[0].Role)
				assert.Equal(t, anthropic.MessageParamRoleAssistant, params[1].Role)
			},
		},
		{
			name: "tool result groups into single user message",
			messages: []sdk.Message{
				sdk.NewUserMessage("List files"),
				{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{
					{ID: "toolu_1", Name: "bash", Arguments: map[string]any{"command": "ls"}},
					{ID: "toolu_2", Name: "read", Arguments: map[string]any{"path": "/tmp"}},
				}},
				sdk.NewToolResultMessage("toolu_1", "bash", "file1.txt\nfile2.txt", false),
				sdk.NewToolResultMessage("toolu_2", "read", "content", false),
			},
			wantLen: 3, // user + assistant + grouped tool results
			check: func(t *testing.T, params []anthropic.MessageParam) {
				assert.Equal(t, anthropic.MessageParamRoleUser, params[0].Role)
				assert.Equal(t, anthropic.MessageParamRoleAssistant, params[1].Role)
				assert.Equal(t, anthropic.MessageParamRoleUser, params[2].Role)
				// Verify both tool results are in the third message
				blocks := params[2].Content
				require.Len(t, blocks, 2)
			},
		},
		{
			name: "tool result with error",
			messages: []sdk.Message{
				sdk.NewToolResultMessage("toolu_err", "bash", "command not found", true),
			},
			wantLen: 1,
			check: func(t *testing.T, params []anthropic.MessageParam) {
				assert.Equal(t, anthropic.MessageParamRoleUser, params[0].Role)
				blocks := params[0].Content
				require.Len(t, blocks, 1)
				assert.Equal(t, "toolu_err", *blocks[0].GetToolUseID())
				assert.True(t, *blocks[0].GetIsError())
			},
		},
		{
			name: "assistant with thinking and redacted thinking",
			messages: []sdk.Message{
				{Role: sdk.RoleAssistant, Content: "answer", Thinking: []sdk.SignedThinking{
					{Signature: "sig123", Thinking: "thinking text"},
				}, RedactedThinking: []sdk.RedactedThinking{
					{Data: "redacted_data"},
				}},
			},
			wantLen: 1,
			check: func(t *testing.T, params []anthropic.MessageParam) {
				assert.Equal(t, anthropic.MessageParamRoleAssistant, params[0].Role)
				blocks := params[0].Content
				require.Len(t, blocks, 3)
				assert.NotNil(t, blocks[0].OfThinking)
				assert.NotNil(t, blocks[1].OfRedactedThinking)
				assert.Equal(t, "answer", *blocks[2].GetText())
			},
		},
		{
			name: "assistant with only tool calls",
			messages: []sdk.Message{
				{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{
					{ID: "toolu_1", Name: "bash", Arguments: map[string]any{"command": "ls"}},
				}},
			},
			wantLen: 1,
			check: func(t *testing.T, params []anthropic.MessageParam) {
				assert.Equal(t, anthropic.MessageParamRoleAssistant, params[0].Role)
				blocks := params[0].Content
				require.Len(t, blocks, 1)
				require.NotNil(t, blocks[0].OfToolUse)
				assert.Equal(t, "toolu_1", blocks[0].OfToolUse.ID)
			},
		},
		{
			name: "assistant with nil content",
			messages: []sdk.Message{
				{Role: sdk.RoleAssistant, Content: nil},
			},
			wantLen: 0,
			check:   func(t *testing.T, params []anthropic.MessageParam) {},
		},
		{
			name: "user message with non-string content",
			messages: []sdk.Message{
				{Role: sdk.RoleUser, Content: 42},
			},
			wantLen: 1,
			check: func(t *testing.T, params []anthropic.MessageParam) {
				blocks := params[0].Content
				require.Len(t, blocks, 1)
				assert.Equal(t, "42", *blocks[0].GetText())
			},
		},
		{
			name: "interleaved user and tool results",
			messages: []sdk.Message{
				sdk.NewUserMessage("first"),
				sdk.NewToolResultMessage("toolu_1", "bash", "out1", false),
				sdk.NewUserMessage("second"),
				sdk.NewToolResultMessage("toolu_2", "bash", "out2", false),
			},
			wantLen: 4,
			check: func(t *testing.T, params []anthropic.MessageParam) {
				assert.Equal(t, anthropic.MessageParamRoleUser, params[0].Role)
				assert.Equal(t, anthropic.MessageParamRoleUser, params[1].Role)
				assert.Equal(t, anthropic.MessageParamRoleUser, params[2].Role)
				assert.Equal(t, anthropic.MessageParamRoleUser, params[3].Role)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := convertMessages(tt.messages)
			assert.Len(t, params, tt.wantLen)

			if tt.check != nil {
				tt.check(t, params)
			}
		})
	}
}

func TestConvertTools(t *testing.T) {
	tools := []sdk.ToolDef{
		{
			Name:        "bash",
			Description: "Run a command",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The command",
					},
				},
				"required": []string{"command"},
			},
		},
	}

	result := convertTools(tools)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].OfTool)
	assert.Equal(t, "bash", result[0].OfTool.Name)
	assert.True(t, result[0].OfTool.Description.Valid())
	assert.Equal(t, "Run a command", result[0].OfTool.Description.Value)
	require.NotNil(t, result[0].OfTool.InputSchema)
	assert.Equal(t, []string{"command"}, result[0].OfTool.InputSchema.Required)
	props, ok := result[0].OfTool.InputSchema.Properties.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "command")
}

func TestConvertTools_NilParameters(t *testing.T) {
	tools := []sdk.ToolDef{
		{
			Name:        "bash",
			Description: "Run a command",
		},
	}

	result := convertTools(tools)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].OfTool)
	require.NotNil(t, result[0].OfTool.InputSchema)
	assert.Nil(t, result[0].OfTool.InputSchema.Properties)
	assert.Empty(t, result[0].OfTool.InputSchema.Required)
}

func TestConvertTools_RequiredAsAnySlice(t *testing.T) {
	tools := []sdk.ToolDef{
		{
			Name:        "bash",
			Description: "Run a command",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
				},
				"required": []any{"command"},
			},
		},
	}

	result := convertTools(tools)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].OfTool)
	assert.Equal(t, []string{"command"}, result[0].OfTool.InputSchema.Required)
}

func TestConvertTools_RequiredAsAnySliceWithNonStrings(t *testing.T) {
	tools := []sdk.ToolDef{
		{
			Name:        "bash",
			Description: "Run a command",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
					"force":   map[string]any{"type": "boolean"},
				},
				"required": []any{"command", 42, true, "force"},
			},
		},
	}

	result := convertTools(tools)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].OfTool)
	assert.Equal(t, []string{"command", "force"}, result[0].OfTool.InputSchema.Required)
}

func TestConvertTools_NonMapParameters(t *testing.T) {
	tools := []sdk.ToolDef{
		{
			Name:        "bash",
			Description: "Run a command",
			Parameters:  "invalid",
		},
	}

	result := convertTools(tools)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].OfTool)
	require.NotNil(t, result[0].OfTool.InputSchema)
	assert.Nil(t, result[0].OfTool.InputSchema.Properties)
	assert.Empty(t, result[0].OfTool.InputSchema.Required)
}

func TestResolveThinkingLevel_Clamping(t *testing.T) {
	model.ResetModelRegistry()
	defer model.ResetModelRegistry()

	RegisterModels()

	// kimi-for-coding supports xhigh
	level := resolveThinkingLevel("kimi-for-coding", model.ThinkingXHigh)
	assert.Equal(t, model.ThinkingXHigh, level)

	// Model without xhigh support should clamp to high
	model.RegisterModel(model.ModelDef{
		ID: "no-xhigh", Provider: "kimi",
		Reasoning: true, SupportsXHigh: false,
	})

	level = resolveThinkingLevel("no-xhigh", model.ThinkingXHigh)
	assert.Equal(t, model.ThinkingHigh, level)

	// Non-reasoning model should turn off thinking
	model.RegisterModel(model.ModelDef{
		ID: "no-reasoning", Provider: "kimi",
		Reasoning: false,
	})

	level = resolveThinkingLevel("no-reasoning", model.ThinkingHigh)
	assert.Equal(t, model.ThinkingOff, level)

	// ThinkingOff should stay off regardless of model
	level = resolveThinkingLevel("kimi-for-coding", model.ThinkingOff)
	assert.Equal(t, model.ThinkingOff, level)

	// Unknown model should return level unchanged
	level = resolveThinkingLevel("unknown-model", model.ThinkingHigh)
	assert.Equal(t, model.ThinkingHigh, level)
}

func TestStream_ThinkingDoneEmitted(t *testing.T) {
	events := []sseEvent{
		{EventType: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"kimi-for-coding","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":"sig_abc123"}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"let me think"}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":1}`},
		{EventType: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		{EventType: "message_stop", Data: `{"type":"message_stop"}`},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, events)
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("think")},
	}, model.WithThinkingLevel(model.ThinkingMedium))
	require.NoError(t, err)

	evts := collectEvents(t, ch)

	var thinkingDone []sdk.SignedThinking

	for _, evt := range evts {
		if evt.Type == sdk.ProviderEventThinkingDone {
			thinkingDone = append(thinkingDone, evt.Content.(sdk.SignedThinking))
		}
	}

	require.Len(t, thinkingDone, 1)
	assert.Equal(t, "let me think", thinkingDone[0].Thinking)
	assert.Equal(t, "sig_abc123", thinkingDone[0].Signature)
}

func TestStream_RedactedThinking(t *testing.T) {
	events := []sseEvent{
		{EventType: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"kimi-for-coding","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"redacted_data_123"}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":1}`},
		{EventType: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		{EventType: "message_stop", Data: `{"type":"message_stop"}`},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, events)
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("think")},
	}, model.WithThinkingLevel(model.ThinkingMedium))
	require.NoError(t, err)

	evts := collectEvents(t, ch)

	var redacted []sdk.RedactedThinking

	for _, evt := range evts {
		if evt.Type == sdk.ProviderEventRedactedThinkingDone {
			redacted = append(redacted, evt.Content.(sdk.RedactedThinking))
		}
	}

	require.Len(t, redacted, 1)
	assert.Equal(t, "redacted_data_123", redacted[0].Data)
}

func TestStream_WithModelOverride(t *testing.T) {
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		receivedBody = string(buf)

		writeSSE(w, textStreamEvents("response"))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hello")},
	}, model.WithModel("kimi-test-model"))
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Contains(t, receivedBody, "kimi-test-model")
}

func TestStream_WithMaxTokensOverride(t *testing.T) {
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		receivedBody = string(buf)

		writeSSE(w, textStreamEvents("hi"))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hello")},
	}, model.WithMaxTokens(12345))
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Contains(t, receivedBody, "12345")
}

func TestNewProviderWithClient_EmptyModel(t *testing.T) {
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		receivedBody = string(buf)

		writeSSE(w, textStreamEvents("hi"))
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	p := NewProviderWithClient(client, "")
	require.NotNil(t, p)

	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hello")},
	})
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Contains(t, receivedBody, "kimi-for-coding")
}

func TestStream_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, textStreamEvents("Hello"))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := p.Stream(ctx, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hello")},
	})
	require.NoError(t, err)

	// Cancel immediately
	cancel()

	// Collect events - should get empty or partial results without hanging
	done := make(chan []sdk.ProviderEvent)

	go func() {
		var events []sdk.ProviderEvent
		for evt := range ch {
			events = append(events, evt)
		}

		done <- events
	}()

	select {
	case events := <-done:
		// Channel closed properly - no panic or hang
		_ = events
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canceled stream to close")
	}
}

func TestStream_NoSystemPromptWhenEmpty(t *testing.T) {
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		receivedBody = string(buf)

		writeSSE(w, textStreamEvents("hi"))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hello")},
	})
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.NotContains(t, receivedBody, `"system"`)
}

func TestStream_ParseToolArgs_EmptyJSON(t *testing.T) {
	// Empty JSON input - should produce empty map
	events := []sseEvent{
		{EventType: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"kimi-for-coding","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":20,"output_tokens":1}}}`},
		{EventType: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_empty","name":"bash","input":{}}}`},
		{EventType: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`},
		{EventType: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{EventType: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":50}}`},
		{EventType: "message_stop", Data: `{"type":"message_stop"}`},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, events)
	}))
	defer server.Close()

	p := newTestProvider(server)
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("Run")},
	})
	require.NoError(t, err)

	collected := collectEvents(t, ch)

	var toolCalls []sdk.ToolCall

	for _, evt := range collected {
		if evt.Type == sdk.ProviderEventToolCall {
			toolCalls = append(toolCalls, evt.Content.(sdk.ToolCall))
		}
	}

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Empty(t, toolCalls[0].Arguments)
}

func TestParseToolArgs_MalformedJSON(t *testing.T) {
	var sent bool

	send := func(evt sdk.ProviderEvent) bool {
		sent = true
		return true
	}

	args := parseToolArgs("bash", "{not valid json", send)
	assert.NotNil(t, args)
	assert.Empty(t, args)
	assert.True(t, sent)
}

func TestParseToolArgs_EmptyRaw(t *testing.T) {
	send := func(evt sdk.ProviderEvent) bool { return true }

	args := parseToolArgs("bash", "", send)
	assert.NotNil(t, args)
	assert.Empty(t, args)
}
