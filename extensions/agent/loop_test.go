package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"weave/bus"
	"weave/sdk"
	"weave/sdk/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

type providerResponse struct {
	textDeltas     []string
	thinkingDeltas []string
	toolCalls      []sdk.ToolCall
	err            error
}

func newMockProvider(responses []providerResponse) *ProviderMock {
	var mu sync.Mutex

	callCount := 0

	return &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()
			idx := callCount
			callCount++
			mu.Unlock()

			if idx >= len(responses) {
				ch := make(chan sdk.ProviderEvent)
				close(ch)

				return ch, nil
			}

			resp := responses[idx]
			if resp.err != nil {
				return nil, resp.err
			}

			bufSize := len(resp.textDeltas) + len(resp.thinkingDeltas) + len(resp.toolCalls) + 1
			ch := make(chan sdk.ProviderEvent, bufSize)

			go func() {
				defer close(ch)

				for _, delta := range resp.thinkingDeltas {
					select {
					case ch <- sdk.ProviderEvent{Type: sdk.ProviderEventThinking, Content: delta}:
					case <-ctx.Done():
						return
					}
				}

				for _, delta := range resp.textDeltas {
					select {
					case ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: delta}:
					case <-ctx.Done():
						return
					}
				}

				for _, tc := range resp.toolCalls {
					select {
					case ch <- sdk.ProviderEvent{Type: sdk.ProviderEventToolCall, Content: tc}:
					case <-ctx.Done():
						return
					}
				}
			}()

			return ch, nil
		},
	}
}

func newMockTool(name string, def sdk.ToolDef, executeFunc func(ctx context.Context, args map[string]any) (sdk.ToolResult, error)) *ToolMock {
	mt := &ToolMock{
		NameFunc:       func() string { return name },
		DefinitionFunc: func() sdk.ToolDef { return def },
	}
	if executeFunc != nil {
		mt.ExecuteFunc = executeFunc
	}

	return mt
}

func setupAgent(t *testing.T, providerName string) (*AgentExtension, *bus.Bus, func()) {
	t.Helper()

	a, err := NewAgentExtension(nil, nil, CompactionConfig{})
	require.NoError(t, err, "NewAgentExtension")

	a.providerName = providerName

	b := bus.New()

	return a, b, func() {
		_ = b.Close()
	}
}

func registerMockProvider(_ string, mp *ProviderMock) {
	sdk.RegisterProvider[struct{}, struct{}]("anthropic", func(sdk.Config, struct{}, struct{}) (sdk.Provider, error) {
		return mp, nil
	})
}

func registerMockTool(mt *ToolMock) {
	sdk.RegisterTool[struct{}](mt.NameFunc(), func(sdk.Config, sdk.PreferenceReader, struct{}) (sdk.Tool, error) {
		return mt, nil
	})
}

// subscribeAllToChan creates an internal channel and registers an OnAll handler
// that forwards all bus events to it. Returns the channel for reading.
func subscribeAllToChan(b *bus.Bus) <-chan sdk.Event {
	ch := make(chan sdk.Event, 256)

	b.OnAll(func(ev sdk.Event) error {
		select {
		case ch <- ev:
		default:
		}

		return nil
	})

	return ch
}

func waitForTopic(events <-chan sdk.Event, topic string, timeout time.Duration) (sdk.Event, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return sdk.Event{}, false
			}

			if evt.Topic == topic {
				return evt, true
			}
		case <-timer.C:
			return sdk.Event{}, false
		}
	}
}

func collectTopic(events <-chan sdk.Event, topic string, timeout time.Duration) []sdk.Event {
	var result []sdk.Event

	deadline := time.After(timeout)

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return result
			}

			if evt.Topic == topic {
				result = append(result, evt)
			}
		case <-deadline:
			return result
		}
	}
}

// mockPrefsConfig is a lightweight sdk.Config that returns a fixed provider from Preferences.
type mockPrefsConfig struct {
	sdk.Config
	provider      string
	model         string
	thinkingLevel string
	prefsErr      error
}

func (m *mockPrefsConfig) FilePath() string                         { return "" }
func (m *mockPrefsConfig) ProjectDir() string                       { return "" }
func (m *mockPrefsConfig) ExtensionConfig(_, _ string, _ any) error { return nil }
func (m *mockPrefsConfig) IsHeadless() bool                         { return true }

func (m *mockPrefsConfig) Preferences(target any) error {
	if m.prefsErr != nil {
		return m.prefsErr
	}

	type prefs struct {
		Provider      string `json:"provider,omitempty"`
		Model         string `json:"model,omitempty"`
		ThinkingLevel string `json:"thinking_level,omitempty"`
	}

	p := prefs{
		Provider:      m.provider,
		Model:         m.model,
		ThinkingLevel: m.thinkingLevel,
	}

	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal preferences: %w", err)
	}

	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("unmarshal preferences: %w", err)
	}

	return nil
}
func (m *mockPrefsConfig) SavePreferences(any) error         { return nil }
func (m *mockPrefsConfig) SaveProviderKey(_, _ string) error { return nil }
func (m *mockPrefsConfig) RespectGitignore() bool            { return true }

// --- tests ---

func TestAgent_SingleTurn_NoTools(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"hello", " world"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test prompt"))

	turnStart, found := waitForTopic(allCh, TopicTurnStart, 2*time.Second)
	require.True(t, found, "timeout waiting for turn_start")

	count, ok := turnStart.Payload.(int)
	require.True(t, ok, "turn_start payload type = %T", turnStart.Payload)
	assert.Equal(t, 1, count)

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")
	payload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, "hello world", payload["content"])

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 1)
	assert.Len(t, calls[0].Req.Messages, 1)
}

func TestAgent_ToolCallCycle(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("bash", sdk.ToolDef{Name: "bash", Description: "run commands"}, nil)
	registerMockTool(mt)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "echo hi"}},
			},
		},
		{textDeltas: []string{"done"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "run echo"))

	toolCallEvt, ok := waitForTopic(allCh, TopicToolCall, 2*time.Second)
	require.True(t, ok, "timeout waiting for tool_call")

	toolCallPayload, ok := toolCallEvt.Payload.(map[string]any)
	require.True(t, ok, "tool_call payload type = %T", toolCallEvt.Payload)
	assert.Equal(t, "bash", toolCallPayload["tool"])

	toolResultEvt, ok := waitForTopic(allCh, TopicToolResult, 2*time.Second)
	require.True(t, ok, "timeout waiting for tool_result")

	payload, ok := toolResultEvt.Payload.(map[string]any)
	require.True(t, ok, "tool_result payload type = %T", toolResultEvt.Payload)
	assert.Equal(t, "bash", payload["tool"])

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second msg_end")
	msgEndPayload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, "done", msgEndPayload["content"])

	execCalls := mt.ExecuteCalls()
	require.Len(t, execCalls, 1)
	assert.Equal(t, "echo hi", execCalls[0].Args["command"])

	assert.Len(t, mp.StreamCalls(), 2)
}

