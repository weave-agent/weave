package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider(server *httptest.Server, m string) sdk.Provider {
	if m == "" {
		m = "gpt-5.5"
	}

	return &provider{
		client:  server.Client(),
		model:   m,
		baseURL: server.URL,
		oauthToken: sdk.OAuthCredential{
			AccessToken:  makeTestJWT("acct_123"),
			RefreshToken: "rt-test",
			ExpiresAt:    time.Now().Add(time.Hour),
		},
	}
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

// makeTestJWT creates a minimal JWT with a chatgpt_account_id claim for testing.
func makeTestJWT(accountID string) string {
	header := `{"alg":"RS256","typ":"JWT"}`
	payload := fmt.Sprintf(`{"https://api.openai.com/auth":{"chatgpt_account_id":%q}}`, accountID)

	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

// sseEvent builds a single SSE event with an event type and data payload.
func sseEvent(eventType, data string) string {
	return "event: " + eventType + "\ndata: " + data + "\n\n"
}

// sseResponseCompleted emits the response.completed event to end a stream.
func sseResponseCompleted() string {
	return sseEvent("response.completed", `{"type":"response.completed","response":{"status":"completed"}}`)
}

func setupServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = fmt.Fprint(w, response)
	}))
}

func TestStream_TextResponse(t *testing.T) {
	stream := sseEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"Hello!"}`) +
		sseResponseCompleted()

	server := setupServer(stream)
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var textParts []string

	for _, e := range events {
		if e.Type == sdk.ProviderEventTextDelta {
			textParts = append(textParts, e.Content.(string))
		}
	}

	assert.Equal(t, []string{"Hello!"}, textParts)
}

func TestStream_ThinkingDelta(t *testing.T) {
	stream := sseEvent("response.reasoning_summary_text.delta", `{"type":"response.reasoning_summary_text.delta","delta":"thinking..."}`) +
		sseResponseCompleted()

	server := setupServer(stream)
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var thinking []string

	for _, e := range events {
		if e.Type == sdk.ProviderEventThinking {
			thinking = append(thinking, e.Content.(string))
		}
	}

	assert.Equal(t, []string{"thinking..."}, thinking)
}

func TestStream_ReasoningTextDelta(t *testing.T) {
	stream := sseEvent("response.reasoning_text.delta", `{"type":"response.reasoning_text.delta","delta":"reasoning..."}`) +
		sseResponseCompleted()

	server := setupServer(stream)
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var thinking []string

	for _, e := range events {
		if e.Type == sdk.ProviderEventThinking {
			thinking = append(thinking, e.Content.(string))
		}
	}

	assert.Equal(t, []string{"reasoning..."}, thinking)
}

func TestStream_ToolCall(t *testing.T) {
	stream := sseEvent("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_abc","call_id":"call_abc","name":"bash"}}`) +
		sseEvent("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"command\":"}`) +
		sseEvent("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","output_index":0,"delta":"\"ls\"}"}`) +
		sseEvent("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call"}}`) +
		sseResponseCompleted()

	server := setupServer(stream)
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("run ls")},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

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
	stream := sseEvent("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"bash"}}`) +
		sseEvent("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"command\":\"ls\"}"}`) +
		sseEvent("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call"}}`) +
		sseEvent("response.output_item.added", `{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"read"}}`) +
		sseEvent("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"path\":\"/tmp\"}"}`) +
		sseEvent("response.output_item.done", `{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call"}}`) +
		sseResponseCompleted()

	server := setupServer(stream)
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("do stuff")},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

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
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseResponseCompleted())
	}))
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		SystemPrompt: "You are helpful.",
		Messages:     []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Equal(t, "You are helpful.", receivedBody["instructions"])
}

func TestStream_WithTools(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseResponseCompleted())
	}))
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
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
	collectEvents(t, ch)

	tools, ok := receivedBody["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)

	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "bash", tool["name"])
	assert.Equal(t, "function", tool["type"])
	assert.Equal(t, "auto", receivedBody["tool_choice"])
}

func TestStream_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Invalid token","type":"invalid_request_error"}}`)
	}))
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	_, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid token")
}

func TestStream_ResponseFailed(t *testing.T) {
	stream := sseEvent("response.failed", `{"type":"response.failed","response":{"error":{"message":"rate limit exceeded"}}}`)

	server := setupServer(stream)
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var hasError bool

	for _, e := range events {
		if e.Type == sdk.ProviderEventError {
			hasError = true

			assert.Contains(t, fmt.Sprint(e.Content), "rate limit exceeded")
		}
	}

	assert.True(t, hasError)
}

func TestStream_SendsCorrectEndpoint(t *testing.T) {
	var receivedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseResponseCompleted())
	}))
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Equal(t, "/codex/responses", receivedPath)
}

