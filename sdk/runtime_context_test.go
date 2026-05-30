package sdk

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weave-agent/weave/sdk/model"
)

func TestNoopSessionControllerUnsupported(t *testing.T) {
	t.Parallel()

	noop := NoopSessionController{}
	ctx := context.Background()

	assert.ErrorIs(t, noop.SendUserMessage(ctx, "hello"), ErrRuntimeCapabilityUnsupported)
	_, err := noop.AppendEntry(ctx, SessionEntry{Message: NewUserMessage("hello")})
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)
	assert.ErrorIs(t, noop.SetName(ctx, "name"), ErrRuntimeCapabilityUnsupported)
	_, err = noop.Name(ctx)
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)
	assert.ErrorIs(t, noop.SetLabel(ctx, "entry", "label"), ErrRuntimeCapabilityUnsupported)
	_, err = noop.Compact(ctx, CompactRequest{Reason: "test"})
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)
	_, err = noop.Fork(ctx, ForkSessionRequest{Name: "fork"})
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)
	_, err = noop.Switch(ctx, SwitchSessionRequest{SessionID: "session"})
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)
	_, err = noop.Tree(ctx)
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)
}

func TestRuntimeResourceRegistryFailureIsolationAndUnregister(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	reg := NewRuntimeResourceRegistry(
		resourceProviderStub{
			name: "bad",
			listFunc: func(context.Context, ResourceQuery) ([]ResourceInfo, error) {
				return nil, boom
			},
			getFunc: func(context.Context, ResourceQuery) (Resource, error) {
				return Resource{}, ErrRuntimeNotFound
			},
		},
		resourceProviderStub{
			name: "good",
			listFunc: func(context.Context, ResourceQuery) ([]ResourceInfo, error) {
				return []ResourceInfo{{Kind: ResourceKindSkill, URI: "skill://go", Name: "go"}}, nil
			},
			getFunc: func(context.Context, ResourceQuery) (Resource, error) {
				return Resource{ResourceInfo: ResourceInfo{Kind: ResourceKindSkill, URI: "skill://go"}, Content: []byte("content")}, nil
			},
		},
	)

	list := reg.List(context.Background(), ResourceQuery{Kind: ResourceKindSkill})
	require.Len(t, list.Resources, 1)
	assert.Equal(t, "skill://go", list.Resources[0].URI)
	require.Len(t, list.Errors, 1)
	assert.ErrorIs(t, list.Errors[0], boom)
	assert.Equal(t, "bad", list.Errors[0].Provider)

	resource, err := reg.Get(context.Background(), ResourceQuery{URI: "skill://go"})
	require.NoError(t, err)
	assert.Equal(t, []byte("content"), resource.Content)

	handle := reg.Register(resourceProviderStub{
		name: "temp",
		listFunc: func(context.Context, ResourceQuery) ([]ResourceInfo, error) {
			return []ResourceInfo{{Kind: ResourceKindPrompt, URI: "prompt://temp"}}, nil
		},
		getFunc: func(context.Context, ResourceQuery) (Resource, error) {
			return Resource{}, ErrRuntimeNotFound
		},
	})
	assert.Len(t, reg.List(context.Background(), ResourceQuery{}).Resources, 2)
	require.NoError(t, handle.Close())
	assert.Len(t, reg.List(context.Background(), ResourceQuery{}).Resources, 1)
}

func TestRuntimeResourceRegistryLookupFailures(t *testing.T) {
	t.Parallel()

	reg := NewRuntimeResourceRegistry()
	_, err := reg.Get(context.Background(), ResourceQuery{URI: "missing"})
	assert.ErrorIs(t, err, ErrRuntimeNotFound)

	reg.Register(resourceProviderStub{
		name: "bad",
		listFunc: func(context.Context, ResourceQuery) ([]ResourceInfo, error) {
			return nil, nil
		},
		getFunc: func(context.Context, ResourceQuery) (Resource, error) {
			return Resource{}, errors.New("failed")
		},
	})
	_, err = reg.Get(context.Background(), ResourceQuery{URI: "missing"})
	assert.ErrorContains(t, err, "bad: failed")
}

func TestExtensionContextDefaultsAndExecDelegation(t *testing.T) {
	t.Parallel()

	ctx := NewExtensionContext(RuntimeContextOptions{})
	assert.NotNil(t, ctx.Bus())
	assert.NotNil(t, ctx.Hooks())
	assert.NotNil(t, ctx.Tools())
	assert.NotNil(t, ctx.Providers())
	assert.NotNil(t, ctx.Session())
	assert.NotNil(t, ctx.Resources())
	assert.NotNil(t, ctx.Models())

	_, err := ctx.Exec(context.Background(), ExecRequest{Command: "go", Args: []string{"test"}})
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)

	called := false
	withExec := NewExtensionContext(RuntimeContextOptions{
		Exec: func(_ context.Context, req ExecRequest) (ExecResult, error) {
			called = true
			assert.Equal(t, GuardianActionExec, req.Action)

			return ExecResult{Stdout: "ok", ExitCode: 0}, nil
		},
	})
	result, err := withExec.Exec(context.Background(), ExecRequest{Command: "go", Action: GuardianActionExec})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "ok", result.Stdout)
}

