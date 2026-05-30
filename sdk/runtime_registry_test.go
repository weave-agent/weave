package sdk

import (
	"context"
	"errors"
	"testing"

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

	tool, ok := reg.Get("runtime")
	require.True(t, ok)
	result, err := tool.Run(context.Background(), ToolRequest{Arguments: map[string]any{"value": "ok"}})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Content)

	require.NoError(t, reg.Disable("runtime"))
	_, ok = reg.Get("runtime")
	assert.False(t, ok)
	assert.Equal(t, []RuntimeToolInfo{{Name: "runtime", Definition: ToolDef{Name: "runtime", Description: "runtime tool"}, Enabled: false}}, reg.List())

	require.NoError(t, reg.Enable("runtime"))
	assert.Len(t, reg.List(), 1)

	require.NoError(t, handle.Close())
	_, ok = reg.Get("runtime")
	assert.False(t, ok)
	assert.ErrorIs(t, reg.Unregister("runtime"), ErrRuntimeNotFound)
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
	_, ok := reg.Get("write")
	assert.False(t, ok)

	tool, ok := reg.Get("read")
	require.True(t, ok)
	result, err := tool.Run(context.Background(), ToolRequest{Arguments: map[string]any{"path": "file.txt"}})
	require.NoError(t, err)
	assert.Equal(t, "decorated:file.txt", result.Content)

	require.NoError(t, handle.Close())
	tool, ok = reg.Get("read")
	require.True(t, ok)
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
		return ToolResult{}, err
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
	var observed []string
	reg.UseMiddleware("test", ProviderMiddlewareFuncs{
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

	provider, ok := reg.Get("static")
	require.True(t, ok)
	events, err := provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.NoError(t, err)

	var got []ProviderEvent
	for event := range events {
		got = append(got, event)
	}

	require.Len(t, got, 1)
	assert.Equal(t, "static:system:before", got[0].Content)
	assert.Equal(t, []string{"observe:static:system:before", "after:static:system:before"}, observed)
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

	provider, ok := reg.Get("runtime")
	require.True(t, ok)
	events, err := provider.Stream(context.Background(), ProviderRequest{SystemPrompt: "system"})
	require.NoError(t, err)

	event := <-events
	assert.Equal(t, ProviderEventError, event.Type)
	assert.ErrorIs(t, event.Content.(error), expectedErr)
	_, open := <-events
	assert.False(t, open)

	require.NoError(t, handle.Close())
	_, ok = reg.Get("runtime")
	assert.False(t, ok)
	assert.ErrorIs(t, reg.Unregister("runtime"), ErrRuntimeNotFound)
}

type streamProvider struct {
	prefix string
}

func (p streamProvider) Stream(_ context.Context, req ProviderRequest, _ ...model.StreamOption) (<-chan ProviderEvent, error) {
	ch := make(chan ProviderEvent, 1)
	ch <- ProviderEvent{Type: ProviderEventTextDelta, Content: p.prefix + ":" + req.SystemPrompt}
	close(ch)

	return ch, nil
}