func TestAgent_SteeringInjection(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first"}},
		{textDeltas: []string{"steered"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))
	b.Publish(sdk.NewEvent(TopicSteer, "steer this"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 1)
	assert.Len(t, calls[0].Req.Messages, 2, "expected 2 messages (prompt + steering)")
}

func TestAgent_SteeringDuringTurn(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	streamingStarted := make(chan struct{})

	responses := []providerResponse{
		{textDeltas: []string{"first"}},
		{textDeltas: []string{"steered"}},
	}

	mp := newMockProvider(responses)

	originalStreamFunc := mp.StreamFunc
	mp.StreamFunc = func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
		<-chan sdk.ProviderEvent, error,
	) {
		callIdx := len(mp.StreamCalls()) - 1
		if callIdx == 0 {
			close(streamingStarted)
			time.Sleep(100 * time.Millisecond)
		}

		return originalStreamFunc(ctx, req)
	}

	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	<-streamingStarted
	b.Publish(sdk.NewEvent(TopicSteer, "steer during turn"))

	for i := range 2 {
		_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
		require.True(t, ok, "timeout waiting for turn_end %d", i+1)
	}

	require.NoError(t, a.Close())

	_, ok := waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	assert.Len(t, mp.StreamCalls(), 2)
}

func TestAgent_FollowupReentry(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first response"}},
		{textDeltas: []string{"follow-up response"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// Send initial prompt
	b.Publish(sdk.NewEvent(TopicPrompt, "initial"))

	// Wait for the first turn to complete (real-world timing: user reads response)
	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Send follow-up after the first turn completes (not before)
	b.Publish(sdk.NewEvent(TopicFollowup, "follow up question"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	assert.Len(t, mp.StreamCalls(), 2)
}

func TestAgent_PromptResetsConversation(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first response"}},
		{textDeltas: []string{"new conversation"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// First conversation
	b.Publish(sdk.NewEvent(TopicPrompt, "first prompt"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Simulate /new: send a second agent.prompt to reset the conversation
	b.Publish(sdk.NewEvent(TopicPrompt, "new prompt after /new"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 2)
	// First call: 1 message (just the first prompt)
	assert.Len(t, calls[0].Req.Messages, 1)
	// Second call: 1 message (conversation was reset, only the new prompt)
	assert.Len(t, calls[1].Req.Messages, 1)
}

func TestAgent_ErrorAbort(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{err: context.DeadlineExceeded},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "trigger error"))

	_, ok := waitForTopic(allCh, TopicTurnStart, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_start")

	endEvt, ok := waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")
	payload, ok := endEvt.Payload.(string)
	require.True(t, ok, "end payload should be string, got %T", endEvt.Payload)
	assert.Contains(t, payload, "stream error:")
}

func TestAgent_ProviderErrorOnStartup(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	a, b, cleanup := setupAgent(t, "nonexistent")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	endEvts := collectTopic(allCh, TopicEnd, 2*time.Second)
	require.Len(t, endEvts, 1)
	payload, ok := endEvts[0].Payload.(string)
	require.True(t, ok, "end payload should be string, got %T", endEvts[0].Payload)
	assert.Contains(t, payload, "No providers configured")
}

func TestAgent_ContextCancellation(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	blockCh := make(chan sdk.ProviderEvent)

	registerMockProvider("anthropic", &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			ch := make(chan sdk.ProviderEvent)

			go func() {
				defer close(ch)

				select {
				case <-ctx.Done():
				case blockCh <- sdk.ProviderEvent{}:
				}
			}()

			return ch, nil
		},
	})

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	done := make(chan error, 1)

	go func() { done <- a.Close() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Close hung — context cancellation did not unblock the loop")
	}

	_, ok := waitForTopic(allCh, TopicEnd, time.Second)
	assert.True(t, ok, "expected TopicEnd after cancellation")
}

func TestAgent_MsgUpdateEvents(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"a", "b", "c"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "stream test"))

	updates := collectTopic(allCh, TopicMsgUpdate, 2*time.Second)
	require.Len(t, updates, 3)

	expected := []string{"a", "b", "c"}
	for i, u := range updates {
		assert.Equal(t, expected[i], u.Payload)
	}
}

func TestAgent_MissingToolError(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "nonexistent", Arguments: nil},
			},
		},
		{textDeltas: []string{"recovered"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "call missing tool"))

	toolResultEvt, ok := waitForTopic(allCh, TopicToolResult, 2*time.Second)
	require.True(t, ok, "timeout waiting for tool_result")

	payload := toolResultEvt.Payload.(map[string]any)
	result := payload["result"].(sdk.ToolResult)
	assert.True(t, result.IsError)

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second msg_end")
	msgEndPayload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, "recovered", msgEndPayload["content"])
}

func TestAgent_MultipleToolCalls(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	tool1 := newMockTool("tool-a", sdk.ToolDef{Name: "tool-a"}, nil)
	tool2 := newMockTool("tool-b", sdk.ToolDef{Name: "tool-b"}, nil)

	registerMockTool(tool1)
	registerMockTool(tool2)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "tool-a", Arguments: map[string]any{"x": 1}},
				{ID: "tc2", Name: "tool-b", Arguments: map[string]any{"y": 2}},
			},
		},
		{textDeltas: []string{"both done"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "multi-tool"))

	var (
		toolResults []sdk.Event
		finalMsgEnd *sdk.Event
	)

	endDeadline := time.After(5 * time.Second)

	for {
		select {
		case evt, ok := <-allCh:
			require.True(t, ok, "event channel closed")

			switch evt.Topic {
			case TopicToolResult:
				toolResults = append(toolResults, evt)
			case TopicMsgEnd:
				e := evt
				finalMsgEnd = &e
				// After both tool results are in, the next msg_end is the
				// assistant's follow-up response. Close to end cleanly.
				if len(toolResults) == 2 {
					require.NoError(t, a.Close())
				}
			case TopicTurnEnd:
				// Let the loop continue to the next turn.
			case TopicEnd:
				goto done
			}
		case <-endDeadline:
			t.Fatal("timeout waiting for end")
		}
	}

done:

	require.Len(t, toolResults, 2)

	require.NotNil(t, finalMsgEnd)
	msgEndPayload, ok := finalMsgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", finalMsgEnd.Payload)
	assert.Equal(t, "both done", msgEndPayload["content"])
}

func TestAgent_StreamingUpdatesPreserveOrder(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	deltas := make([]string, 20)
	for i := range deltas {
		deltas[i] = strings.Repeat("x", i+1)
	}

	mp := newMockProvider([]providerResponse{
		{textDeltas: deltas},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "order test"))

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 3*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	expected := strings.Join(deltas, "")
	msgEndPayload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, expected, msgEndPayload["content"])
}

