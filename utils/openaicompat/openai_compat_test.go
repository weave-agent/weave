package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/model"
	"github.com/weave-agent/weave/sdk/retry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertMessages(t *testing.T) {
	tests := []struct {
		name string
		msgs []sdk.Message
		want []ChatMessage
	}{
		{
			name: "user message",
			msgs: []sdk.Message{sdk.NewUserMessage("hello")},
			want: []ChatMessage{{Role: "user", Content: "hello"}},
		},
		{
			name: "assistant text message",
			msgs: []sdk.Message{sdk.NewAssistantMessage("hi there")},
			want: []ChatMessage{{Role: "assistant", Content: "hi there"}},
		},
		{
			name: "assistant with tool calls",
			msgs: []sdk.Message{
				{
					Role:    sdk.RoleAssistant,
					Content: "",
					ToolCalls: []sdk.ToolCall{
						{ID: "call_1", Name: "bash", Arguments: map[string]any{"command": "ls"}},
					},
				},
			},
			want: []ChatMessage{
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []StreamTool{
						{
							ID:   "call_1",
							Type: "function",
							Function: struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							}{Name: "bash", Arguments: `{"command":"ls"}`},
						},
					},
				},
			},
		},
		{
			name: "tool result message",
			msgs: []sdk.Message{
				sdk.NewToolResultMessage("call_1", "bash", "file1.txt\nfile2.txt", false),
			},
			want: []ChatMessage{
				{Role: "tool", Content: "<tool_output name=\"bash\">\nfile1.txt\nfile2.txt\n</tool_output>", ToolCallID: "call_1"},
			},
		},
		{
			name: "mixed messages",
			msgs: []sdk.Message{
				sdk.NewUserMessage("run ls"),
				{
					Role:    sdk.RoleAssistant,
					Content: "",
					ToolCalls: []sdk.ToolCall{
						{ID: "call_1", Name: "bash", Arguments: map[string]any{"command": "ls"}},
					},
				},
				sdk.NewToolResultMessage("call_1", "bash", "output", false),
			},
			want: []ChatMessage{
				{Role: "user", Content: "run ls"},
				{
					Role: "assistant",
					ToolCalls: []StreamTool{
						{
							ID:   "call_1",
							Type: "function",
							Function: struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							}{Name: "bash", Arguments: `{"command":"ls"}`},
						},
					},
				},
				{Role: "tool", Content: "<tool_output name=\"bash\">\noutput\n</tool_output>", ToolCallID: "call_1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertMessages(tt.msgs)
			require.Len(t, got, len(tt.want))

			for i, w := range tt.want {
				assert.Equal(t, w.Role, got[i].Role)
				assert.Equal(t, w.Content, got[i].Content)
				assert.Equal(t, w.ToolCallID, got[i].ToolCallID)
				assert.Len(t, got[i].ToolCalls, len(w.ToolCalls))

				for j, tc := range w.ToolCalls {
					assert.Equal(t, tc.ID, got[i].ToolCalls[j].ID)
					assert.Equal(t, tc.Type, got[i].ToolCalls[j].Type)
					assert.Equal(t, tc.Function.Name, got[i].ToolCalls[j].Function.Name)
					assert.JSONEq(t, tc.Function.Arguments, got[i].ToolCalls[j].Function.Arguments)
				}
			}
		})
	}
}

func TestConvertTools(t *testing.T) {
	tools := []sdk.ToolDef{
		{
			Name:        "bash",
			Description: "Run a bash command",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
				},
			},
		},
	}

	got := ConvertTools(tools)
	require.Len(t, got, 1)
	assert.Equal(t, "function", got[0].Type)
	assert.Equal(t, "bash", got[0].Function.Name)
	assert.Equal(t, "Run a bash command", got[0].Function.Description)
}

func TestConvertToolsEmpty(t *testing.T) {
	got := ConvertTools(nil)
	assert.Nil(t, got)

	got = ConvertTools([]sdk.ToolDef{})
	assert.Nil(t, got)
}

func sseChunk(delta ChunkDelta, finish *string) string {
	chunk := StreamChunk{
		ID: "chatcmpl-test",
		Choices: []struct {
			Index        int        `json:"index"`
			Delta        ChunkDelta `json:"delta"`
			FinishReason *string    `json:"finish_reason"`
		}{
			{Index: 0, Delta: delta, FinishReason: finish},
		},
	}
	data, _ := json.Marshal(chunk)

	return "data: " + string(data) + "\n"
}

