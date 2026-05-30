package sdk

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weave-agent/weave/sdk/model"
)

func TestRuntimeToolRegistry_RegisterGetDisableEnableAndUnregister(t *testing.T) {
	ResetToolRegistry()

	reg := NewRuntimeToolRegistry(nil)
	handle, err := reg.Register(RuntimeTool{
		Name:       "runtime",
		Definition: ToolDef{Name: "runtime", Description: "runtime tool"},
		Run: func(_ context.Context, req ToolRequest) (ToolResult, error) {
			return ToolResult{Content: req.Arguments["value"].(string)}, nil
		},
	})
	require.NoError(t, err)

	tool, err := reg.Get("runtime")
	require.NoError(t, err)

	result, err := tool.Run(context.Background(), ToolRequest{Arguments: map[string]any{"value": "ok"}})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Content)

	require.NoError(t, reg.Disable("runtime"))
	_, err = reg.Get("runtime")
	require.ErrorIs(t, err, ErrRuntimeNotFound)
	_, err = tool.Run(context.Background(), ToolRequest{Arguments: map[string]any{"value": "disabled"}})
	require.ErrorIs(t, err, ErrRuntimeNotFound)
	assert.Equal(t, []RuntimeToolInfo{{Name: "runtime", Definition: ToolDef{Name: "runtime", Description: "runtime tool"}, Enabled: false}}, reg.List())

	require.NoError(t, reg.Enable("runtime"))
	assert.Equal(t, []RuntimeToolInfo{{Name: "runtime", Definition: ToolDef{Name: "runtime", Description: "runtime tool"}, Enabled: true}}, reg.List())

	tool, err = reg.Get("runtime")
	require.NoError(t, err)

	result, err = tool.Run(context.Background(), ToolRequest{Arguments: map[string]any{"value": "again"}})
	require.NoError(t, err)
	assert.Equal(t, "again", result.Content)

	require.NoError(t, handle.Close())

	_, err = reg.Get("runtime")
	require.ErrorIs(t, err, ErrRuntimeNotFound)
	_, err = tool.Run(context.Background(), ToolRequest{Arguments: map[string]any{"value": "removed"}})
	require.ErrorIs(t, err, ErrRuntimeNotFound)
	require.ErrorIs(t, reg.Unregister("runtime"), ErrRuntimeNotFound)
}

func TestRuntimeToolRegistry_RegisterDisabledTool(t *testing.T) {
	ResetToolRegistry()

	reg := NewRuntimeToolRegistry(nil)
	_, err := reg.Register(RuntimeTool{
		Name:       "disabled-runtime",
		Definition: ToolDef{Name: "disabled-runtime", Description: "disabled runtime tool"},
		Disabled:   true,
		Run: func(context.Context, ToolRequest) (ToolResult, error) {
			return ToolResult{Content: "should not run"}, nil
		},
	})
	require.NoError(t, err)

	_, err = reg.Get("disabled-runtime")
	require.ErrorIs(t, err, ErrRuntimeNotFound)
	assert.Equal(t, []RuntimeToolInfo{{
		Name:       "disabled-runtime",
		Definition: ToolDef{Name: "disabled-runtime", Description: "disabled runtime tool"},
		Enabled:    false,
	}}, reg.List())

	require.NoError(t, reg.Enable("disabled-runtime"))
	tool, err := reg.Get("disabled-runtime")
	require.NoError(t, err)

	result, err := tool.Run(context.Background(), ToolRequest{})
	require.NoError(t, err)
	assert.Equal(t, "should not run", result.Content)
}

func TestRuntimeToolRegistry_DuplicateNames(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("static", func(Config, PreferenceReader, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "static" }}, nil
	})

	reg := NewRuntimeToolRegistry(nil)
	_, err := reg.Register(RuntimeTool{Name: "static", Run: func(context.Context, ToolRequest) (ToolResult, error) {
		return ToolResult{}, nil
	}})
	require.ErrorIs(t, err, ErrRuntimeDuplicateName)

	_, err = reg.Register(RuntimeTool{Name: "runtime", Run: func(context.Context, ToolRequest) (ToolResult, error) {
		return ToolResult{}, nil
	}})
	require.NoError(t, err)

	_, err = reg.Register(RuntimeTool{Name: "runtime", Run: func(context.Context, ToolRequest) (ToolResult, error) {
		return ToolResult{}, nil
	}})
	require.ErrorIs(t, err, ErrRuntimeDuplicateName)
}

