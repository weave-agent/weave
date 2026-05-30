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

func TestRuntimeHooksExposeToolCallDefaults(t *testing.T) {
	hooks := NewRuntimeHooks()
	call := ToolCall{ID: "call-1", Name: "read"}

	result, err := hooks.ToolCall().Run(context.Background(), ToolCallRequest{Call: call})
	require.NoError(t, err)
	assert.Equal(t, ToolCallResult{Call: call, Continue: true}, result)
}

type recordingBus struct {
	events []Event
}

func (b *recordingBus) Publish(e Event) {
	b.events = append(b.events, e)
}

func (b *recordingBus) On(string, Handler) {}
func (b *recordingBus) OnAll(Handler)      {}
func (b *recordingBus) Off(Handler)        {}
func (b *recordingBus) Close() error       { return nil }
