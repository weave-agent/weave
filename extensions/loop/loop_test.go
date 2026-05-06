package agentloop

import (
	"context"
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
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

func resetRegistries() {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()
	sdk.ResetToolRegistry()
}

func setupLoop(t *testing.T, providerName string) (*Loop, *bus.Bus, func()) {
	t.Helper()

	l, err := NewLoop(nil, providerName)
	require.NoError(t, err, "NewLoop")

	b := bus.New()

	return l, b, func() {
		_ = b.Close()
	}
}

func registerMockProvider(_ string, mp *ProviderMock) {
	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return mp, nil
	})
}

func registerMockTool(mt *ToolMock) {
	sdk.RegisterTool(mt.NameFunc(), func(sdk.Config) (sdk.Tool, error) {
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

// --- tests ---

func TestLoop_StartupAndShutdown(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := &ProviderMock{}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	require.NoError(t, l.Subscribe(b))

	require.NoError(t, l.Close())
}

func TestLoop_SingleTurn_NoTools(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"hello", " world"}},
	})
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

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

	require.NoError(t, l.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 1)
	assert.Len(t, calls[0].Req.Messages, 1)
}

func TestLoop_ToolCallCycle(t *testing.T) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "run echo"))

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

func TestLoop_SteeringInjection(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first"}},
		{textDeltas: []string{"steered"}},
	})
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))
	b.Publish(sdk.NewEvent(TopicSteer, "steer this"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	require.NoError(t, l.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 1)
	assert.Len(t, calls[0].Req.Messages, 2, "expected 2 messages (prompt + steering)")
}

func TestLoop_SteeringDuringTurn(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	streamingStarted := make(chan struct{})

	responses := []providerResponse{
		{textDeltas: []string{"first"}},
		{textDeltas: []string{"steered"}},
	}

	mp := newMockProvider(responses)

	originalStreamFunc := mp.StreamFunc
	mp.StreamFunc = func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
		callIdx := len(mp.StreamCalls()) - 1
		if callIdx == 0 {
			close(streamingStarted)
			time.Sleep(100 * time.Millisecond)
		}

		return originalStreamFunc(ctx, req)
	}

	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	<-streamingStarted
	b.Publish(sdk.NewEvent(TopicSteer, "steer during turn"))

	for i := range 2 {
		_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
		require.True(t, ok, "timeout waiting for turn_end %d", i+1)
	}

	require.NoError(t, l.Close())

	_, ok := waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	assert.Len(t, mp.StreamCalls(), 2)
}

func TestLoop_FollowupReentry(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first response"}},
		{textDeltas: []string{"follow-up response"}},
	})
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	// Send initial prompt
	b.Publish(sdk.NewEvent(TopicPrompt, "initial"))

	// Wait for the first turn to complete (real-world timing: user reads response)
	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Send follow-up after the first turn completes (not before)
	b.Publish(sdk.NewEvent(TopicFollowup, "follow up question"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end")

	require.NoError(t, l.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	assert.Len(t, mp.StreamCalls(), 2)
}

func TestLoop_PromptResetsConversation(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first response"}},
		{textDeltas: []string{"new conversation"}},
	})
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	// First conversation
	b.Publish(sdk.NewEvent(TopicPrompt, "first prompt"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Simulate /new: send a second agent.prompt to reset the conversation
	b.Publish(sdk.NewEvent(TopicPrompt, "new prompt after /new"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end")

	require.NoError(t, l.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	calls := mp.StreamCalls()
	require.Len(t, calls, 2)
	// First call: 1 message (just the first prompt)
	assert.Len(t, calls[0].Req.Messages, 1)
	// Second call: 1 message (conversation was reset, only the new prompt)
	assert.Len(t, calls[1].Req.Messages, 1)
}

func TestLoop_ErrorAbort(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{err: context.DeadlineExceeded},
	})
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "trigger error"))

	_, ok := waitForTopic(allCh, TopicTurnStart, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_start")

	endEvt, ok := waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")
	assert.NotNil(t, endEvt.Payload)
}

func TestLoop_ProviderErrorOnStartup(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	l, b, cleanup := setupLoop(t, "nonexistent")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	endEvts := collectTopic(allCh, TopicEnd, 2*time.Second)
	require.Len(t, endEvts, 1)
	assert.NotNil(t, endEvts[0].Payload)
}

func TestLoop_ContextCancellation(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	blockCh := make(chan sdk.ProviderEvent)

	registerMockProvider("anthropic", &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	done := make(chan error, 1)

	go func() { done <- l.Close() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Close hung — context cancellation did not unblock the loop")
	}

	_, ok := waitForTopic(allCh, TopicEnd, time.Second)
	assert.True(t, ok, "expected TopicEnd after cancellation")
}

func TestLoop_MsgUpdateEvents(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"a", "b", "c"}},
	})
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "stream test"))

	updates := collectTopic(allCh, TopicMsgUpdate, 2*time.Second)
	require.Len(t, updates, 3)

	expected := []string{"a", "b", "c"}
	for i, u := range updates {
		assert.Equal(t, expected[i], u.Payload)
	}
}