func TestRuntimeToolRegistry_GetPropagatesStaticFactoryError(t *testing.T) {
	ResetToolRegistry()

	expectedErr := errors.New("tool config failed")

	RegisterTool[struct{}]("broken", func(Config, PreferenceReader, struct{}) (Tool, error) {
		return nil, expectedErr
	})

	reg := NewRuntimeToolRegistry(nil)
	_, err := reg.Get("broken")
	require.ErrorIs(t, err, expectedErr)
	require.ErrorContains(t, err, "get static tool")
}

func TestRuntimeToolRegistry_StaticCompatibilityFilterAndDecorator(t *testing.T) {
	ResetToolRegistry()

	RegisterTool[struct{}]("read", func(Config, PreferenceReader, struct{}) (Tool, error) {
		return &ToolMock{
			NameFunc: func() string { return "read" },
			DefinitionFunc: func() ToolDef {
				return ToolDef{Name: "read", Description: "read files"}
			},
			ExecuteFunc: func(_ context.Context, args map[string]any) (ToolResult, error) {
				return ToolResult{Content: args["path"].(string)}, nil
			},
		}, nil
	})
	RegisterTool[struct{}]("write", func(Config, PreferenceReader, struct{}) (Tool, error) {
		return &ToolMock{NameFunc: func() string { return "write" }}, nil
	})

	reg := NewRuntimeToolRegistry(nil)
	handle := reg.Decorate("test", func(tool Tool) Tool {
		return decoratedTool{Tool: tool}
	})

	SetToolFilter([]string{"read"})
	defer SetToolFilter(nil)

	assert.Equal(t, []RuntimeToolInfo{{Name: "read", Definition: ToolDef{Name: "read", Description: "read files"}, Enabled: true}}, reg.List())
	_, err := reg.Get("write")
	require.ErrorIs(t, err, ErrRuntimeNotFound)

	tool, err := reg.Get("read")
	require.NoError(t, err)

	result, err := tool.Run(context.Background(), ToolRequest{Arguments: map[string]any{"path": "file.txt"}})
	require.NoError(t, err)
	assert.Equal(t, "decorated:file.txt", result.Content)

	require.NoError(t, handle.Close())

	tool, err = reg.Get("read")
	require.NoError(t, err)

	result, err = tool.Run(context.Background(), ToolRequest{Call: ToolCall{Arguments: map[string]any{"path": "call.txt"}}})
	require.NoError(t, err)
	assert.Equal(t, "call.txt", result.Content)
}

type decoratedTool struct {
	Tool
}

func (t decoratedTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	result, err := t.Tool.Execute(ctx, args)
	if err != nil {
		return ToolResult{}, fmt.Errorf("execute decorated tool: %w", err)
	}

	result.Content = "decorated:" + result.Content

	return result, nil
}

func TestRuntimeProviderRegistry_StaticCompatibilityAndMiddleware(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider[struct{}, struct{}]("static", func(Config, struct{}, struct{}) (Provider, error) {
		return streamProvider{prefix: "static"}, nil
	})

	reg := NewRuntimeProviderRegistry(nil)
	provider, err := reg.Get("static")
	require.NoError(t, err)

	var observed []string

	handle := reg.UseMiddleware("test", ProviderMiddlewareFuncs{
		BeforeProviderRequestFunc: func(_ context.Context, req ProviderRequest) (ProviderRequest, error) {
			req.SystemPrompt += ":before"

			return req, nil
		},
		ObserveProviderStreamFunc: func(_ context.Context, event ProviderEvent) error {
			observed = append(observed, "observe:"+event.Content.(string))

			return nil
		},
		AfterProviderResponseFunc: func(_ context.Context, event ProviderEvent) error {
			observed = append(observed, "after:"+event.Content.(string))

			return nil
		},
	})

	events, err := provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.NoError(t, err)

	var got []ProviderEvent
	for event := range events {
		got = append(got, event)
	}

	require.Len(t, got, 1)
	assert.Equal(t, "static:system:before", got[0].Content)
	assert.Equal(t, []string{"observe:static:system:before", "after:static:system:before"}, observed)

	require.NoError(t, handle.Close())

	observed = nil

	events, err = provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.NoError(t, err)

	got = nil
	for event := range events {
		got = append(got, event)
	}

	require.Len(t, got, 1)
	assert.Equal(t, "static:system", got[0].Content)
	assert.Empty(t, observed)
}

