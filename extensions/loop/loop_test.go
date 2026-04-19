package agentloop

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"weave/bus"
	"weave/sdk"
)

// --- mock provider ---

type mockProvider struct {
	responses []providerResponse
	callMu    sync.Mutex
	calls     []sdk.ProviderRequest

	// StreamFunc overrides the default Stream behavior if set.
	StreamFunc func(ctx context.Context, req sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error)
}

type providerResponse struct {
	textDeltas []string
	toolCalls  []sdk.ToolCall
	err        error
}

func (m *mockProvider) Stream(ctx context.Context, req sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
	if m.StreamFunc != nil {
		m.callMu.Lock()
		m.calls = append(m.calls, req)
		m.callMu.Unlock()

		return m.StreamFunc(ctx, req)
	}

	m.callMu.Lock()
	idx := len(m.calls)
	m.calls = append(m.calls, req)
	m.callMu.Unlock()

	if idx >= len(m.responses) {
		ch := make(chan sdk.ProviderEvent)
		close(ch)

		return ch, nil
	}

	resp := m.responses[idx]
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
}

// --- mock tool ---

type mockTool struct {
	name    string
	def     sdk.ToolDef
	execute func(ctx context.Context, args map[string]any) (sdk.ToolResult, error)
	callMu  sync.Mutex
	calls   []map[string]any
}

func (m *mockTool) Name() string            { return m.name }
func (m *mockTool) Definition() sdk.ToolDef { return m.def }
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	m.callMu.Lock()
	m.calls = append(m.calls, args)
	m.callMu.Unlock()

	if m.execute != nil {
		return m.execute(ctx, args)
	}

	return sdk.ToolResult{Content: "mock result"}, nil
}

// --- helpers ---

func resetRegistries() {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()
	sdk.ResetToolRegistry()
}

func setupLoop(t *testing.T, providerName string) (*Loop, *bus.Bus, func()) {
	t.Helper()

	l, err := NewLoop(nil, providerName)
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	b := bus.New()

	return l, b, func() {
		_ = b.Close()
	}
}

func registerMockProvider(_ string, mp *mockProvider) {
	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return mp, nil
	})
}

func registerMockTool(mt *mockTool) {
	sdk.RegisterTool(mt.name, func(sdk.Config) (sdk.Tool, error) {
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

	mp := &mockProvider{}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	l.Subscribe(b)

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestLoop_SingleTurn_NoTools(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := &mockProvider{
		responses: []providerResponse{
			{textDeltas: []string{"hello", " world"}},
		},
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "test prompt"))

	turnStart, found := waitForTopic(allCh, TopicTurnStart, 2*time.Second)
	if !found {
		t.Fatal("timeout waiting for turn_start")
	}

	if count, ok := turnStart.Payload.(int); !ok || count != 1 {
		t.Errorf("turn_start payload = %v, want 1", turnStart.Payload)
	}

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for msg_end")
	}

	if msgEnd.Payload != "hello world" {
		t.Errorf("msg_end payload = %q, want %q", msgEnd.Payload, "hello world")
	}

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for turn_end")
	}

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for end")
	}

	mp.callMu.Lock()
	if len(mp.calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(mp.calls))
	}

	if len(mp.calls[0].Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(mp.calls[0].Messages))
	}
	mp.callMu.Unlock()
}

func TestLoop_ToolCallCycle(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := &mockTool{
		name: "bash",
		def:  sdk.ToolDef{Name: "bash", Description: "run commands"},
	}
	registerMockTool(mt)

	mp := &mockProvider{
		responses: []providerResponse{
			{
				toolCalls: []sdk.ToolCall{
					{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "echo hi"}},
				},
			},
			{textDeltas: []string{"done"}},
		},
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "run echo"))

	toolResultEvt, ok := waitForTopic(allCh, TopicToolResult, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for tool_result")
	}

	payload, ok := toolResultEvt.Payload.(map[string]any)
	if !ok {
		t.Fatalf("tool_result payload type = %T", toolResultEvt.Payload)
	}

	if payload["tool"] != "bash" {
		t.Errorf("tool_result tool = %v, want bash", payload["tool"])
	}

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for second msg_end")
	}

	if msgEnd.Payload != "done" {
		t.Errorf("final msg_end = %q, want %q", msgEnd.Payload, "done")
	}

	mt.callMu.Lock()
	if len(mt.calls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(mt.calls))
	} else if mt.calls[0]["command"] != "echo hi" {
		t.Errorf("tool args = %v, want command=echo hi", mt.calls[0])
	}
	mt.callMu.Unlock()

	mp.callMu.Lock()
	if len(mp.calls) != 2 {
		t.Errorf("expected 2 provider calls, got %d", len(mp.calls))
	}
	mp.callMu.Unlock()
}