func sseUsageChunk(usage Usage) string {
	chunk := StreamChunk{
		ID:    "chatcmpl-test",
		Usage: &usage,
	}
	data, _ := json.Marshal(chunk)

	return "data: " + string(data) + "\n"
}

func sseDone() string {
	return "data: [DONE]\n"
}

func sseStream(events ...string) string {
	return strings.Join(events, "") + "\n"
}

func setupServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = fmt.Fprint(w, response)
	}))
}

func collectEvents(ch <-chan sdk.ProviderEvent) []sdk.ProviderEvent {
	var events []sdk.ProviderEvent
	for e := range ch {
		events = append(events, e)
	}

	return events
}

func TestStream_TextOnly(t *testing.T) {
	stream := sseStream(
		sseChunk(ChunkDelta{Role: "assistant"}, nil),
		sseChunk(ChunkDelta{Content: "Hello"}, nil),
		sseChunk(ChunkDelta{Content: " world"}, nil),
		sseChunk(ChunkDelta{}, new("stop")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-5.5",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)

	var textParts []string

	for _, e := range events {
		if e.Type == sdk.ProviderEventTextDelta {
			textParts = append(textParts, e.Content.(string))
		}
	}

	assert.Equal(t, []string{"Hello", " world"}, textParts)
	assert.Equal(t, sdk.ProviderEventTextDelta, events[len(events)-1].Type)
}

func TestStream_UsageWithoutCachedPromptDetails(t *testing.T) {
	stream := sseStream(
		sseChunk(ChunkDelta{Content: "ok"}, nil),
		sseUsageChunk(Usage{InputTokens: 11, OutputTokens: 3}),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-5.5",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)

	var usages []sdk.ProviderUsage
	for _, e := range events {
		if e.Type == sdk.ProviderEventUsage {
			usages = append(usages, e.Content.(sdk.ProviderUsage))
		}
	}

	require.Len(t, usages, 1)
	assert.Equal(t, 11, usages[0].InputTokens)
	assert.Equal(t, 3, usages[0].OutputTokens)
	assert.Zero(t, usages[0].CacheReadTokens)
}

func TestStream_UsageWithCachedPromptDetails(t *testing.T) {
	stream := sseStream(
		sseChunk(ChunkDelta{Content: "ok"}, nil),
		`data: {"id":"chatcmpl-test","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":13}}}`+"\n",
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-5.5",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)

	var usages []sdk.ProviderUsage
	for _, e := range events {
		if e.Type == sdk.ProviderEventUsage {
			usages = append(usages, e.Content.(sdk.ProviderUsage))
		}
	}

	require.Len(t, usages, 1)
	assert.Equal(t, 20, usages[0].InputTokens)
	assert.Equal(t, 4, usages[0].OutputTokens)
	assert.Equal(t, 13, usages[0].CacheReadTokens)
}

func TestStream_ToolCall(t *testing.T) {
	stream := sseStream(
		sseChunk(ChunkDelta{Role: "assistant"}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, ID: "call_abc", Type: "function", Function: &FunctionCallDelta{Name: "bash"}},
			},
		}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, Function: &FunctionCallDelta{Arguments: `{"command":"ls"}`}},
			},
		}, nil),
		sseChunk(ChunkDelta{}, new("tool_calls")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-5.5",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("run ls")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)

	var toolCalls []sdk.ToolCall

	for _, e := range events {
		if e.Type == sdk.ProviderEventToolCall {
			toolCalls = append(toolCalls, e.Content.(sdk.ToolCall))
		}
	}

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_abc", toolCalls[0].ID)
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Equal(t, map[string]any{"command": "ls"}, toolCalls[0].Arguments)
}