func TestRuntimeProviderRegistry_MiddlewarePreservesTokenCounter(t *testing.T) {
	ResetProviderRegistry()

	reg := NewRuntimeProviderRegistry(nil)

	var counted ProviderRequest

	handle, err := reg.Register(RuntimeProvider{
		Name: "runtime",
		Factory: func(Config) (Provider, error) {
			return countingStreamProvider{counted: &counted}, nil
		},
	})
	require.NoError(t, err)

	provider, err := reg.Get("runtime")
	require.NoError(t, err)

	counter, ok := provider.(TokenCounter)
	require.True(t, ok)

	reg.UseMiddleware("test", ProviderMiddlewareFuncs{
		BeforeProviderRequestFunc: func(_ context.Context, req ProviderRequest) (ProviderRequest, error) {
			req.SystemPrompt += ":before"

			return req, nil
		},
	})

	count, err := counter.CountTokens(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.NoError(t, err)
	assert.Equal(t, ProviderRequest{SystemPrompt: "system:before"}, counted)
	assert.Equal(t, TokenCount{InputTokens: len("system:before"), Source: TokenCountSourceExact, Confidence: 1}, count)

	require.NoError(t, handle.Close())

	_, err = counter.CountTokens(context.Background(), ProviderRequest{SystemPrompt: "after-close"})
	require.ErrorIs(t, err, ErrRuntimeNotFound)
}

func TestRuntimeProviderRegistry_RegisterDuplicateUnregisterAndMiddlewareErrors(t *testing.T) {
	ResetProviderRegistry()

	reg := NewRuntimeProviderRegistry(nil)
	handle, err := reg.Register(RuntimeProvider{
		Name: "runtime",
		Factory: func(Config) (Provider, error) {
			return streamProvider{prefix: "runtime"}, nil
		},
	})
	require.NoError(t, err)

	_, err = reg.Register(RuntimeProvider{
		Name: "runtime",
		Factory: func(Config) (Provider, error) {
			return streamProvider{prefix: "duplicate"}, nil
		},
	})
	require.ErrorIs(t, err, ErrRuntimeDuplicateName)
	assert.Equal(t, []RuntimeProviderInfo{{Name: "runtime"}}, reg.List())

	expectedErr := errors.New("observe failed")

	reg.UseMiddleware("test", ProviderMiddlewareFuncs{
		ObserveProviderStreamFunc: func(context.Context, ProviderEvent) error {
			return expectedErr
		},
	})

	provider, err := reg.Get("runtime")
	require.NoError(t, err)

	events, err := provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.NoError(t, err)

	event := <-events
	assert.Equal(t, ProviderEventError, event.Type)
	require.ErrorIs(t, event.Content.(error), expectedErr)

	_, open := <-events
	assert.False(t, open)

	require.NoError(t, handle.Close())

	events, err = provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "after-close"})
	require.ErrorIs(t, err, ErrRuntimeNotFound)
	assert.Nil(t, events)

	_, err = reg.Get("runtime")
	require.ErrorIs(t, err, ErrRuntimeNotFound)
	require.ErrorIs(t, reg.Unregister("runtime"), ErrRuntimeNotFound)
}

func TestRuntimeProviderRegistry_GetPropagatesFactoryErrors(t *testing.T) {
	ResetProviderRegistry()

	staticErr := errors.New("static provider config failed")
	runtimeErr := errors.New("runtime provider config failed")

	RegisterProvider[struct{}, struct{}]("static-broken", func(Config, struct{}, struct{}) (Provider, error) {
		return nil, staticErr
	})

	reg := NewRuntimeProviderRegistry(nil)
	_, err := reg.Register(RuntimeProvider{
		Name: "runtime-broken",
		Factory: func(Config) (Provider, error) {
			return nil, runtimeErr
		},
	})
	require.NoError(t, err)

	_, err = reg.Get("static-broken")
	require.ErrorIs(t, err, staticErr)
	require.ErrorContains(t, err, "get static provider")

	_, err = reg.Get("runtime-broken")
	require.ErrorIs(t, err, runtimeErr)
	require.ErrorContains(t, err, "create runtime provider")
}