func TestAgent_InterruptHaltsTurn(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	started := make(chan struct{})

	responses := []providerResponse{
		{textDeltas: []string{"partial"}},
		{textDeltas: []string{"after interrupt"}},
	}

	mp := newMockProvider(responses)
	originalStreamFunc := mp.StreamFunc
	mp.StreamFunc = func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
		<-chan sdk.ProviderEvent, error,
	) {
		callIdx := len(mp.StreamCalls()) - 1
		if callIdx == 0 {
			close(started)
			// Give the interrupt time to arrive and cancel the context
			time.Sleep(200 * time.Millisecond)
		}

		return originalStreamFunc(ctx, req)
	}

	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	<-started
	b.Publish(sdk.NewEvent(TopicInterrupt, "user interrupt"))

	// The interrupted turn should eventually publish TurnEnd
	_, ok := waitForTopic(allCh, TopicTurnEnd, 3*time.Second)
	require.True(t, ok, "timeout waiting for turn_end after interrupt")

	// The loop should stay alive and accept a follow-up
	b.Publish(sdk.NewEvent(TopicFollowup, "continue"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 3*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end after followup")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	// Two stream calls: first (interrupted) and second (follow-up)
	assert.Len(t, mp.StreamCalls(), 2)
}

func TestAgent_ThinkingContentInMsgEnd(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{
			thinkingDeltas: []string{"let me think", "... carefully"},
			textDeltas:     []string{"here is the answer"},
		},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "think about it"))

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	payload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, "here is the answer", payload["content"])
	assert.Equal(t, "let me think... carefully", payload["thinking"])

	require.NoError(t, a.Close())
}

func TestAgent_NoThinkingKeyWhenEmpty(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"no thinking here"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "quick one"))

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	payload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, "no thinking here", payload["content"])
	_, hasThinking := payload["thinking"]
	assert.False(t, hasThinking, "should not have thinking key when no thinking deltas")

	require.NoError(t, a.Close())
}

func TestAgent_ThinkingLevelChange(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var mu sync.Mutex

	var capturedOpts []model.StreamOption

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()
			capturedOpts = opts
			mu.Unlock()

			ch := make(chan sdk.ProviderEvent, 1)
			ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "ok"}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "first"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first msg_end")

	// Default thinking level should be medium
	mu.Lock()
	require.Len(t, capturedOpts, 1)
	so := model.NewStreamOptions(capturedOpts...)
	mu.Unlock()
	assert.Equal(t, model.ThinkingMedium, so.ThinkingLevel)

	// Change thinking level to high
	b.Publish(sdk.NewEvent(TopicThinkingChange, map[string]string{"level": "high"}))

	// Send a follow-up to trigger a new stream call with updated thinking level
	b.Publish(sdk.NewEvent(TopicFollowup, "after thinking change"))

	_, ok = waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second msg_end")

	mu.Lock()
	require.Len(t, capturedOpts, 1)
	so = model.NewStreamOptions(capturedOpts...)
	mu.Unlock()
	assert.Equal(t, model.ThinkingHigh, so.ThinkingLevel)

	require.NoError(t, a.Close())
}

func TestAgent_ModelChangeWithModelKey(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var capturedOpts []model.StreamOption

	var mu sync.Mutex

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()
			capturedOpts = opts
			mu.Unlock()

			ch := make(chan sdk.ProviderEvent, 1)
			ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "ok"}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first msg_end")

	// Initially no model option — only thinking level
	mu.Lock()
	require.Len(t, capturedOpts, 1)
	so := model.NewStreamOptions(capturedOpts...)
	mu.Unlock()
	assert.Empty(t, so.Model)

	// Switch model within same provider (the bug fix case).
	// Model change should NOT trigger a spurious streamTurn — it applies on the next user input.
	b.Publish(sdk.NewEvent(TopicModelChange, map[string]string{
		"provider": "anthropic",
		"model":    "claude-opus-4-7",
	}))

	// Send a follow-up to trigger a stream call with the updated model.
	b.Publish(sdk.NewEvent(TopicFollowup, "after model change"))

	_, ok = waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for follow-up msg_end")

	mu.Lock()
	require.Len(t, capturedOpts, 2)
	so = model.NewStreamOptions(capturedOpts...)
	mu.Unlock()
	assert.Equal(t, "claude-opus-4-7", so.Model, "model should be passed via StreamOptions")

	require.NoError(t, a.Close())
}

func TestAgent_ModelChangeDifferentProvider(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	anthropicMock := newMockProvider([]providerResponse{
		{textDeltas: []string{"anthropic response"}},
	})
	openaiMock := newMockProvider([]providerResponse{
		{textDeltas: []string{"openai response"}},
	})

	sdk.RegisterProvider[struct{}, struct{}]("anthropic", func(sdk.Config, struct{}, struct{}) (sdk.Provider, error) {
		return anthropicMock, nil
	})
	sdk.RegisterProvider[struct{}, struct{}]("openai", func(sdk.Config, struct{}, struct{}) (sdk.Provider, error) {
		return openaiMock, nil
	})

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	msgEnd1, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first msg_end")

	payload1 := msgEnd1.Payload.(map[string]any)
	assert.Equal(t, "anthropic response", payload1["content"])

	assert.Len(t, anthropicMock.StreamCalls(), 1)

	// Switch to openai provider — model change applies on next user input
	b.Publish(sdk.NewEvent(TopicModelChange, map[string]string{
		"provider": "openai",
		"model":    "gpt-5.5",
	}))

	// Send a follow-up to trigger a turn with the new provider
	b.Publish(sdk.NewEvent(TopicFollowup, "after provider switch"))

	msgEnd2, ok := waitForTopic(allCh, TopicMsgEnd, 4*time.Second)
	require.True(t, ok, "timeout waiting for openai msg_end")

	payload2 := msgEnd2.Payload.(map[string]any)
	assert.Equal(t, "openai response", payload2["content"])

	require.NoError(t, a.Close())

	// Verify openai was called and model was passed
	require.Len(t, openaiMock.StreamCalls(), 1)
	openaiOpts := openaiMock.StreamCalls()[0].Opts
	so := model.NewStreamOptions(openaiOpts...)
	assert.Equal(t, "gpt-5.5", so.Model)
}

func TestAgent_InnerLoopStepLimit(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("bash", sdk.ToolDef{Name: "bash", Description: "run commands"}, nil)
	registerMockTool(mt)

	// Provider always returns a tool call, so the inner loop would run forever
	// without the step limit.
	mp := newMockProvider([]providerResponse{
		{toolCalls: []sdk.ToolCall{{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "echo hi"}}}},
		{toolCalls: []sdk.ToolCall{{ID: "tc2", Name: "bash", Arguments: map[string]any{"command": "echo hi"}}}},
		{toolCalls: []sdk.ToolCall{{ID: "tc3", Name: "bash", Arguments: map[string]any{"command": "echo hi"}}}},
		{toolCalls: []sdk.ToolCall{{ID: "tc4", Name: "bash", Arguments: map[string]any{"command": "echo hi"}}}},
	})
	registerMockProvider("anthropic", mp)

	// Use a low step limit so the test runs fast.
	a, err := NewAgentExtension(nil, nil, CompactionConfig{MaxSteps: 3})
	require.NoError(t, err)

	a.providerName = "anthropic"

	b := bus.New()
	defer b.Close()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "loop test"))

	// Wait for the step-limit exceeded event.
	var compactedEvt sdk.Event

	found := false
	deadline := time.After(5 * time.Second)

	for !found {
		select {
		case evt, ok := <-allCh:
			require.True(t, ok, "event channel closed")

			if evt.Topic == TopicCompacted {
				compactedEvt = evt
				found = true
			}

			if evt.Topic == TopicEnd {
				t.Fatal("got TopicEnd before step-limit exceeded event")
			}
		case <-deadline:
			t.Fatal("timeout waiting for step-limit exceeded event")
		}
	}

	payload, ok := compactedEvt.Payload.(map[string]any)
	require.True(t, ok, "compacted payload type = %T", compactedEvt.Payload)
	errMsg, ok := payload["error"].(string)
	require.True(t, ok, "error field type = %T", payload["error"])
	assert.Contains(t, errMsg, "step limit exceeded")
	assert.Contains(t, errMsg, "3")

	// The loop should have broken out and eventually ended.
	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	// Should have executed 3 tool calls before hitting the limit.
	assert.Len(t, mt.ExecuteCalls(), 3)
}