func TestStream_MultipleToolCalls(t *testing.T) {
	stream := sseStream(
		sseChunk(ChunkDelta{Role: "assistant"}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, ID: "call_1", Type: "function", Function: &FunctionCallDelta{Name: "bash"}},
				{Index: 1, ID: "call_2", Type: "function", Function: &FunctionCallDelta{Name: "read"}},
			},
		}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, Function: &FunctionCallDelta{Arguments: `{"command":"ls"}`}},
				{Index: 1, Function: &FunctionCallDelta{Arguments: `{"path":"/tmp/file"}`}},
			},
		}, nil),
		sseChunk(ChunkDelta{}, new("tool_calls")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("do stuff")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)

	var toolCalls []sdk.ToolCall

	for _, e := range events {
		if e.Type == sdk.ProviderEventToolCall {
			toolCalls = append(toolCalls, e.Content.(sdk.ToolCall))
		}
	}

	require.Len(t, toolCalls, 2)
	names := []string{toolCalls[0].Name, toolCalls[1].Name}
	assert.Contains(t, names, "bash")
	assert.Contains(t, names, "read")
}

func TestStream_WithSystemPrompt(t *testing.T) {
	var receivedBody ChatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, new("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		SystemPrompt: "You are helpful.",
		Messages:     []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(ch)

	require.NotEmpty(t, receivedBody.Messages)
	assert.Equal(t, "system", receivedBody.Messages[0].Role)
	assert.Equal(t, "You are helpful.", receivedBody.Messages[0].Content)
	assert.Equal(t, "user", receivedBody.Messages[1].Role)
}

func TestStream_WithTools(t *testing.T) {
	var receivedBody ChatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, new("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
		Tools: []sdk.ToolDef{
			{
				Name:        "bash",
				Description: "Run a command",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string"},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	collectEvents(ch)

	require.Len(t, receivedBody.Tools, 1)
	assert.Equal(t, "bash", receivedBody.Tools[0].Function.Name)
}

func TestStream_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`)
	}))
	defer server.Close()

	_, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "bad-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.Error(t, err)

	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, ErrorTypeAuth, apiErr.Type)
	assert.Equal(t, 401, apiErr.StatusCode)
	assert.Equal(t, "Invalid API key", apiErr.Message)
}

func TestStream_UnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "internal server error")
	}))
	defer server.Close()

	_, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		RetryConfig: &retry.Config{
			MaxRetries: 0,
		},
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.Error(t, err)

	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, ErrorTypeServer, apiErr.Type)
	assert.Equal(t, 500, apiErr.StatusCode)
	assert.Contains(t, apiErr.Body, "internal server error")
}

func TestStream_ContextCancellation(t *testing.T) {
	firstChunkSent := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		_, _ = fmt.Fprint(w, sseChunk(ChunkDelta{Content: "x"}, nil))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		close(firstChunkSent)

		// Block until client disconnects.
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := Stream(ctx, server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	// Wait for the server to send the first chunk, then give the
	// parseSSE goroutine time to read it before canceling.
	<-firstChunkSent
	time.Sleep(50 * time.Millisecond)
	cancel()

	events := collectEvents(ch)
	// Verify the channel closed and we got at least some events before cancellation.
	assert.NotEmpty(t, events, "expected at least one event before cancellation")
}

func TestStream_MixedTextAndToolCalls(t *testing.T) {
	stream := sseStream(
		sseChunk(ChunkDelta{Role: "assistant"}, nil),
		sseChunk(ChunkDelta{Content: "Let me check"}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, ID: "call_1", Type: "function", Function: &FunctionCallDelta{Name: "bash"}},
			},
		}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, Function: &FunctionCallDelta{Arguments: `{"command":"ls"}`}},
			},
		}, nil),
		sseChunk(ChunkDelta{}, new("tool_calls")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("check files")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)

	var (
		textParts []string
		toolCalls []sdk.ToolCall
	)

	for _, e := range events {
		switch e.Type {
		case sdk.ProviderEventTextDelta:
			textParts = append(textParts, e.Content.(string))
		case sdk.ProviderEventToolCall:
			toolCalls = append(toolCalls, e.Content.(sdk.ToolCall))
		}
	}

	assert.Equal(t, []string{"Let me check"}, textParts)
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "bash", toolCalls[0].Name)
}

func TestStream_PartialJSONArguments(t *testing.T) {
	// Simulate real-world streaming where JSON arguments arrive in fragments
	stream := sseStream(
		sseChunk(ChunkDelta{Role: "assistant"}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, ID: "call_1", Type: "function", Function: &FunctionCallDelta{Name: "bash"}},
			},
		}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, Function: &FunctionCallDelta{Arguments: `{"com`}},
			},
		}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, Function: &FunctionCallDelta{Arguments: `mand":"ls -la"}`}},
			},
		}, nil),
		sseChunk(ChunkDelta{}, new("tool_calls")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("run ls")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)

	var toolCalls []sdk.ToolCall

	for _, e := range events {
		if e.Type == sdk.ProviderEventToolCall {
			toolCalls = append(toolCalls, e.Content.(sdk.ToolCall))
		}
	}

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Equal(t, map[string]any{"command": "ls -la"}, toolCalls[0].Arguments)
}