func TestLoop_SteeringInjection(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := &mockProvider{
		responses: []providerResponse{
			{textDeltas: []string{"first"}},
			{textDeltas: []string{"steered"}},
		},
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))
	b.Publish(sdk.NewEvent(TopicSteer, "steer this"))

	// Steering published before the turn is included in the first provider call.
	// No extra turn is triggered — the steering is already part of the request.
	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for turn_end")
	}

	_, ok = waitForTopic(allCh, TopicEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for end")
	}

	mp.callMu.Lock()
	if len(mp.calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(mp.calls))
	}

	// The single call should have both the prompt and the steering message.
	if len(mp.calls[0].Messages) != 2 {
		t.Errorf("expected 2 messages (prompt + steering), got %d", len(mp.calls[0].Messages))
	}
	mp.callMu.Unlock()
}

func TestLoop_SteeringDuringTurn(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	streamingStarted := make(chan struct{})

	mp := &mockProvider{
		responses: []providerResponse{
			{textDeltas: []string{"first"}},
			{textDeltas: []string{"steered"}},
		},
	}

	mp.StreamFunc = func(ctx context.Context, req sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
		ch := make(chan sdk.ProviderEvent, 2)

		go func() {
			defer close(ch)

			mp.callMu.Lock()
			callIdx := len(mp.calls) - 1
			mp.callMu.Unlock()

			if callIdx == 0 {
				close(streamingStarted)
				time.Sleep(100 * time.Millisecond)
			}

			if callIdx < len(mp.responses) {
				resp := mp.responses[callIdx]

				for _, delta := range resp.textDeltas {
					select {
					case ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: delta}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()

		return ch, nil
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
		if !ok {
			t.Fatalf("timeout waiting for turn_end %d", i+1)
		}
	}

	_, ok := waitForTopic(allCh, TopicEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for end")
	}

	mp.callMu.Lock()
	if len(mp.calls) != 2 {
		t.Fatalf("expected 2 provider calls, got %d", len(mp.calls))
	}

	mp.callMu.Unlock()
}

func TestLoop_FollowupReentry(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := &mockProvider{
		responses: []providerResponse{
			{textDeltas: []string{"first response"}},
			{textDeltas: []string{"follow-up response"}},
		},
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	// Publish prompt and follow-up together — follow-up will be waiting
	// on the channel when the inner loop finishes its first pass.
	b.Publish(sdk.NewEvent(TopicPrompt, "initial"))
	b.Publish(sdk.NewEvent(TopicFollowup, "follow up question"))

	// Should see two turn_end events (one for each turn)
	for i := range 2 {
		_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
		if !ok {
			t.Fatalf("timeout waiting for turn_end %d", i+1)
		}
	}

	_, ok := waitForTopic(allCh, TopicEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for end")
	}

	mp.callMu.Lock()
	if len(mp.calls) != 2 {
		t.Errorf("expected 2 provider calls, got %d", len(mp.calls))
	}
	mp.callMu.Unlock()
}

func TestLoop_ErrorAbort(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := &mockProvider{
		responses: []providerResponse{
			{err: context.DeadlineExceeded},
		},
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "trigger error"))

	_, ok := waitForTopic(allCh, TopicTurnStart, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for turn_start")
	}

	endEvt, ok := waitForTopic(allCh, TopicEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for end")
	}

	if endEvt.Payload == nil {
		t.Error("expected non-nil error payload on end")
	}
}

func TestLoop_ProviderErrorOnStartup(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	// No provider registered
	l, b, cleanup := setupLoop(t, "nonexistent")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "test"))

	endEvts := collectTopic(allCh, TopicEnd, 2*time.Second)
	if len(endEvts) != 1 {
		t.Fatalf("expected exactly 1 end event, got %d", len(endEvts))
	}

	if endEvts[0].Payload == nil {
		t.Error("expected non-nil error payload in end event")
	}
}

func TestLoop_ContextCancellation(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	// Provider that blocks forever until context is canceled.
	blockCh := make(chan sdk.ProviderEvent)

	registerMockProvider("anthropic", &mockProvider{
		responses: []providerResponse{},
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
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close hung — context cancellation did not unblock the loop")
	}

	_, ok := waitForTopic(allCh, TopicEnd, time.Second)
	if !ok {
		t.Error("expected TopicEnd after cancellation")
	}
}

func TestLoop_MsgUpdateEvents(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := &mockProvider{
		responses: []providerResponse{
			{textDeltas: []string{"a", "b", "c"}},
		},
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "stream test"))

	updates := collectTopic(allCh, TopicMsgUpdate, 2*time.Second)
	if len(updates) != 3 {
		t.Fatalf("expected 3 msg_update events, got %d", len(updates))
	}

	expected := []string{"a", "b", "c"}
	for i, u := range updates {
		if u.Payload != expected[i] {
			t.Errorf("update[%d] = %v, want %v", i, u.Payload, expected[i])
		}
	}
}

func TestLoop_MissingToolError(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := &mockProvider{
		responses: []providerResponse{
			{
				toolCalls: []sdk.ToolCall{
					{ID: "tc1", Name: "nonexistent", Arguments: nil},
				},
			},
			{textDeltas: []string{"recovered"}},
		},
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "call missing tool"))

	toolResultEvt, ok := waitForTopic(allCh, TopicToolResult, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for tool_result")
	}

	payload := toolResultEvt.Payload.(map[string]any)

	result := payload["result"].(sdk.ToolResult)
	if !result.IsError {
		t.Error("expected tool_result to have IsError=true")
	}

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 2*time.Second)
	if !ok {
		t.Fatal("timeout waiting for second msg_end")
	}

	if msgEnd.Payload != "recovered" {
		t.Errorf("final response = %q, want %q", msgEnd.Payload, "recovered")
	}
}

