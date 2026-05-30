package sdk

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeHookOrderingAndMutation(t *testing.T) {
	hook := NewHook[string, string](WithHookInitialResult(func(req string) string {
		return req
	}))

	hook.Use("second", func(_ context.Context, state *HookState[string, string]) error {
		state.Request += "-second"
		state.Result += "-second"

		return nil
	}, WithHookOrder(20))
	hook.Use("first", func(_ context.Context, state *HookState[string, string]) error {
		state.Request += "-first"
		state.Result += "-first"

		return nil
	}, WithHookOrder(10))
	hook.Use("third", func(_ context.Context, state *HookState[string, string]) error {
		state.Request += "-third"
		state.Result += "-third"

		return nil
	}, WithHookOrder(20))

	state, err := hook.RunState(context.Background(), "start")
	require.NoError(t, err)
	assert.Equal(t, "start-first-second-third", state.Request)
	assert.Equal(t, "start-first-second-third", state.Result)
	assert.False(t, state.Stopped())
}

func TestRuntimeHookStopSkipsLaterHandlers(t *testing.T) {
	hook := NewHook[string, string]()
	called := false

	hook.Use("stop", func(_ context.Context, state *HookState[string, string]) error {
		state.Result = "vetoed"
		state.Stop()

		return nil
	})
	hook.Use("later", func(_ context.Context, state *HookState[string, string]) error {
		called = true
		state.Result = "later"

		return nil
	})

	result, err := hook.Run(context.Background(), "input")
	require.NoError(t, err)
	assert.Equal(t, "vetoed", result)
	assert.False(t, called)
}

func TestRuntimeHookErrorPropagation(t *testing.T) {
	hook := NewHook[string, string]()
	wantErr := errors.New("boom")
	called := false

	hook.Use("error", func(_ context.Context, _ *HookState[string, string]) error {
		return wantErr
	})
	hook.Use("later", func(_ context.Context, _ *HookState[string, string]) error {
		called = true
		return nil
	})

	_, err := hook.Run(context.Background(), "input")
	require.ErrorIs(t, err, wantErr)
	assert.False(t, called)
}

func TestRuntimeHookHandleCloseUnregistersAndCleansUpOnce(t *testing.T) {
	hook := NewHook[string, string]()
	calls := 0
	cleanups := 0

	handle := hook.Use("owner", func(_ context.Context, state *HookState[string, string]) error {
		calls++
		state.Result = "called"

		return nil
	}, WithHookCleanup(func() error {
		cleanups++
		return nil
	}))

	result, err := hook.Run(context.Background(), "input")
	require.NoError(t, err)
	assert.Equal(t, "called", result)

	require.NoError(t, handle.Close())
	require.NoError(t, handle.Close())

	result, err = hook.Run(context.Background(), "input")
	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, 1, calls)
	assert.Equal(t, 1, cleanups)
}

func TestRuntimeHookCleanupErrorsAreJoined(t *testing.T) {
	hook := NewHook[string, string]()
	errA := errors.New("cleanup a")
	errB := errors.New("cleanup b")

	handle := hook.Use("owner", func(_ context.Context, _ *HookState[string, string]) error {
		return nil
	}, WithHookCleanup(func() error {
		return errA
	}), WithHookCleanup(func() error {
		return errB
	}))

	err := handle.Close()
	require.ErrorIs(t, err, errA)
	require.ErrorIs(t, err, errB)
}

func TestNewBusObserverHookPublishesState(t *testing.T) {
	bus := &recordingBus{}
	hook := NewHook[ToolCallRequest, ToolCallResult](WithHookInitialResult(func(req ToolCallRequest) ToolCallResult {
		return ToolCallResult{Call: req.Call, Continue: true}
	}))

	hook.Use("mutate", func(_ context.Context, state *HookState[ToolCallRequest, ToolCallResult]) error {
		state.Result.Call.Name = "edited"
		return nil
	})
	hook.Use("observer", NewBusObserverHook(bus, ProviderEventToolCall, func(state HookState[ToolCallRequest, ToolCallResult]) any {
		return state.Result.Call
	}))

	_, err := hook.Run(context.Background(), ToolCallRequest{Call: ToolCall{Name: "original"}})
	require.NoError(t, err)
	require.Len(t, bus.events, 1)
	assert.Equal(t, ProviderEventToolCall, bus.events[0].Topic)
	assert.Equal(t, ToolCall{Name: "edited"}, bus.events[0].Payload)
}