func TestStream_ToolCallOrdering(t *testing.T) {
	stream := sseStream(
		sseChunk(ChunkDelta{Role: "assistant"}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 2, ID: "call_2", Type: "function", Function: &FunctionCallDelta{Name: "read"}},
				{Index: 0, ID: "call_0", Type: "function", Function: &FunctionCallDelta{Name: "bash"}},
				{Index: 1, ID: "call_1", Type: "function", Function: &FunctionCallDelta{Name: "edit"}},
			},
		}, nil),
		sseChunk(ChunkDelta{
			ToolCalls: []ToolCallDelta{
				{Index: 0, Function: &FunctionCallDelta{Arguments: `{"command":"ls"}`}},
				{Index: 1, Function: &FunctionCallDelta{Arguments: `{"path":"a"}`}},
				{Index: 2, Function: &FunctionCallDelta{Arguments: `{"path":"b"}`}},
			},
		}, nil),
		sseChunk(ChunkDelta{}, new("tool_calls")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("do stuff")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)

	var toolCalls []sdk.ToolCall

	for _, e := range events {
		if e.Type == sdk.ProviderEventToolCall {
			toolCalls = append(toolCalls, e.Content.(sdk.ToolCall))
		}
	}

	require.Len(t, toolCalls, 3)
	// Tool calls should be emitted in index order, not arrival order
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Equal(t, "edit", toolCalls[1].Name)
	assert.Equal(t, "read", toolCalls[2].Name)
}

func TestStream_WithModelOverride(t *testing.T) {
	var receivedBody ChatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, new("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-5.5",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	}, model.WithModel("gpt-4.1"))
	require.NoError(t, err)
	collectEvents(ch)

	assert.Equal(t, "gpt-4.1", receivedBody.Model)
}

func TestStream_ReasoningContentEmitted(t *testing.T) {
	chunk := StreamChunk{
		ID: "chatcmpl-test",
		Choices: []struct {
			Index        int        `json:"index"`
			Delta        ChunkDelta `json:"delta"`
			FinishReason *string    `json:"finish_reason"`
		}{
			{Index: 0, Delta: ChunkDelta{ReasoningContent: "thinking step"}, FinishReason: nil},
		},
	}
	data, _ := json.Marshal(chunk)

	stream := sseStream(
		sseChunk(ChunkDelta{Role: "assistant"}, nil),
		"data: "+string(data)+"\n",
		sseChunk(ChunkDelta{Content: "answer"}, nil),
		sseChunk(ChunkDelta{}, new("stop")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("think")},
	}, model.WithThinkingLevel(model.ThinkingMedium))
	require.NoError(t, err)

	events := collectEvents(ch)

	var (
		thinking []string
		text     []string
	)

	for _, e := range events {
		switch e.Type {
		case sdk.ProviderEventThinking:
			thinking = append(thinking, e.Content.(string))
		case sdk.ProviderEventTextDelta:
			text = append(text, e.Content.(string))
		}
	}

	assert.Equal(t, []string{"thinking step"}, thinking)
	assert.Equal(t, []string{"answer"}, text)
}

func TestError_ErrorMethod(t *testing.T) {
	err := &Error{Type: ErrorTypeAuth, Message: "bad key"}
	assert.Equal(t, "openai-compat: auth: bad key", err.Error())
}

func TestError_IsRetriable(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want bool
	}{
		{"rate_limit", &Error{Type: ErrorTypeRateLimit}, true},
		{"server", &Error{Type: ErrorTypeServer}, true},
		{"transport", &Error{Type: ErrorTypeTransport}, true},
		{"auth", &Error{Type: ErrorTypeAuth}, false},
		{"client", &Error{Type: ErrorTypeClient}, false},
		{"parse", &Error{Type: ErrorTypeParse}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.IsRetriable())
		})
	}
}

