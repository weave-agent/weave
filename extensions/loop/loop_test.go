package agentloop

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"weave/bus"
	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

type providerResponse struct {
	textDeltas []string
	toolCalls  []sdk.ToolCall
	err        error
}

func newMockProvider(responses []providerResponse) *ProviderMock {
	var mu sync.Mutex

	callCount := 0

	return &ProviderMock{
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
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

			ch := make(chan sdk.ProviderEvent, len(resp.textDeltas)+len(resp.toolCalls)+1)

			go func() {
				defer close(ch)

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

	l.Subscribe(b)

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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))
	b.Publish(sdk.NewEvent(TopicSteer, "steer this"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

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
	mp.StreamFunc = func(ctx context.Context, req sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	<-streamingStarted
	b.Publish(sdk.NewEvent(TopicSteer, "steer during turn"))

	for i := range 2 {
		_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
		require.True(t, ok, "timeout waiting for turn_end %d", i+1)
	}

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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "initial"))
	b.Publish(sdk.NewEvent(TopicFollowup, "follow up question"))

	for i := range 2 {
		_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
		require.True(t, ok, "timeout waiting for turn_end %d", i+1)
	}

	_, ok := waitForTopic(allCh, TopicEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for end")

	assert.Len(t, mp.StreamCalls(), 2)
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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

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
		StreamFunc: func(ctx context.Context, req sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

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

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "order test"))

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 3*time.Second)
	require.True(t, ok, "timeout waiting for msg_end")

	expected := strings.Join(deltas, "")
	msgEndPayload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok, "msg_end payload type = %T", msgEnd.Payload)
	assert.Equal(t, expected, msgEndPayload["content"])
}