func TestRuntimeHooksWithBusPublishesCompatibilityEvents(t *testing.T) {
	bus := &recordingBus{}
	hooks := NewRuntimeHooksWithBus(bus)

	hooks.ToolCall().Use("mutate", func(_ context.Context, state *HookState[ToolCallRequest, ToolCallResult]) error {
		state.Result.Call.Name = "edited"
		return nil
	}, WithHookOrder(1))

	_, err := hooks.ToolCall().Run(context.Background(), ToolCallRequest{Call: ToolCall{ID: "call-1", Name: "read"}})
	require.NoError(t, err)

	_, err = hooks.ToolResult().Run(context.Background(), ToolResultRequest{
		Call:   ToolCall{ID: "call-1", Name: "read"},
		Result: ToolResult{Content: "done"},
	})
	require.NoError(t, err)

	_, err = hooks.ToolResult().Run(context.Background(), ToolResultRequest{
		Call:   ToolCall{ID: "call-2", Name: "write"},
		Result: ToolResult{Content: "failed", IsError: true},
	})
	require.NoError(t, err)

	require.Len(t, bus.events, 7)
	assert.Equal(t, TopicToolStart, bus.events[0].Topic)
	assert.Equal(t, ToolProgress{ToolCallID: "call-1", ToolName: "edited"}, bus.events[0].Payload)
	assert.Equal(t, ProviderEventToolCall, bus.events[1].Topic)
	assert.Equal(t, ToolCall{ID: "call-1", Name: "edited"}, bus.events[1].Payload)
	assert.Equal(t, TopicAgentToolCall, bus.events[2].Topic)
	assert.Equal(t, map[string]any{
		legacyAgentPayloadID:   "call-1",
		legacyAgentPayloadTool: "edited",
		legacyAgentPayloadArgs: map[string]any(nil),
	}, bus.events[2].Payload)
	assert.Equal(t, TopicToolComplete, bus.events[3].Topic)
	assert.Equal(t, ToolProgress{ToolCallID: "call-1", ToolName: "read", Content: "done"}, bus.events[3].Payload)
	assert.Equal(t, TopicAgentToolResult, bus.events[4].Topic)
	assert.Equal(t, map[string]any{
		legacyAgentPayloadID:     "call-1",
		legacyAgentPayloadTool:   "read",
		legacyAgentPayloadResult: ToolResult{Content: "done"},
	}, bus.events[4].Payload)
	assert.Equal(t, TopicToolError, bus.events[5].Topic)
	assert.Equal(t, ToolProgress{ToolCallID: "call-2", ToolName: "write", Content: "failed", IsError: true}, bus.events[5].Payload)
	assert.Equal(t, TopicAgentToolResult, bus.events[6].Topic)
	assert.Equal(t, map[string]any{
		legacyAgentPayloadID:     "call-2",
		legacyAgentPayloadTool:   "write",
		legacyAgentPayloadResult: ToolResult{Content: "failed", IsError: true},
	}, bus.events[6].Payload)
}

func TestRuntimeHooksBusObserversPublishAfterAllHandlers(t *testing.T) {
	bus := &recordingBus{}
	hooks := NewRuntimeHooksWithBus(bus)

	hooks.ToolCall().Use("late", func(_ context.Context, state *HookState[ToolCallRequest, ToolCallResult]) error {
		state.Result.Call.Name = "late-edit"
		return nil
	}, WithHookOrder(20_000))

	_, err := hooks.ToolCall().Run(context.Background(), ToolCallRequest{Call: ToolCall{ID: "call-1", Name: "read"}})
	require.NoError(t, err)

	require.Len(t, bus.events, 3)
	assert.Equal(t, ToolProgress{ToolCallID: "call-1", ToolName: "late-edit"}, bus.events[0].Payload)
	assert.Equal(t, ToolCall{ID: "call-1", Name: "late-edit"}, bus.events[1].Payload)
	assert.Equal(t, map[string]any{
		legacyAgentPayloadID:   "call-1",
		legacyAgentPayloadTool: "late-edit",
		legacyAgentPayloadArgs: map[string]any(nil),
	}, bus.events[2].Payload)
}