func TestError_Classification(t *testing.T) {
	tests := []struct {
		code int
		want ErrorType
	}{
		{401, ErrorTypeAuth},
		{403, ErrorTypeAuth},
		{429, ErrorTypeRateLimit},
		{500, ErrorTypeServer},
		{502, ErrorTypeServer},
		{503, ErrorTypeServer},
		{400, ErrorTypeClient},
		{404, ErrorTypeClient},
		{418, ErrorTypeClient},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.code), func(t *testing.T) {
			assert.Equal(t, tt.want, classifyStatus(tt.code))
		})
	}
}

func TestStream_ExtraHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, new("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		ExtraHeaders: map[string]string{
			"X-Custom-Auth": "token-123",
			"X-Request-ID":  "req-abc",
		},
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(ch)

	assert.Equal(t, "token-123", receivedHeaders.Get("X-Custom-Auth"))
	assert.Equal(t, "req-abc", receivedHeaders.Get("X-Request-ID"))
	assert.Equal(t, "Bearer test-key", receivedHeaders.Get("Authorization"))
}

func TestStream_ExtraBody(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&receivedBody))

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, new("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-5.5",
		ExtraBody: map[string]any{
			"temperature": 0.7,
			"seed":        float64(42),
		},
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(ch)

	assert.InDelta(t, 0.7, receivedBody["temperature"], 0.001)
	assert.InDelta(t, float64(42), receivedBody["seed"], 0.001)
	assert.Equal(t, "gpt-5.5", receivedBody["model"])
}

func TestStream_ExtraBody_NoOverrideCoreFields(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&receivedBody))

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, new("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-5.5",
		ExtraBody: map[string]any{
			"model": "should-be-ignored",
		},
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(ch)

	assert.Equal(t, "gpt-5.5", receivedBody["model"])
}

func TestStream_RetryOn429(t *testing.T) {
	attemptCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`)

			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, new("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		RetryConfig: &retry.Config{
			MaxRetries: 2,
			BaseDelay:  1 * time.Millisecond,
			MaxDelay:   10 * time.Millisecond,
			Multiplier: 2,
		},
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)
	require.NotEmpty(t, events)
	assert.Equal(t, 3, attemptCount, "expected 3 attempts (2 failures + 1 success)")
}

func TestStream_RetryDeterministicWithJitterNone(t *testing.T) {
	attemptCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"error":{"message":"Server unavailable","type":"server_error"}}`)

			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, new("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		RetryConfig: &retry.Config{
			MaxRetries: 3,
			BaseDelay:  5 * time.Millisecond,
			MaxDelay:   50 * time.Millisecond,
			Multiplier: 2,
			Jitter:     retry.JitterNone,
		},
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)
	require.NotEmpty(t, events)
	assert.Equal(t, 3, attemptCount, "expected 3 attempts with jitter=none")
}

func TestStream_RateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`)
	}))
	defer server.Close()

	_, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		RetryConfig: &retry.Config{
			MaxRetries: 0,
		},
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.Error(t, err)

	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, ErrorTypeRateLimit, apiErr.Type)
	assert.Equal(t, 429, apiErr.StatusCode)
}

func TestStream_OversizedErrorBodyIsCapped(t *testing.T) {
	oversizedBody := strings.Repeat("x", maxErrorBodySize+100)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, oversizedBody)
	}))
	defer server.Close()

	_, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		RetryConfig: &retry.Config{
			MaxRetries: 0,
		},
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.Error(t, err)

	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, ErrorTypeServer, apiErr.Type)
	assert.Equal(t, 500, apiErr.StatusCode)
	assert.Contains(t, apiErr.Body, "... (truncated)")
	assert.Len(t, apiErr.Body, maxErrorBodySize+len("... (truncated)"))
}

func TestStream_StructuredErrorParsingUnderCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Model not found","type":"invalid_request_error"}}`)
	}))
	defer server.Close()

	_, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		RetryConfig: &retry.Config{
			MaxRetries: 0,
		},
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.Error(t, err)

	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, ErrorTypeClient, apiErr.Type)
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.Equal(t, "Model not found", apiErr.Message)
	assert.NotContains(t, apiErr.Body, "truncated")
}