func TestAgent_StepLimitConfigurable(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("bash", sdk.ToolDef{Name: "bash", Description: "run commands"}, nil)
	registerMockTool(mt)

	mp := newMockProvider([]providerResponse{
		{toolCalls: []sdk.ToolCall{{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "echo 1"}}}},
		{toolCalls: []sdk.ToolCall{{ID: "tc2", Name: "bash", Arguments: map[string]any{"command": "echo 2"}}}},
		{toolCalls: []sdk.ToolCall{{ID: "tc3", Name: "bash", Arguments: map[string]any{"command": "echo 3"}}}},
		{toolCalls: []sdk.ToolCall{{ID: "tc4", Name: "bash", Arguments: map[string]any{"command": "echo 4"}}}},
		{toolCalls: []sdk.ToolCall{{ID: "tc5", Name: "bash", Arguments: map[string]any{"command": "echo 5"}}}},
	})
	registerMockProvider("anthropic", mp)

	// Custom step limit of 5.
	a, err := NewAgentExtension(nil, nil, CompactionConfig{MaxSteps: 5})
	require.NoError(t, err)

	a.providerName = "anthropic"

	b := bus.New()
	defer b.Close()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "configurable limit test"))

	var compactedEvt sdk.Event

	found := false
	deadline := time.After(5 * time.Second)

	for !found {
		select {
		case evt, ok := <-allCh:
			require.True(t, ok, "event channel closed")

			if evt.Topic == TopicCompacted {
				compactedEvt = evt
				found = true
			}

			if evt.Topic == TopicEnd {
				t.Fatal("got TopicEnd before step-limit exceeded event")
			}
		case <-deadline:
			t.Fatal("timeout waiting for step-limit exceeded event")
		}
	}

	payload, ok := compactedEvt.Payload.(map[string]any)
	require.True(t, ok, "compacted payload type = %T", compactedEvt.Payload)
	errMsg, ok := payload["error"].(string)
	require.True(t, ok, "error field type = %T", payload["error"])
	assert.Contains(t, errMsg, "5")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	// Should have executed 5 tool calls before hitting the limit.
	assert.Len(t, mt.ExecuteCalls(), 5)
}

func TestExecuteTool_PanicRecovery(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	panicTool := newMockTool("panic-tool", sdk.ToolDef{Name: "panic-tool", Description: "panics"}, func(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
		panic("intentional test panic")
	})
	registerMockTool(panicTool)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "panic-tool", Arguments: map[string]any{"x": 1}},
			},
		},
		{textDeltas: []string{"recovered"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "trigger panic"))

	toolResultEvt, ok := waitForTopic(allCh, TopicToolResult, 2*time.Second)
	require.True(t, ok, "timeout waiting for tool_result")

	payload := toolResultEvt.Payload.(map[string]any)
	result := payload["result"].(sdk.ToolResult)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "tool panicked:")
	assert.Contains(t, result.Content, "intentional test panic")

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second msg_end")
	msgEndPayload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, "recovered", msgEndPayload["content"])
}

func TestAgent_InvalidThinkingLevelIgnored(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var mu sync.Mutex

	var capturedOpts []model.StreamOption

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()
			capturedOpts = opts
			mu.Unlock()

			ch := make(chan sdk.ProviderEvent, 1)
			ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "ok"}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first msg_end")

	// Try invalid thinking level
	b.Publish(sdk.NewEvent(TopicThinkingChange, map[string]string{"level": "invalid"}))

	b.Publish(sdk.NewEvent(TopicFollowup, "after bad thinking"))

	_, ok = waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second msg_end")

	mu.Lock()
	so := model.NewStreamOptions(capturedOpts...)
	mu.Unlock()
	assert.Equal(t, model.ThinkingMedium, so.ThinkingLevel)

	require.NoError(t, a.Close())
}

func TestAgent_HeadlessUsesPersistedModel(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var capturedOpts []model.StreamOption

	var mu sync.Mutex

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()
			capturedOpts = opts
			mu.Unlock()

			ch := make(chan sdk.ProviderEvent, 1)
			ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "ok"}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	// Simulate headless: config has persisted model, no TUI will publish model.change
	cfg := &mockPrefsConfig{model: "claude-opus-4-7", thinkingLevel: "high"}

	a, err := NewAgentExtension(cfg, cfg, CompactionConfig{})
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// Simulate headless: publish prompt directly without any model.change
	b.Publish(sdk.NewEvent(TopicPrompt, "headless prompt"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	mu.Lock()
	so := model.NewStreamOptions(capturedOpts...)
	mu.Unlock()

	assert.Equal(t, "claude-opus-4-7", so.Model, "headless mode should use persisted model")
	assert.Equal(t, model.ThinkingHigh, so.ThinkingLevel, "headless mode should use persisted thinking level")

	require.NoError(t, a.Close())
}

// --- system prompt tests ---

func TestAgent_SystemPromptContainsDefault(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var capturedReq sdk.ProviderRequest

	var mu sync.Mutex

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()
			capturedReq = req
			mu.Unlock()

			ch := make(chan sdk.ProviderEvent, 1)
			ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "ok"}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	mu.Lock()
	require.NotEmpty(t, capturedReq.SystemPrompt, "system prompt should not be empty")
	assert.Contains(t, capturedReq.SystemPrompt, "You are Weave")
	assert.Contains(t, capturedReq.SystemPrompt, "Current date:")
	mu.Unlock()

	require.NoError(t, a.Close())
}

func TestAgent_SystemPromptWithContextFiles(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	projectDir := t.TempDir()
	weaveDir := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(weaveDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte("Project rules here."), 0o644))

	var capturedReq sdk.ProviderRequest

	var mu sync.Mutex

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()
			capturedReq = req
			mu.Unlock()

			ch := make(chan sdk.ProviderEvent, 1)
			ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "ok"}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	cfg := sdk.FilePathConfig(filepath.Join(weaveDir, "settings.json"))
	a, err := NewAgentExtension(cfg, sdk.NoopPreferenceStore{}, CompactionConfig{})
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	mu.Lock()
	assert.Contains(t, capturedReq.SystemPrompt, "# Project Context")
	assert.Contains(t, capturedReq.SystemPrompt, "Project rules here.")
	mu.Unlock()

	require.NoError(t, a.Close())
}