func TestRuntimeProviderRegistry_MiddlewareErrorCancelsProviderStream(t *testing.T) {
	reg := NewRuntimeProviderRegistry(nil)
	canceled := make(chan struct{})
	expectedErr := errors.New("observe failed")

	_, err := reg.Register(RuntimeProvider{
		Name: "runtime",
		Factory: func(Config) (Provider, error) {
			return cancelAwareStreamProvider{canceled: canceled}, nil
		},
	})
	require.NoError(t, err)

	reg.UseMiddleware("test", ProviderMiddlewareFuncs{
		ObserveProviderStreamFunc: func(context.Context, ProviderEvent) error {
			return expectedErr
		},
	})

	provider, err := reg.Get("runtime")
	require.NoError(t, err)

	events, err := provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.NoError(t, err)

	event := <-events
	assert.Equal(t, ProviderEventError, event.Type)
	require.ErrorIs(t, event.Content.(error), expectedErr)

	_, open := <-events
	assert.False(t, open)
	require.Eventually(t, func() bool {
		select {
		case <-canceled:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestRuntimeProviderRegistry_BeforeMiddlewareErrorSkipsProvider(t *testing.T) {
	reg := NewRuntimeProviderRegistry(nil)
	expectedErr := errors.New("before failed")
	calls := 0

	_, err := reg.Register(RuntimeProvider{
		Name: "runtime",
		Factory: func(Config) (Provider, error) {
			return streamProvider{prefix: "runtime", calls: &calls}, nil
		},
	})
	require.NoError(t, err)

	reg.UseMiddleware("test", ProviderMiddlewareFuncs{
		BeforeProviderRequestFunc: func(context.Context, ProviderRequest) (ProviderRequest, error) {
			return ProviderRequest{}, expectedErr
		},
	})

	provider, err := reg.Get("runtime")
	require.NoError(t, err)

	events, err := provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, events)
	assert.Zero(t, calls)
}

func TestRuntimeProviderRegistry_AfterMiddlewareErrorStopsStream(t *testing.T) {
	reg := NewRuntimeProviderRegistry(nil)
	expectedErr := errors.New("after failed")

	_, err := reg.Register(RuntimeProvider{
		Name: "runtime",
		Factory: func(Config) (Provider, error) {
			return streamProvider{prefix: "runtime"}, nil
		},
	})
	require.NoError(t, err)

	reg.UseMiddleware("test", ProviderMiddlewareFuncs{
		AfterProviderResponseFunc: func(context.Context, ProviderEvent) error {
			return expectedErr
		},
	})

	provider, err := reg.Get("runtime")
	require.NoError(t, err)

	events, err := provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.NoError(t, err)

	event := <-events
	assert.Equal(t, ProviderEventError, event.Type)
	require.ErrorIs(t, event.Content.(error), expectedErr)

	_, open := <-events
	assert.False(t, open)
}

func TestRuntimeProviderRegistry_UnderlyingStreamErrorIsWrapped(t *testing.T) {
	reg := NewRuntimeProviderRegistry(nil)
	expectedErr := errors.New("stream failed")

	_, err := reg.Register(RuntimeProvider{
		Name: "runtime",
		Factory: func(Config) (Provider, error) {
			return streamProvider{streamErr: expectedErr}, nil
		},
	})
	require.NoError(t, err)

	reg.UseMiddleware("noop", ProviderMiddlewareFuncs{})

	provider, err := reg.Get("runtime")
	require.NoError(t, err)

	events, err := provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, events)
	require.ErrorContains(t, err, "provider stream")
}

type streamProvider struct {
	prefix    string
	streamErr error
	calls     *int
}

func (p streamProvider) Stream(_ context.Context, req ProviderRequest, _ ...model.StreamOption) (<-chan ProviderEvent, error) {
	if p.calls != nil {
		(*p.calls)++
	}

	if p.streamErr != nil {
		return nil, p.streamErr
	}

	ch := make(chan ProviderEvent, 1)
	ch <- ProviderEvent{Type: ProviderEventTextDelta, Content: p.prefix + ":" + req.SystemPrompt}

	close(ch)

	return ch, nil
}

type countingStreamProvider struct {
	counted *ProviderRequest
}

func (p countingStreamProvider) Stream(_ context.Context, req ProviderRequest, _ ...model.StreamOption) (<-chan ProviderEvent, error) {
	ch := make(chan ProviderEvent, 1)
	ch <- ProviderEvent{Type: ProviderEventTextDelta, Content: req.SystemPrompt}

	close(ch)

	return ch, nil
}

func (p countingStreamProvider) CountTokens(_ context.Context, req ProviderRequest, _ ...model.StreamOption) (TokenCount, error) {
	if p.counted != nil {
		*p.counted = req
	}

	return TokenCount{InputTokens: len(req.SystemPrompt), Source: TokenCountSourceExact, Confidence: 1}, nil
}

type cancelAwareStreamProvider struct {
	canceled chan<- struct{}
}

func (p cancelAwareStreamProvider) Stream(ctx context.Context, _ ProviderRequest, _ ...model.StreamOption) (<-chan ProviderEvent, error) {
	ch := make(chan ProviderEvent)
	go func() {
		defer close(ch)

		select {
		case ch <- ProviderEvent{Type: ProviderEventTextDelta, Content: "first"}:
		case <-ctx.Done():
			close(p.canceled)
			return
		}

		select {
		case ch <- ProviderEvent{Type: ProviderEventTextDelta, Content: "second"}:
		case <-ctx.Done():
			close(p.canceled)
			return
		}
	}()

	return ch, nil
}