func TestRuntimeHooksWithBusPublishesLifecycleCompatibilityEvents(t *testing.T) {
	bus := &recordingBus{}
	hooks := NewRuntimeHooksWithBus(bus)
	message := NewUserMessage("hello")

	_, err := hooks.Input().Run(context.Background(), InputHookRequest{Content: "hello"})
	require.NoError(t, err)
	_, err = hooks.ProviderRequest().Run(context.Background(), ProviderRequestHookRequest{
		Provider: "openai",
		Request:  ProviderRequest{SystemPrompt: "system"},
	})
	require.NoError(t, err)
	_, err = hooks.ProviderResponse().Run(context.Background(), ProviderResponseHookRequest{
		Provider: "openai",
		Event:    ProviderEvent{Type: ProviderEventTextDelta, Content: "hi"},
	})
	require.NoError(t, err)
	_, err = hooks.Message().Run(context.Background(), MessageHookRequest{Message: message})
	require.NoError(t, err)
	_, err = hooks.Turn().Run(context.Background(), TurnHookRequest{Messages: []Message{message}})
	require.NoError(t, err)
	_, err = hooks.Session().Run(context.Background(), SessionHookRequest{
		Event: TopicSessionResume,
		Entry: SessionResumePayload{SessionID: "session-1", Messages: []Message{message}},
	})
	require.NoError(t, err)

	require.Len(t, bus.events, 7)
	assert.Equal(t, TopicProviderRequest, bus.events[0].Topic)
	assert.Equal(t, ProviderRequestBusPayload{Provider: "openai", Request: ProviderRequest{SystemPrompt: "system"}}, bus.events[0].Payload)
	assert.Equal(t, TopicAgentMessageStart, bus.events[1].Topic)
	assert.Nil(t, bus.events[1].Payload)
	assert.Equal(t, TopicProviderResponse, bus.events[2].Topic)
	assert.Equal(t, ProviderResponseBusPayload{Provider: "openai", Event: ProviderEvent{Type: ProviderEventTextDelta, Content: "hi"}}, bus.events[2].Payload)
	assert.Equal(t, TopicAgentMessageUpdate, bus.events[3].Topic)
	assert.Equal(t, "hi", bus.events[3].Payload)
	assert.Equal(t, TopicMessage, bus.events[4].Topic)
	assert.Equal(t, message, bus.events[4].Payload)
	assert.Equal(t, TopicTurn, bus.events[5].Topic)
	assert.Equal(t, TurnHookResult{Messages: []Message{message}}, bus.events[5].Payload)
	assert.Equal(t, TopicSessionResume, bus.events[6].Topic)
	assert.Equal(t, SessionResumePayload{SessionID: "session-1", Messages: []Message{message}}, bus.events[6].Payload)
}

func TestRuntimeHooksWithBusPublishesLegacyAgentTopics(t *testing.T) {
	bus := &recordingBus{}
	hooks := NewRuntimeHooksWithBus(bus)

	_, err := hooks.ProviderRequest().Run(context.Background(), ProviderRequestHookRequest{Provider: "openai"})
	require.NoError(t, err)
	_, err = hooks.ProviderResponse().Run(context.Background(), ProviderResponseHookRequest{
		Provider: "openai",
		Event:    ProviderEvent{Type: ProviderEventTextDelta, Content: "delta"},
	})
	require.NoError(t, err)
	_, err = hooks.ToolCall().Run(context.Background(), ToolCallRequest{
		Call: ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{"path": "file.txt"}},
	})
	require.NoError(t, err)
	_, err = hooks.ToolResult().Run(context.Background(), ToolResultRequest{
		Call:   ToolCall{ID: "call-1", Name: "read"},
		Result: ToolResult{Content: "done"},
	})
	require.NoError(t, err)

	assistant := NewAssistantMessage("final")
	assistant.ToolCalls = []ToolCall{{ID: "call-2", Name: "write"}}
	assistant.Thinking = []SignedThinking{{Thinking: "think "}, {Thinking: "again"}}
	assistant.RedactedThinking = []RedactedThinking{{Data: "redacted"}}

	_, err = hooks.Message().Run(context.Background(), MessageHookRequest{Message: assistant})
	require.NoError(t, err)

	events := eventsByTopic(bus.events)
	assert.Equal(t, []any{nil}, events[TopicAgentMessageStart])
	assert.Equal(t, []any{"delta"}, events[TopicAgentMessageUpdate])
	assert.Equal(t, []any{map[string]any{
		legacyAgentPayloadID:   "call-1",
		legacyAgentPayloadTool: "read",
		legacyAgentPayloadArgs: map[string]any{"path": "file.txt"},
	}}, events[TopicAgentToolCall])
	assert.Equal(t, []any{map[string]any{
		legacyAgentPayloadID:     "call-1",
		legacyAgentPayloadTool:   "read",
		legacyAgentPayloadResult: ToolResult{Content: "done"},
	}}, events[TopicAgentToolResult])
	assert.Equal(t, []any{map[string]any{
		legacyAgentPayloadContent:          "final",
		legacyAgentPayloadToolCalls:        []ToolCall{{ID: "call-2", Name: "write"}},
		legacyAgentPayloadThinking:         "think again",
		legacyAgentPayloadRedactedThinking: []RedactedThinking{{Data: "redacted"}},
	}}, events[TopicAgentMessageEnd])
}