func TestStream_SendsCodexHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseResponseCompleted())
	}))
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Equal(t, "responses=experimental", receivedHeaders.Get("OpenAI-Beta"))
	assert.Equal(t, "weave", receivedHeaders.Get("originator"))
	assert.Equal(t, "acct_123", receivedHeaders.Get("chatgpt-account-id"))
	assert.Contains(t, receivedHeaders.Get("Authorization"), "Bearer ")
}

func TestStream_ThinkingLevel(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"off", ""},
		{"minimal", "low"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"xhigh", "high"},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var receivedBody map[string]any

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)

				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, sseResponseCompleted())
			}))
			defer server.Close()

			p := newTestProvider(server, "gpt-5.5")
			ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
				Messages: []sdk.Message{sdk.NewUserMessage("hi")},
			}, model.WithThinkingLevel(model.ThinkingLevel(tt.level)))
			require.NoError(t, err)
			collectEvents(t, ch)

			if tt.want == "" {
				assert.Nil(t, receivedBody["reasoning"])
			} else {
				reasoning, ok := receivedBody["reasoning"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.want, reasoning["effort"])
			}
		})
	}
}

func TestStream_RefreshesExpiredOAuthToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	var chatAuth string

	chatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseResponseCompleted())
	}))
	defer chatServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			return
		}

		assert.Equal(t, "refresh_token", r.FormValue("grant_type"))
		assert.Equal(t, "rt-old", r.FormValue("refresh_token"))

		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  makeTestJWT("acct_new"),
			"expires_in":    3600,
			"refresh_token": "rt-new",
		})
	}))
	defer tokenServer.Close()

	oldCred := sdk.OAuthCredential{
		AccessToken:  makeTestJWT("acct_old"),
		RefreshToken: "rt-old",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}
	require.NoError(t, sdk.SetOAuthCredential("codex", oldCred))

	p := &provider{
		client:     chatServer.Client(),
		model:      "gpt-5.5",
		baseURL:    chatServer.URL,
		tokenURL:   tokenServer.URL,
		oauthToken: oldCred,
	}

	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Contains(t, chatAuth, "Bearer ")
}

func TestExtractAccountID(t *testing.T) {
	token := makeTestJWT("acct_abc123")
	id, err := extractAccountID(token)
	require.NoError(t, err)
	assert.Equal(t, "acct_abc123", id)
}

func TestExtractAccountID_InvalidToken(t *testing.T) {
	_, err := extractAccountID("not-a-jwt")
	require.Error(t, err)
}

func TestExtractAccountID_MissingClaim(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user123"}`))
	token := header + "." + payload + ".sig"

	_, err := extractAccountID(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing chatgpt_account_id")
}

func TestExtractAccountID_InvalidBase64(t *testing.T) {
	// Valid JWT structure but invalid base64 in payload section.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	token := header + ".!!!invalid-base64!!!.sig"

	_, err := extractAccountID(token)
	require.Error(t, err)
}

func TestStream_MalformedToolCallArguments(t *testing.T) {
	stream := sseEvent("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_abc","call_id":"call_abc","name":"bash"}}`) +
		sseEvent("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{not-valid-json"}`) +
		sseEvent("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call"}}`) +
		sseResponseCompleted()

	server := setupServer(stream)
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("run ls")},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var hasError bool

	for _, e := range events {
		if e.Type == sdk.ProviderEventError {
			hasError = true

			assert.Contains(t, fmt.Sprint(e.Content), "parse tool call arguments")
		}
	}

	assert.True(t, hasError)
}

func TestStream_ArgsDeltaWithoutItemAdded(t *testing.T) {
	// Args delta arrives before item.added, creating an orphan accumulator.
	stream := sseEvent("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"command\":\"ls\"}"}`) +
		sseEvent("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_abc","call_id":"call_abc","name":"bash"}}`) +
		sseEvent("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call"}}`) +
		sseResponseCompleted()

	server := setupServer(stream)
	defer server.Close()

	p := newTestProvider(server, "gpt-5.5")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("run ls")},
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)

	var toolCalls []sdk.ToolCall

	for _, e := range events {
		if e.Type == sdk.ProviderEventToolCall {
			toolCalls = append(toolCalls, e.Content.(sdk.ToolCall))
		}
	}

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "bash", toolCalls[0].Name)
}

func TestStream_DefaultModel(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseResponseCompleted())
	}))
	defer server.Close()

	p := newTestProvider(server, "")
	ch, err := p.Stream(context.Background(), sdk.ProviderRequest{
		Messages: []sdk.Message{sdk.NewUserMessage("hi")},
	})
	require.NoError(t, err)
	collectEvents(t, ch)

	assert.Equal(t, "gpt-5.5", receivedBody["model"])
}

func TestRegister(t *testing.T) {
	assert.True(t, sdk.ProviderRegistered("codex"))
}