func TestAgent_SystemPromptWithSkills(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	projectDir := t.TempDir()
	weaveDir := filepath.Join(projectDir, ".weave")
	skillsDir := filepath.Join(weaveDir, "skills", "test-skill")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(skillsDir, "SKILL.md"),
		[]byte("---\nname: test-skill\ndescription: A test skill for testing\n---\n\nSkill body here."),
		0o644,
	))

	var capturedReq sdk.ProviderRequest

	var mu sync.Mutex

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()
			capturedReq = req
			mu.Unlock()

			ch := make(chan sdk.ProviderEvent, 1)
			ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "ok"}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	cfg := sdk.FilePathConfig(filepath.Join(weaveDir, "settings.json"))
	a, err := NewAgentExtension(cfg, sdk.NoopPreferenceStore{}, CompactionConfig{})
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	mu.Lock()
	assert.Contains(t, capturedReq.SystemPrompt, "<available_skills>")
	assert.Contains(t, capturedReq.SystemPrompt, "<name>test-skill</name>")
	assert.Contains(t, capturedReq.SystemPrompt, "<skills_usage>")
	mu.Unlock()

	require.NoError(t, a.Close())
}

func TestAgent_SystemPromptWithSystemMD(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	projectDir := t.TempDir()
	weaveDir := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(weaveDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(weaveDir, "SYSTEM.md"),
		[]byte("Custom system prompt from SYSTEM.md."),
		0o644,
	))

	var capturedReq sdk.ProviderRequest

	var mu sync.Mutex

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()
			capturedReq = req
			mu.Unlock()

			ch := make(chan sdk.ProviderEvent, 1)
			ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "ok"}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	cfg := sdk.FilePathConfig(filepath.Join(weaveDir, "settings.json"))
	a, err := NewAgentExtension(cfg, sdk.NoopPreferenceStore{}, CompactionConfig{})
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	mu.Lock()
	assert.Contains(t, capturedReq.SystemPrompt, "Custom system prompt from SYSTEM.md.")
	assert.NotContains(t, capturedReq.SystemPrompt, "You are Weave")
	mu.Unlock()

	require.NoError(t, a.Close())
}