func TestRuntimeHooksSessionResumeSetsInitialBeforePublishing(t *testing.T) {
	ResetInitialSession()
	t.Cleanup(ResetInitialSession)

	initialVisibleDuringPublish := false
	bus := &recordingBus{
		onPublish: func(event Event) {
			if event.Topic != TopicSessionResume {
				return
			}

			payload, ok := GetInitialSession()
			initialVisibleDuringPublish = ok && payload.SessionID == "mutated"
		},
	}
	hooks := NewRuntimeHooksWithBus(bus)

	hooks.Session().Use("mutate", func(_ context.Context, state *HookState[SessionHookRequest, SessionHookResult]) error {
		payload, ok := state.Result.Entry.(SessionResumePayload)
		require.True(t, ok)

		payload.SessionID = "mutated"
		state.Result.Entry = payload

		return nil
	})

	_, err := hooks.Session().Run(context.Background(), SessionHookRequest{
		Event: TopicSessionResume,
		Entry: SessionResumePayload{SessionID: "original"},
	})
	require.NoError(t, err)
	assert.True(t, initialVisibleDuringPublish)
}

func TestRuntimeHooksWithBusPublishesFinalHookResultWithoutFallback(t *testing.T) {
	bus := &recordingBus{}
	hooks := NewRuntimeHooksWithBus(bus)

	hooks.ProviderResponse().Use("redact", func(_ context.Context, state *HookState[ProviderResponseHookRequest, ProviderResponseHookResult]) error {
		state.Result.Event = ProviderEvent{}

		return nil
	})
	hooks.Message().Use("redact", func(_ context.Context, state *HookState[MessageHookRequest, MessageHookResult]) error {
		state.Result.Message = Message{}

		return nil
	})
	hooks.Turn().Use("redact", func(_ context.Context, state *HookState[TurnHookRequest, TurnHookResult]) error {
		state.Result.Messages = nil

		return nil
	})
	hooks.Session().Use("redact", func(_ context.Context, state *HookState[SessionHookRequest, SessionHookResult]) error {
		state.Result.Entry = nil

		return nil
	})

	message := NewUserMessage("secret")
	_, err := hooks.ProviderResponse().Run(context.Background(), ProviderResponseHookRequest{
		Provider: "openai",
		Event:    ProviderEvent{Type: ProviderEventTextDelta, Content: "secret"},
	})
	require.NoError(t, err)
	_, err = hooks.Message().Run(context.Background(), MessageHookRequest{Message: message})
	require.NoError(t, err)
	_, err = hooks.Turn().Run(context.Background(), TurnHookRequest{Messages: []Message{message}})
	require.NoError(t, err)
	_, err = hooks.Session().Run(context.Background(), SessionHookRequest{
		Event: TopicSessionResume,
		Entry: SessionResumePayload{SessionID: "secret"},
	})
	require.NoError(t, err)

	require.Len(t, bus.events, 4)
	assert.Equal(t, TopicProviderResponse, bus.events[0].Topic)
	assert.Equal(t, ProviderResponseBusPayload{Provider: "openai"}, bus.events[0].Payload)
	assert.Equal(t, TopicMessage, bus.events[1].Topic)
	assert.Equal(t, Message{}, bus.events[1].Payload)
	assert.Equal(t, TopicTurn, bus.events[2].Topic)
	assert.Equal(t, TurnHookResult{}, bus.events[2].Payload)
	assert.Equal(t, TopicSessionResume, bus.events[3].Topic)
	assert.Nil(t, bus.events[3].Payload)
}

