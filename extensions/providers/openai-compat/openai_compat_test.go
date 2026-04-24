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

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

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
				{Role: "tool", Content: "file1.txt\nfile2.txt", ToolCallID: "call_1"},
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
				{Role: "tool", Content: "output", ToolCallID: "call_1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertMessages(tt.msgs)
			require.Equal(t, len(tt.want), len(got))
			for i, w := range tt.want {
				assert.Equal(t, w.Role, got[i].Role)
				assert.Equal(t, w.Content, got[i].Content)
				assert.Equal(t, w.ToolCallID, got[i].ToolCallID)
				assert.Equal(t, len(w.ToolCalls), len(got[i].ToolCalls))
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
		fmt.Fprint(w, response)
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
		sseChunk(ChunkDelta{}, strPtr("stop")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-4o",
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
		sseChunk(ChunkDelta{}, strPtr("tool_calls")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-4o",
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
		sseChunk(ChunkDelta{}, strPtr("tool_calls")),
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
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, strPtr("stop")),
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
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, strPtr("stop")),
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
		fmt.Fprint(w, `{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`)
	}))
	defer server.Close()

	_, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "bad-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid API key")
}

func TestStream_UnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal server error")
	}))
	defer server.Close()

	_, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestStream_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Slowly drip data so the stream stays open
		for i := 0; i < 100; i++ {
			fmt.Fprint(w, sseChunk(ChunkDelta{Content: "x"}, nil))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(10 * time.Millisecond)
		}
		sseDone()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	ch, err := Stream(ctx, server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(ch)
	// Channel should close (possibly with some events or an error)
	_ = events
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
		sseChunk(ChunkDelta{}, strPtr("tool_calls")),
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

	var textParts []string
	var toolCalls []sdk.ToolCall
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

func TestStream_DefaultModel(t *testing.T) {
	var receivedBody ChatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, strPtr("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(ch)

	assert.Equal(t, "gpt-4o", receivedBody.Model)
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
		sseChunk(ChunkDelta{}, strPtr("tool_calls")),
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
		sseChunk(ChunkDelta{}, strPtr("tool_calls")),
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
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseStream(
			sseChunk(ChunkDelta{Content: "ok"}, nil),
			sseChunk(ChunkDelta{}, strPtr("stop")),
			sseDone(),
		))
	}))
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-4o",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	}, sdk.WithModel("gpt-4o-mini"))
	require.NoError(t, err)
	collectEvents(ch)

	assert.Equal(t, "gpt-4o-mini", receivedBody.Model)
}

func TestStream_WithThinkingLevel_SetsReasoningEffort(t *testing.T) {
	tests := []struct {
		level   sdk.ThinkingLevel
		want    string
		wantNot string
	}{
		{sdk.ThinkingOff, "", "reasoning_effort"},
		{sdk.ThinkingMinimal, "low", ""},
		{sdk.ThinkingLow, "low", ""},
		{sdk.ThinkingMedium, "medium", ""},
		{sdk.ThinkingHigh, "high", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			var receivedBody ChatRequest

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewDecoder(r.Body).Decode(&receivedBody)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sseStream(
					sseChunk(ChunkDelta{Content: "ok"}, nil),
					sseChunk(ChunkDelta{}, strPtr("stop")),
					sseDone(),
				))
			}))
			defer server.Close()

			ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
				BaseURL: server.URL,
				APIKey:  "test-key",
				Model:   "gpt-4o",
			}, sdk.ProviderRequest{
				Messages: []sdk.Message{sdk.NewUserMessage("hi")},
			}, sdk.WithThinkingLevel(tt.level))
			require.NoError(t, err)
			collectEvents(ch)

			assert.Equal(t, tt.want, receivedBody.ReasoningEffort)
			if tt.wantNot != "" {
				raw, _ := json.Marshal(receivedBody)
				assert.NotContains(t, string(raw), tt.wantNot)
			}
		})
	}
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
		sseChunk(ChunkDelta{}, strPtr("stop")),
		sseDone(),
	)

	server := setupServer(stream)
	defer server.Close()

	ch, err := Stream(context.Background(), server.Client(), ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("think")},
	}, sdk.WithThinkingLevel(sdk.ThinkingMedium))
	require.NoError(t, err)

	events := collectEvents(ch)

	var thinking []string
	var text []string
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