func TestAgent_SystemPromptRebuiltOnNewConversation(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	projectDir := t.TempDir()
	weaveDir := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(weaveDir, 0o755))

	var capturedReqs []sdk.ProviderRequest

	var mu sync.Mutex

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first"}},
		{textDeltas: []string{"second"}},
	})
	originalStreamFunc := mp.StreamFunc
	mp.StreamFunc = func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (
		<-chan sdk.ProviderEvent, error,
	) {
		mu.Lock()

		capturedReqs = append(capturedReqs, req)
		mu.Unlock()

		return originalStreamFunc(ctx, req, opts...)
	}
	registerMockProvider("anthropic", mp)

	cfg := sdk.FilePathConfig(filepath.Join(weaveDir, "settings.json"))
	a, err := NewAgentExtension(cfg, sdk.NoopPreferenceStore{}, CompactionConfig{})
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// First conversation
	b.Publish(sdk.NewEvent(TopicPrompt, "first"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Add a context file after first conversation
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte("New context."), 0o644))

	// New conversation should rebuild system prompt
	b.Publish(sdk.NewEvent(TopicPrompt, "second"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end")

	require.NoError(t, a.Close())

	mu.Lock()
	require.Len(t, capturedReqs, 2)
	// Second conversation should have the new context
	assert.Contains(t, capturedReqs[1].SystemPrompt, "New context.")
	mu.Unlock()
}

// --- resolve function tests ---

func TestResolveProviderName_EnvVarHighest(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	sdk.RegisterProvider[struct{}, struct{}]("anthropic", func(sdk.Config, struct{}, struct{}) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	cfg := &mockPrefsConfig{provider: "anthropic"}

	result := resolveProviderName("openai", cfg)
	assert.Equal(t, "openai", result, "env var should win over settings")
}

func TestResolveProviderName_SettingsPreference(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	sdk.RegisterProvider[struct{}, struct{}]("anthropic", func(sdk.Config, struct{}, struct{}) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	cfg := &mockPrefsConfig{provider: "openai"}

	result := resolveProviderName("", cfg)
	assert.Equal(t, "openai", result, "settings provider should be used when no env var")
}

func TestResolveProviderName_FirstRegistered(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	sdk.RegisterProvider[struct{}, struct{}]("zai", func(sdk.Config, struct{}, struct{}) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})
	sdk.RegisterProvider[struct{}, struct{}]("openai", func(sdk.Config, struct{}, struct{}) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	cfg := &mockPrefsConfig{provider: ""}

	result := resolveProviderName("", cfg)
	assert.Equal(t, "openai", result, "should pick first registered provider (alphabetically) when no env or settings")
}

func TestResolveProviderName_AnthropicFallback(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	cfg := &mockPrefsConfig{provider: ""}

	result := resolveProviderName("", cfg)
	assert.Equal(t, "anthropic", result, "should fall back to anthropic when nothing else available")
}

func TestResolveProviderName_PrefsErrorFallsThrough(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	sdk.RegisterProvider[struct{}, struct{}]("openai", func(sdk.Config, struct{}, struct{}) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	cfg := &mockPrefsConfig{prefsErr: assert.AnError}

	result := resolveProviderName("", cfg)
	assert.Equal(t, "openai", result, "should fall through to registered providers when prefs error")
}

func TestResolveProviderName_EnvOverridesSettings(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	cfg := &mockPrefsConfig{provider: "openai"}

	result := resolveProviderName("zai", cfg)
	assert.Equal(t, "zai", result, "env var should override settings preference")
}

func TestResolveModelName_FromSettings(t *testing.T) {
	cfg := &mockPrefsConfig{model: "claude-opus-4-7"}

	result := resolveModelName(cfg)
	assert.Equal(t, "claude-opus-4-7", result)
}

func TestResolveModelName_EmptyWhenUnset(t *testing.T) {
	cfg := &mockPrefsConfig{}

	result := resolveModelName(cfg)
	assert.Empty(t, result)
}

func TestResolveModelName_NilConfig(t *testing.T) {
	result := resolveModelName(nil)
	assert.Empty(t, result)
}

func TestResolveModelName_PrefsError(t *testing.T) {
	cfg := &mockPrefsConfig{model: "claude-opus-4-7", prefsErr: assert.AnError}

	result := resolveModelName(cfg)
	assert.Empty(t, result)
}

func TestResolveThinkingLevel_FromSettings(t *testing.T) {
	cfg := &mockPrefsConfig{thinkingLevel: "high"}

	result := resolveThinkingLevel(cfg)
	assert.Equal(t, model.ThinkingHigh, result)
}

func TestResolveThinkingLevel_FallsBackToEnvVar(t *testing.T) {
	t.Setenv("WEAVE_THINKING_LEVEL", "low")

	cfg := &mockPrefsConfig{}

	result := resolveThinkingLevel(cfg)
	assert.Equal(t, model.ThinkingLow, result)
}

func TestResolveThinkingLevel_SettingsOverEnvVar(t *testing.T) {
	t.Setenv("WEAVE_THINKING_LEVEL", "low")

	cfg := &mockPrefsConfig{thinkingLevel: "high"}

	result := resolveThinkingLevel(cfg)
	assert.Equal(t, model.ThinkingHigh, result, "settings should take priority over env var")
}

func TestResolveThinkingLevel_DefaultMedium(t *testing.T) {
	result := resolveThinkingLevel(nil)
	assert.Equal(t, model.ThinkingMedium, result)
}

func TestResolveThinkingLevel_InvalidFallsBack(t *testing.T) {
	cfg := &mockPrefsConfig{thinkingLevel: "garbage"}

	result := resolveThinkingLevel(cfg)
	assert.Equal(t, model.ThinkingMedium, result, "invalid thinking level should fall back to DefaultThinkingLevel")
}

func TestNewAgentExtension_ReadsModelFromSettings(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := &ProviderMock{}
	registerMockProvider("anthropic", mp)

	cfg := &mockPrefsConfig{model: "claude-opus-4-7", thinkingLevel: "high"}

	a, err := NewAgentExtension(cfg, cfg, CompactionConfig{})
	require.NoError(t, err)

	assert.Equal(t, "claude-opus-4-7", a.modelName)
	assert.Equal(t, model.ThinkingHigh, a.thinkingLevel)
}

func TestNewAgentExtension_ClearsModelWhenProviderMismatch(t *testing.T) {
	resetRegistries()
	defer resetRegistries()
	defer model.ResetModelRegistry()

	registerMockProvider("anthropic", &ProviderMock{})

	// Register gpt-5.5 as an OpenAI model
	model.RegisterModel(model.ModelDef{
		ID:       "gpt-5.5",
		Provider: "openai",
	})

	// Settings have provider=openai, model=gpt-5.5, but WEAVE_PROVIDER=anthropic wins
	t.Setenv("WEAVE_PROVIDER", "anthropic")

	cfg := &mockPrefsConfig{provider: "openai", model: "gpt-5.5"}

	a, err := NewAgentExtension(cfg, cfg, CompactionConfig{})
	require.NoError(t, err)

	assert.Empty(t, a.modelName, "model should be cleared when it belongs to a different provider")
}

func TestNewAgentExtension_KeepsModelWhenSameProvider(t *testing.T) {
	resetRegistries()
	defer resetRegistries()
	defer model.ResetModelRegistry()

	registerMockProvider("anthropic", &ProviderMock{})

	model.RegisterModel(model.ModelDef{
		ID:       "claude-opus-4-7",
		Provider: "anthropic",
	})

	cfg := &mockPrefsConfig{model: "claude-opus-4-7"}

	a, err := NewAgentExtension(cfg, cfg, CompactionConfig{})
	require.NoError(t, err)

	assert.Equal(t, "claude-opus-4-7", a.modelName, "model should be kept when provider matches")
}

func TestNewAgentExtension_KeepsUnregisteredModel(t *testing.T) {
	resetRegistries()
	defer resetRegistries()
	defer model.ResetModelRegistry()

	registerMockProvider("anthropic", &ProviderMock{})

	// Model not in registry — user might be using a custom model name
	cfg := &mockPrefsConfig{model: "my-custom-model"}

	a, err := NewAgentExtension(cfg, cfg, CompactionConfig{})
	require.NoError(t, err)

	assert.Equal(t, "my-custom-model", a.modelName, "unregistered model should be kept (custom model)")
}

// --- session resume tests ---

func TestAgent_SessionResume_RestoresMessages(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"response after resume"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// Publish session.resume before prompt
	b.Publish(sdk.NewEvent(TopicSessionResume, sdk.SessionResumePayload{
		SessionID: "sess-123",
		Messages: []sdk.Message{
			{Role: sdk.RoleUser, Content: "previous message"},
			{Role: sdk.RoleAssistant, Content: "previous response"},
		},
	}))

	// Give the bus handler time to route the event before publishing prompt
	time.Sleep(50 * time.Millisecond)

	// Now send the prompt
	b.Publish(sdk.NewEvent(TopicPrompt, "new prompt"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 1)
	// Should have 3 messages: 2 restored + 1 new prompt
	assert.Len(t, calls[0].Req.Messages, 3, "expected restored messages + new prompt")
	assert.Equal(t, "previous message", calls[0].Req.Messages[0].Content)
	assert.Equal(t, "previous response", calls[0].Req.Messages[1].Content)
	assert.Equal(t, "new prompt", calls[0].Req.Messages[2].Content)
}

func TestAgent_SessionResume_ClearsFlagAfterPrompt(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first"}},
		{textDeltas: []string{"second"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// Publish session.resume
	b.Publish(sdk.NewEvent(TopicSessionResume, sdk.SessionResumePayload{
		SessionID: "sess-123",
		Messages: []sdk.Message{
			{Role: sdk.RoleUser, Content: "restored"},
		},
	}))

	time.Sleep(50 * time.Millisecond)

	// First prompt after resume should append
	b.Publish(sdk.NewEvent(TopicPrompt, "first prompt"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Second prompt should reset (resumed flag cleared)
	b.Publish(sdk.NewEvent(TopicPrompt, "second prompt"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 2)
	// First call: 2 messages (restored + first prompt)
	assert.Len(t, calls[0].Req.Messages, 2)
	// Second call: 1 message (reset, only second prompt)
	assert.Len(t, calls[1].Req.Messages, 1)
	assert.Equal(t, "second prompt", calls[1].Req.Messages[0].Content)
}

func TestAgent_SessionResume_SetsSessionID(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"ok"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicSessionResume, sdk.SessionResumePayload{
		SessionID: "my-session-id",
		Messages: []sdk.Message{
			{Role: sdk.RoleUser, Content: "hello"},
		},
	}))

	time.Sleep(50 * time.Millisecond)

	b.Publish(sdk.NewEvent(TopicPrompt, "prompt"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	assert.Equal(t, "my-session-id", a.sessionID)
	assert.False(t, a.resumed, "resumed flag should be cleared after prompt is processed")
}

func TestAgent_SessionResume_WithToolResults(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("bash", sdk.ToolDef{Name: "bash", Description: "run commands"}, nil)
	registerMockTool(mt)

	mp := newMockProvider([]providerResponse{
		{toolCalls: []sdk.ToolCall{{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "echo hi"}}}},
		{textDeltas: []string{"done"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// Resume with a tool result message in history
	b.Publish(sdk.NewEvent(TopicSessionResume, sdk.SessionResumePayload{
		SessionID: "sess-tools",
		Messages: []sdk.Message{
			{Role: sdk.RoleUser, Content: "run echo"},
			{Role: sdk.RoleAssistant, Content: "", ToolCalls: []sdk.ToolCall{{ID: "tc0", Name: "bash", Arguments: map[string]any{"command": "echo old"}}}},
			{Role: sdk.RoleToolResult, ToolCallID: "tc0", Content: "old output"},
		},
	}))

	time.Sleep(50 * time.Millisecond)

	b.Publish(sdk.NewEvent(TopicPrompt, "run echo again"))

	// Wait for both turns to complete
	for i := range 2 {
		_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
		require.True(t, ok, "timeout waiting for turn_end %d", i+1)
	}

	require.NoError(t, a.Close())

	_, ok := waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 2)
	// First call after resume should include restored messages + new prompt
	assert.Len(t, calls[0].Req.Messages, 4, "expected 3 restored + 1 new prompt")
	assert.Equal(t, "run echo", calls[0].Req.Messages[0].Content)
	assert.Equal(t, "run echo again", calls[0].Req.Messages[3].Content)
}

func TestAgent_SessionResume_EmptyMessages(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"hello"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// Resume with empty messages
	b.Publish(sdk.NewEvent(TopicSessionResume, sdk.SessionResumePayload{
		SessionID: "empty-sess",
		Messages:  nil,
	}))

	time.Sleep(50 * time.Millisecond)

	b.Publish(sdk.NewEvent(TopicPrompt, "first"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 1)
	// Should still append since resumed flag is set
	assert.Len(t, calls[0].Req.Messages, 1)
	assert.Equal(t, "first", calls[0].Req.Messages[0].Content)
}

func TestAgent_SessionResume_AfterPromptHandled(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first"}},
		{textDeltas: []string{"second"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// Send prompt first (no resume)
	b.Publish(sdk.NewEvent(TopicPrompt, "original"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Now send session.resume (after prompt already processed)
	b.Publish(sdk.NewEvent(TopicSessionResume, sdk.SessionResumePayload{
		SessionID: "late-sess",
		Messages: []sdk.Message{
			{Role: sdk.RoleUser, Content: "restored"},
		},
	}))

	// Send another prompt - should append to restored messages since session.resume
	// is now handled in waitForInput.
	b.Publish(sdk.NewEvent(TopicPrompt, "new conversation"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 2)
	// First call: 1 message (original prompt)
	assert.Len(t, calls[0].Req.Messages, 1)
	// Second call: restored message + new prompt appended
	assert.Len(t, calls[1].Req.Messages, 2)
	assert.Equal(t, "restored", calls[1].Req.Messages[0].Content)
	assert.Equal(t, "new conversation", calls[1].Req.Messages[1].Content)
}

func TestAgent_SessionResume_InvalidPayloadIgnored(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"ok"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// Publish session.resume with an invalid payload (not SessionResumePayload)
	b.Publish(sdk.NewEvent(TopicSessionResume, "not-a-payload"))

	time.Sleep(50 * time.Millisecond)

	b.Publish(sdk.NewEvent(TopicPrompt, "prompt"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	// Messages should be empty (resume failed), prompt starts fresh
	calls := mp.StreamCalls()
	require.Len(t, calls, 1)
	assert.Len(t, calls[0].Req.Messages, 1)
	assert.Equal(t, "prompt", calls[0].Req.Messages[0].Content)
	assert.False(t, a.resumed)
	assert.Empty(t, a.sessionID)
}

func TestExecuteTool_InvalidArgs(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("test-tool", sdk.ToolDef{
		Name:        "test-tool",
		Description: "a test tool",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "number"},
			},
			"additionalProperties": false,
		},
	}, nil)
	registerMockTool(mt)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "test-tool", Arguments: map[string]any{"count": "not-a-number"}},
			},
		},
		{textDeltas: []string{"recovered"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "trigger invalid args"))

	toolResultEvt, ok := waitForTopic(allCh, TopicToolResult, 2*time.Second)
	require.True(t, ok, "timeout waiting for tool_result")

	payload := toolResultEvt.Payload.(map[string]any)
	result := payload["result"].(sdk.ToolResult)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "invalid arguments:")
	assert.Contains(t, result.Content, "expected type \"number\", got \"string\"")

	// Tool Execute should NOT have been called
	assert.Empty(t, mt.ExecuteCalls())
}

func TestExecuteTool_MissingRequired(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("test-tool", sdk.ToolDef{
		Name:        "test-tool",
		Description: "a test tool",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required":             []any{"name"},
			"additionalProperties": false,
		},
	}, nil)
	registerMockTool(mt)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "test-tool", Arguments: map[string]any{}},
			},
		},
		{textDeltas: []string{"recovered"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "trigger missing required"))

	toolResultEvt, ok := waitForTopic(allCh, TopicToolResult, 2*time.Second)
	require.True(t, ok, "timeout waiting for tool_result")

	payload := toolResultEvt.Payload.(map[string]any)
	result := payload["result"].(sdk.ToolResult)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "invalid arguments:")
	assert.Contains(t, result.Content, `missing required field: "name"`)

	// Tool Execute should NOT have been called
	assert.Empty(t, mt.ExecuteCalls())
}

func TestExecuteTool_UnknownField(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("test-tool", sdk.ToolDef{
		Name:        "test-tool",
		Description: "a test tool",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"additionalProperties": false,
		},
	}, nil)
	registerMockTool(mt)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "test-tool", Arguments: map[string]any{"name": "alice", "extra": "value"}},
			},
		},
		{textDeltas: []string{"recovered"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "trigger unknown field"))

	toolResultEvt, ok := waitForTopic(allCh, TopicToolResult, 2*time.Second)
	require.True(t, ok, "timeout waiting for tool_result")

	payload := toolResultEvt.Payload.(map[string]any)
	result := payload["result"].(sdk.ToolResult)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "invalid arguments:")
	assert.Contains(t, result.Content, `unknown field: "extra"`)

	// Tool Execute should NOT have been called
	assert.Empty(t, mt.ExecuteCalls())
}

type toolExecRecord struct {
	name  string
	start time.Time
	end   time.Time
}

func newRecordingTool(name string, sleep time.Duration, log *[]toolExecRecord, mu *sync.Mutex) *ToolMock {
	return newMockTool(name, sdk.ToolDef{Name: name}, func(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
		start := time.Now()

		time.Sleep(sleep)

		end := time.Now()

		mu.Lock()

		*log = append(*log, toolExecRecord{name: name, start: start, end: end})
		mu.Unlock()

		return sdk.ToolResult{Content: name + " done"}, nil
	})
}

func TestExecuteTools_ParallelReads(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var (
		mu      sync.Mutex
		records []toolExecRecord
	)

	read1 := newRecordingTool("read", 100*time.Millisecond, &records, &mu)
	read2 := newRecordingTool("grep", 100*time.Millisecond, &records, &mu)

	registerMockTool(read1)
	registerMockTool(read2)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "read", Arguments: map[string]any{"path": "a.go"}},
				{ID: "tc2", Name: "grep", Arguments: map[string]any{"pattern": "foo"}},
			},
		},
		{textDeltas: []string{"done"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	start := time.Now()

	b.Publish(sdk.NewEvent(TopicPrompt, "parallel reads"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 5*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	duration := time.Since(start)

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	mu.Lock()
	require.Len(t, records, 2)
	r1, r2 := records[0], records[1]
	mu.Unlock()

	// Both read tools should have started close together (within 30ms).
	startDelta := r1.start.Sub(r2.start)
	if startDelta < 0 {
		startDelta = -startDelta
	}

	assert.Less(t, startDelta, 30*time.Millisecond, "read tools should start concurrently")

	// Total duration should be less than 180ms (parallel, not sequential).
	assert.Less(t, duration, 180*time.Millisecond, "parallel reads should finish faster than sequential")

	// Messages should be in original order.
	calls := mp.StreamCalls()
	require.Len(t, calls, 2)
	require.Len(t, calls[1].Req.Messages, 4) // user prompt + assistant with tool_calls + 2 tool results
	assert.Equal(t, sdk.RoleToolResult, calls[1].Req.Messages[2].Role)
	assert.Equal(t, "tc1", calls[1].Req.Messages[2].ToolCallID)
	assert.Equal(t, sdk.RoleToolResult, calls[1].Req.Messages[3].Role)
	assert.Equal(t, "tc2", calls[1].Req.Messages[3].ToolCallID)
}

func TestExecuteTools_WritesSequential(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var (
		mu      sync.Mutex
		records []toolExecRecord
	)

	editTool := newRecordingTool("edit", 80*time.Millisecond, &records, &mu)
	writeTool := newRecordingTool("write", 80*time.Millisecond, &records, &mu)

	registerMockTool(editTool)
	registerMockTool(writeTool)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "edit", Arguments: map[string]any{"path": "a.go", "old_string": "x", "new_string": "y"}},
				{ID: "tc2", Name: "write", Arguments: map[string]any{"path": "b.go", "content": "hello"}},
			},
		},
		{textDeltas: []string{"done"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	start := time.Now()

	b.Publish(sdk.NewEvent(TopicPrompt, "sequential writes"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 5*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	duration := time.Since(start)

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	mu.Lock()
	require.Len(t, records, 2)
	r1, r2 := records[0], records[1]
	mu.Unlock()

	// Second write should start after first ends (sequential).
	assert.True(t, r2.start.Equal(r1.end) || r2.start.After(r1.end),
		"second write should start after first ends: r1.end=%v r2.start=%v", r1.end, r2.start)

	// Total duration should be at least 140ms (sequential, not parallel).
	assert.GreaterOrEqual(t, duration, 140*time.Millisecond, "sequential writes should take longer than a single write")
}

func TestExecuteTools_MixedParallelAndSequential(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var (
		mu      sync.Mutex
		records []toolExecRecord
	)

	read1 := newRecordingTool("read", 80*time.Millisecond, &records, &mu)
	editTool := newRecordingTool("edit", 50*time.Millisecond, &records, &mu)
	read2 := newRecordingTool("find", 80*time.Millisecond, &records, &mu)

	registerMockTool(read1)
	registerMockTool(editTool)
	registerMockTool(read2)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "read", Arguments: map[string]any{"path": "a.go"}},
				{ID: "tc2", Name: "edit", Arguments: map[string]any{"path": "a.go", "old_string": "x", "new_string": "y"}},
				{ID: "tc3", Name: "find", Arguments: map[string]any{"pattern": "*.go"}},
			},
		},
		{textDeltas: []string{"done"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	start := time.Now()

	b.Publish(sdk.NewEvent(TopicPrompt, "mixed parallel and sequential"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 5*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	duration := time.Since(start)

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	mu.Lock()
	require.Len(t, records, 3)

	// Find the record for each tool by name.
	var rRead, rEdit, rFind toolExecRecord

	for _, r := range records {
		switch r.name {
		case "read":
			rRead = r
		case "edit":
			rEdit = r
		case "find":
			rFind = r
		}
	}
	mu.Unlock()

	// Both reads should have started close together (concurrent).
	readStartDelta := rRead.start.Sub(rFind.start)
	if readStartDelta < 0 {
		readStartDelta = -readStartDelta
	}

	assert.Less(t, readStartDelta, 30*time.Millisecond, "read and find should start concurrently")

	// Edit should start after both reads complete.
	latestReadEnd := rRead.end
	if rFind.end.After(latestReadEnd) {
		latestReadEnd = rFind.end
	}

	assert.True(t, rEdit.start.Equal(latestReadEnd) || rEdit.start.After(latestReadEnd),
		"edit should start after both reads complete")

	// Total should be less than 250ms (reads parallel ~80ms + edit ~50ms = ~130ms,
	// not sequential ~210ms).
	assert.Less(t, duration, 250*time.Millisecond, "mixed execution should be faster than fully sequential")

	// Verify messages are in original tool call order in the next stream request.
	calls := mp.StreamCalls()
	require.Len(t, calls, 2)
	require.Len(t, calls[1].Req.Messages, 5) // user prompt + assistant with tool_calls + 3 tool results
	assert.Equal(t, "tc1", calls[1].Req.Messages[2].ToolCallID)
	assert.Equal(t, "tc2", calls[1].Req.Messages[3].ToolCallID)
	assert.Equal(t, "tc3", calls[1].Req.Messages[4].ToolCallID)
}

func TestExecuteTools_ContextCancelDuringParallelReads(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	started := make(chan struct{}, 2)

	read1 := newMockTool("read", sdk.ToolDef{Name: "read"}, func(ctx context.Context, _ map[string]any) (sdk.ToolResult, error) {
		started <- struct{}{}

		select {
		case <-ctx.Done():
			return sdk.ToolResult{}, ctx.Err()
		case <-time.After(5 * time.Second):
			return sdk.ToolResult{Content: "read done"}, nil
		}
	})
	read2 := newMockTool("grep", sdk.ToolDef{Name: "grep"}, func(ctx context.Context, _ map[string]any) (sdk.ToolResult, error) {
		started <- struct{}{}

		select {
		case <-ctx.Done():
			return sdk.ToolResult{}, ctx.Err()
		case <-time.After(5 * time.Second):
			return sdk.ToolResult{Content: "grep done"}, nil
		}
	})

	registerMockTool(read1)
	registerMockTool(read2)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "read", Arguments: map[string]any{"path": "a.go"}},
				{ID: "tc2", Name: "grep", Arguments: map[string]any{"pattern": "foo"}},
			},
		},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "parallel reads"))

	// Wait for both read tools to start executing.
	<-started
	<-started

	// Cancel the turn while tools are still running.
	b.Publish(sdk.NewEvent(TopicInterrupt, "user interrupt"))

	// The turn should end cleanly without panics or data races.
	_, ok := waitForTopic(allCh, TopicTurnEnd, 3*time.Second)
	require.True(t, ok, "timeout waiting for turn_end after interrupt during parallel reads")

	require.NoError(t, a.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")
}