func TestRuntimeHooksBusObserverHandleUnregisters(t *testing.T) {
	bus := &recordingBus{}
	hooks := NewRuntimeHooks()
	handle := hooks.AttachBusObservers(bus)
	require.NoError(t, handle.Close())

	_, err := hooks.ToolCall().Run(context.Background(), ToolCallRequest{Call: ToolCall{ID: "call-1", Name: "read"}})
	require.NoError(t, err)
	assert.Empty(t, bus.events)
}

func TestExtensionContextDefaultHooksPublishBusCompatibilityEvents(t *testing.T) {
	bus := &recordingBus{}
	ctx := NewExtensionContext(RuntimeContextOptions{Bus: bus})

	_, err := ctx.Hooks().ToolCall().Run(context.Background(), ToolCallRequest{Call: ToolCall{ID: "call-1", Name: "read"}})
	require.NoError(t, err)

	require.Len(t, bus.events, 3)
	assert.Equal(t, TopicToolStart, bus.events[0].Topic)
	assert.Equal(t, ProviderEventToolCall, bus.events[1].Topic)
	assert.Equal(t, TopicAgentToolCall, bus.events[2].Topic)
}

func TestRuntimeHooksExposeToolCallDefaults(t *testing.T) {
	hooks := NewRuntimeHooks()
	call := ToolCall{ID: "call-1", Name: "read"}

	result, err := hooks.ToolCall().Run(context.Background(), ToolCallRequest{Call: call})
	require.NoError(t, err)
	assert.Equal(t, ToolCallResult{Call: call, Continue: true}, result)
}

func TestRuntimeHooksExposeNoHandlerPassThroughDefaults(t *testing.T) {
	hooks := NewRuntimeHooks()
	message := NewUserMessage("hello")

	input, err := hooks.Input().Run(context.Background(), InputHookRequest{Content: "hello"})
	require.NoError(t, err)
	assert.Equal(t, InputHookResult{Content: "hello"}, input)

	prompt, err := hooks.Prompt().Run(context.Background(), PromptHookRequest{
		SystemPrompt: "system",
		Messages:     []Message{message},
	})
	require.NoError(t, err)
	assert.Equal(t, PromptHookResult{SystemPrompt: "system", Messages: []Message{message}}, prompt)

	contextResult, err := hooks.Context().Run(context.Background(), ContextHookRequest{Messages: []Message{message}})
	require.NoError(t, err)
	assert.Equal(t, ContextHookResult{Messages: []Message{message}}, contextResult)

	event := ProviderEvent{Type: ProviderEventTextDelta, Content: "delta"}
	providerResponse, err := hooks.ProviderResponse().Run(context.Background(), ProviderResponseHookRequest{Provider: "openai", Event: event})
	require.NoError(t, err)
	assert.Equal(t, ProviderResponseHookResult{Event: event}, providerResponse)

	turn, err := hooks.Turn().Run(context.Background(), TurnHookRequest{Messages: []Message{message}})
	require.NoError(t, err)
	assert.Equal(t, TurnHookResult{Messages: []Message{message}}, turn)

	session, err := hooks.Session().Run(context.Background(), SessionHookRequest{Event: TopicSessionResume, Entry: "payload"})
	require.NoError(t, err)
	assert.Equal(t, SessionHookResult{Entry: "payload"}, session)
}

type recordingBus struct {
	events    []Event
	onPublish func(Event)
}

func (b *recordingBus) Publish(e Event) {
	b.events = append(b.events, e)
	if b.onPublish != nil {
		b.onPublish(e)
	}
}

func eventsByTopic(events []Event) map[string][]any {
	result := make(map[string][]any)
	for _, event := range events {
		result[event.Topic] = append(result[event.Topic], event.Payload)
	}

	return result
}

func (b *recordingBus) On(string, Handler) {}
func (b *recordingBus) OnAll(Handler)      {}
func (b *recordingBus) Off(Handler)        {}
func (b *recordingBus) Close() error       { return nil }