func TestLoop_MissingToolError(t *testing.T) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

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

func TestLoop_RegisterAsExtension(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	sdk.RegisterExtension("loop", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewLoop(cfg, "anthropic")
	})

	ext, err := sdk.GetExtension("loop", nil)
	require.NoError(t, err, "GetExtension(loop)")
	assert.Equal(t, "loop", ext.Name())

	_, ok := ext.(*Loop)
	require.True(t, ok, "expected *Loop, got %T", ext)
}

func TestLoop_MultipleToolCalls(t *testing.T) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

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
			case TopicTurnEnd:
				// First turn done; close to trigger TopicEnd
				require.NoError(t, l.Close())
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

func TestLoop_StreamingUpdatesPreserveOrder(t *testing.T) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "order test"))

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 3*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	expected := strings.Join(deltas, "")
	msgEndPayload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, expected, msgEndPayload["content"])
}

func TestLoop_InterruptHaltsTurn(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	started := make(chan struct{})

	responses := []providerResponse{
		{textDeltas: []string{"partial"}},
		{textDeltas: []string{"after interrupt"}},
	}

	mp := newMockProvider(responses)
	originalStreamFunc := mp.StreamFunc
	mp.StreamFunc = func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
		callIdx := len(mp.StreamCalls()) - 1
		if callIdx == 0 {
			close(started)
			// Give the interrupt time to arrive and cancel the context
			time.Sleep(200 * time.Millisecond)
		}

		return originalStreamFunc(ctx, req)
	}

	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

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

	require.NoError(t, l.Close())

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	// Two stream calls: first (interrupted) and second (follow-up)
	assert.Len(t, mp.StreamCalls(), 2)
}

func TestLoop_ThinkingContentInMsgEnd(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{
			thinkingDeltas: []string{"let me think", "... carefully"},
			textDeltas:     []string{"here is the answer"},
		},
	})
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "think about it"))

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	payload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, "here is the answer", payload["content"])
	assert.Equal(t, "let me think... carefully", payload["thinking"])

	require.NoError(t, l.Close())
}

func TestLoop_NoThinkingKeyWhenEmpty(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"no thinking here"}},
	})
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "quick one"))

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	payload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, "no thinking here", payload["content"])
	_, hasThinking := payload["thinking"]
	assert.False(t, hasThinking, "should not have thinking key when no thinking deltas")

	require.NoError(t, l.Close())
}

func TestLoop_ThinkingLevelChange(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var mu sync.Mutex

	var capturedOpts []model.StreamOption

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

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

	require.NoError(t, l.Close())
}

func TestLoop_ModelChangeWithModelKey(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var capturedOpts []model.StreamOption

	var mu sync.Mutex

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

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

	require.NoError(t, l.Close())
}

func TestLoop_ModelChangeDifferentProvider(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	anthropicMock := newMockProvider([]providerResponse{
		{textDeltas: []string{"anthropic response"}},
	})
	openaiMock := newMockProvider([]providerResponse{
		{textDeltas: []string{"openai response"}},
	})

	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return anthropicMock, nil
	})
	sdk.RegisterProvider("openai", func(sdk.Config) (sdk.Provider, error) {
		return openaiMock, nil
	})

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

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

	require.NoError(t, l.Close())

	// Verify openai was called and model was passed
	require.Len(t, openaiMock.StreamCalls(), 1)
	openaiOpts := openaiMock.StreamCalls()[0].Opts
	so := model.NewStreamOptions(openaiOpts...)
	assert.Equal(t, "gpt-5.5", so.Model)
}

func TestLoop_InvalidThinkingLevelIgnored(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var mu sync.Mutex

	var capturedOpts []model.StreamOption

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

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

	require.NoError(t, l.Close())
}

func TestLoop_SystemPromptEmpty(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var capturedReq sdk.ProviderRequest

	var mu sync.Mutex

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	mu.Lock()
	assert.Empty(t, capturedReq.SystemPrompt, "system prompt should be empty when no skills loaded")
	mu.Unlock()

	require.NoError(t, l.Close())
}