func TestLoop_RegisterAsExtension(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	// Manually register (simulates what init() does)
	sdk.RegisterExtension("loop", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewLoop(cfg, "anthropic")
	})

	ext, err := sdk.GetExtension("loop", nil)
	if err != nil {
		t.Fatalf("GetExtension(loop): %v", err)
	}

	if ext.Name() != "loop" {
		t.Errorf("extension name = %q, want %q", ext.Name(), "loop")
	}

	if _, ok := ext.(*Loop); !ok {
		t.Errorf("expected *Loop, got %T", ext)
	}
}

func TestLoop_MultipleToolCalls(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	tool1 := &mockTool{name: "tool-a", def: sdk.ToolDef{Name: "tool-a"}}
	tool2 := &mockTool{name: "tool-b", def: sdk.ToolDef{Name: "tool-b"}}

	registerMockTool(tool1)
	registerMockTool(tool2)

	mp := &mockProvider{
		responses: []providerResponse{
			{
				toolCalls: []sdk.ToolCall{
					{ID: "tc1", Name: "tool-a", Arguments: map[string]any{"x": 1}},
					{ID: "tc2", Name: "tool-b", Arguments: map[string]any{"y": 2}},
				},
			},
			{textDeltas: []string{"both done"}},
		},
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "multi-tool"))

	// Collect all events until end
	var toolResults []sdk.Event

	var finalMsgEnd *sdk.Event

	endDeadline := time.After(5 * time.Second)

	for {
		select {
		case evt, ok := <-allCh:
			if !ok {
				t.Fatal("event channel closed")
			}

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

	if len(toolResults) != 2 {
		t.Fatalf("expected 2 tool_result events, got %d", len(toolResults))
	}

	if finalMsgEnd == nil || finalMsgEnd.Payload != "both done" {
		if finalMsgEnd == nil {
			t.Fatal("no msg_end received")
		}

		t.Errorf("final = %q, want %q", finalMsgEnd.Payload, "both done")
	}
}

func TestLoop_StreamingUpdatesPreserveOrder(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	deltas := make([]string, 20)
	for i := range deltas {
		deltas[i] = strings.Repeat("x", i+1)
	}

	mp := &mockProvider{
		responses: []providerResponse{
			{textDeltas: deltas},
		},
	}
	registerMockProvider("anthropic", mp)

	l, b, cleanup := setupLoop(t, "anthropic")
	defer cleanup()

	allCh := b.SubscribeAll()
	l.Subscribe(b)

	b.Publish(sdk.NewEvent(TopicPrompt, "order test"))

	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 3*time.Second)
	if !ok {
		t.Fatal("timeout waiting for msg_end")
	}

	expected := strings.Join(deltas, "")
	if msgEnd.Payload != expected {
		t.Errorf("streaming content mismatch: got %d chars, want %d", len(msgEnd.Payload.(string)), len(expected))
	}
}