func TestRuntimeModelControllerRegistryPreferencesAndEvents(t *testing.T) {
	t.Parallel()

	model.ResetModelRegistry()
	defer model.ResetModelRegistry()
	model.RegisterModel(model.ModelDef{ID: "m1", Provider: "p1", SupportsXHigh: false, Default: true})

	bus := &runtimeContextRecordingBus{}
	prefs := &runtimeContextPrefs{}
	ctrl := NewRuntimeModelController(RuntimeModelControllerOptions{Bus: bus, Prefs: prefs})

	models := ctrl.ListModels()
	require.Len(t, models, 1)
	assert.Equal(t, "m1", models[0].ID)
	_, ok := ctrl.GetModelForProvider("m1", "p1")
	assert.True(t, ok)
	assert.Equal(t, model.ThinkingHigh, ctrl.ClampThinkingLevel(model.ThinkingXHigh, models[0]))

	require.NoError(t, ctrl.SetModel(context.Background(), "m1"))
	current, err := ctrl.CurrentModel(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "m1", current)

	require.NoError(t, ctrl.SetThinkingLevel(context.Background(), model.ThinkingHigh))
	level, err := ctrl.ThinkingLevel(context.Background())
	require.NoError(t, err)
	assert.Equal(t, model.ThinkingHigh, level)

	require.Len(t, bus.events, 2)
	assert.Equal(t, "model.change", bus.events[0].Topic)
	assert.Equal(t, map[string]any{"model": "m1"}, bus.events[0].Payload)
	assert.Equal(t, map[string]any{"thinking": "high"}, bus.events[1].Payload)
	assert.Equal(t, 2, prefs.saved)
}

func TestRuntimeModelControllerUnsupportedWithoutPreferences(t *testing.T) {
	t.Parallel()

	ctrl := NewRuntimeModelController(RuntimeModelControllerOptions{})

	_, err := ctrl.CurrentModel(context.Background())
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)
	assert.ErrorIs(t, ctrl.SetModel(context.Background(), "m1"), ErrRuntimeCapabilityUnsupported)
	_, err = ctrl.ThinkingLevel(context.Background())
	assert.ErrorIs(t, err, ErrRuntimeCapabilityUnsupported)
	assert.ErrorIs(t, ctrl.SetThinkingLevel(context.Background(), model.ThinkingHigh), ErrRuntimeCapabilityUnsupported)
}

type resourceProviderStub struct {
	name     string
	listFunc func(context.Context, ResourceQuery) ([]ResourceInfo, error)
	getFunc  func(context.Context, ResourceQuery) (Resource, error)
}

func (p resourceProviderStub) Name() string { return p.name }

func (p resourceProviderStub) ListResources(ctx context.Context, query ResourceQuery) ([]ResourceInfo, error) {
	if p.listFunc == nil {
		return nil, nil
	}

	return p.listFunc(ctx, query)
}

func (p resourceProviderStub) GetResource(ctx context.Context, query ResourceQuery) (Resource, error) {
	if p.getFunc == nil {
		return Resource{}, ErrRuntimeNotFound
	}

	return p.getFunc(ctx, query)
}

type runtimeContextPrefs struct {
	model         string
	thinkingLevel string
	saved         int
}

func (p *runtimeContextPrefs) Preferences(target any) error {
	switch v := target.(type) {
	case *struct {
		Model string `json:"model"`
	}:
		v.Model = p.model
	case *struct {
		ThinkingLevel string `json:"thinking_level"`
	}:
		v.ThinkingLevel = p.thinkingLevel
	}

	return nil
}

func (p *runtimeContextPrefs) SavePreferences(target any) error {
	switch v := target.(type) {
	case *struct {
		Model string `json:"model"`
	}:
		p.model = v.Model
	case *struct {
		ThinkingLevel string `json:"thinking_level"`
	}:
		p.thinkingLevel = v.ThinkingLevel
	}
	p.saved++

	return nil
}

func (p *runtimeContextPrefs) SaveProviderKey(_, _ string) error { return nil }

type runtimeContextRecordingBus struct {
	events []Event
}

func (b *runtimeContextRecordingBus) Publish(event Event) { b.events = append(b.events, event) }
func (b *runtimeContextRecordingBus) On(string, Handler)  {}
func (b *runtimeContextRecordingBus) OnAll(Handler)       {}
func (b *runtimeContextRecordingBus) Off(Handler)         {}
func (b *runtimeContextRecordingBus) Close() error        { return nil }