func TestLoop_SystemPromptWithSkills(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var capturedReq sdk.ProviderRequest

	var mu sync.Mutex

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	skillsXML := "<available_skills>\n<skill>\n<name>test-skill</name>\n<description>A test skill</description>\n<location>/path/to/test-skill/SKILL.md</location>\n</skill>\n</available_skills>"
	b.Publish(sdk.NewEvent(sdk.TopicSkillsLoaded, skillsXML))

	// Give the goroutine time to process the skills event before sending prompt
	time.Sleep(50 * time.Millisecond)
	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	mu.Lock()
	assert.Equal(t, skillsXML, capturedReq.SystemPrompt, "system prompt should contain skills XML")
	mu.Unlock()

	require.NoError(t, l.Close())
}

func TestLoop_SkillsUpdateViaBus(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var capturedReqs []sdk.ProviderRequest

	var mu sync.Mutex

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first"}},
		{textDeltas: []string{"second"}},
	})
	originalStreamFunc := mp.StreamFunc
	mp.StreamFunc = func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
		mu.Lock()

		capturedReqs = append(capturedReqs, req)
		mu.Unlock()

		return originalStreamFunc(ctx, req, opts...)
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	// First turn with initial skills
	skillsV1 := "<available_skills><skill><name>v1</name></skill></available_skills>"
	b.Publish(sdk.NewEvent(sdk.TopicSkillsLoaded, skillsV1))

	// Give the goroutine time to process the skills event before sending prompt
	time.Sleep(50 * time.Millisecond)
	b.Publish(sdk.NewEvent(TopicPrompt, "first turn"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	mu.Lock()
	require.Len(t, capturedReqs, 1)
	assert.Equal(t, skillsV1, capturedReqs[0].SystemPrompt)
	mu.Unlock()

	// Update skills before second turn
	skillsV2 := "<available_skills><skill><name>v2</name></skill></available_skills>"
	b.Publish(sdk.NewEvent(sdk.TopicSkillsLoaded, skillsV2))

	b.Publish(sdk.NewEvent(TopicFollowup, "second turn"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end")

	mu.Lock()
	require.Len(t, capturedReqs, 2)
	assert.Equal(t, skillsV2, capturedReqs[1].SystemPrompt, "system prompt should reflect updated skills")
	mu.Unlock()

	require.NoError(t, l.Close())
}

func TestLoop_InstructionsOnlySystemPrompt(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var (
		capturedReq sdk.ProviderRequest
		mu          sync.Mutex
	)

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	instructions := "# Project Context\n\nSome project instructions"
	b.Publish(sdk.NewEvent(sdk.TopicInstructionsLoaded, instructions))

	time.Sleep(50 * time.Millisecond)
	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	mu.Lock()
	assert.Equal(t, instructions, capturedReq.SystemPrompt, "system prompt should contain instructions only")
	mu.Unlock()

	require.NoError(t, l.Close())
}

func TestLoop_InstructionsCombinedWithSkills(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var (
		capturedReq sdk.ProviderRequest
		mu          sync.Mutex
	)

	mp := &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	instructions := "# Project Context\n\nSome instructions"
	skillsXML := "<available_skills><skill><name>test</name></skill></available_skills>"

	b.Publish(sdk.NewEvent(sdk.TopicInstructionsLoaded, instructions))
	b.Publish(sdk.NewEvent(sdk.TopicSkillsLoaded, skillsXML))

	time.Sleep(50 * time.Millisecond)
	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	_, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	expected := instructions + "\n\n" + skillsXML

	mu.Lock()
	assert.Equal(t, expected, capturedReq.SystemPrompt, "system prompt should combine instructions + skills")
	mu.Unlock()

	require.NoError(t, l.Close())
}

func TestLoop_InstructionsUpdateViaBus(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var (
		capturedReqs []sdk.ProviderRequest
		mu           sync.Mutex
	)

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"first"}},
		{textDeltas: []string{"second"}},
	})
	originalStreamFunc := mp.StreamFunc
	mp.StreamFunc = func(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
		mu.Lock()

		capturedReqs = append(capturedReqs, req)
		mu.Unlock()

		return originalStreamFunc(ctx, req, opts...)
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, l.Subscribe(b))

	instrV1 := "# Context v1"
	b.Publish(sdk.NewEvent(sdk.TopicInstructionsLoaded, instrV1))

	time.Sleep(50 * time.Millisecond)
	b.Publish(sdk.NewEvent(TopicPrompt, "first turn"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	mu.Lock()
	require.Len(t, capturedReqs, 1)
	assert.Equal(t, instrV1, capturedReqs[0].SystemPrompt)
	mu.Unlock()

	instrV2 := "# Context v2"
	b.Publish(sdk.NewEvent(sdk.TopicInstructionsLoaded, instrV2))

	b.Publish(sdk.NewEvent(TopicFollowup, "second turn"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second turn_end")

	mu.Lock()
	require.Len(t, capturedReqs, 2)
	assert.Equal(t, instrV2, capturedReqs[1].SystemPrompt, "system prompt should reflect updated instructions")
	mu.Unlock()

	require.NoError(t, l.Close())
}
