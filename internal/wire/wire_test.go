package wire

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventbus "github.com/weave-agent/weave/bus"
	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/model"
)

func coreCfg() CoreWireConfig {
	return CoreWireConfig{AgentLoop: "agent"}
}

func TestWire_NoExtensions(t *testing.T) {
	sdk.ResetExtensionRegistry()

	mockBus := &BusMock{}

	wired, err := WireExtensions(nil, mockBus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)
}

func TestWire_EmptyExtensions(t *testing.T) {
	sdk.ResetExtensionRegistry()

	mockBus := &BusMock{}

	wired, err := WireExtensions([]string{}, mockBus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)
}

func TestWire_SubscribesAllExtensions(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var subscribed atomic.Int32

	sdk.RegisterExtension("ext-a", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-a", func(bus sdk.Bus) error {
			subscribed.Add(1)
			return nil
		}), nil
	})
	sdk.RegisterExtension("ext-b", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-b", func(bus sdk.Bus) error {
			subscribed.Add(1)
			return nil
		}), nil
	})

	mockBus := &BusMock{}

	wired, err := WireExtensions([]string{"ext-a", "ext-b"}, mockBus, nil)
	require.NoError(t, err)

	assert.Equal(t, int32(2), subscribed.Load())

	_ = wired
}

func TestWire_MissingExtension(t *testing.T) {
	sdk.ResetExtensionRegistry()

	bus := &BusMock{}

	wired, err := WireExtensions([]string{"nonexistent"}, bus, nil)
	require.NoError(t, err, "unregistered extension should be silently skipped")
	require.NotNil(t, wired)
	assert.Empty(t, wired.extensions)
}

func TestWire_ReceiveBusInSubscribe(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var receivedBus sdk.Bus

	sdk.RegisterExtension("ext-c", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-c", func(bus sdk.Bus) error {
			receivedBus = bus
			return nil
		}), nil
	})

	bus := &BusMock{}

	_, err := WireExtensions([]string{"ext-c"}, bus, nil)
	require.NoError(t, err)
	require.NotNil(t, receivedBus)
}

func TestWire_PartialMissing(t *testing.T) {
	sdk.ResetExtensionRegistry()

	sdk.RegisterExtension("good", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("good", func(bus sdk.Bus) error { return nil }), nil
	})

	bus := &BusMock{}

	wired, err := WireExtensions([]string{"good", "missing"}, bus, nil)
	require.NoError(t, err, "unregistered extension should be silently skipped")
	require.Len(t, wired.extensions, 1)
	assert.Equal(t, "good", wired.extensions[0].Name())
}

func TestWire_RuntimeExtensionsShareRuntimeServices(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var (
		toolHandle sdk.HookHandle
		sawTool    bool
	)

	sdk.RegisterRuntimeExtension("runtime-a", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.RuntimeExtension, error) {
		return sdk.NewRuntimeExtensionFuncWithClose(func(ctx sdk.ExtensionContext) error {
			var err error

			toolHandle, err = ctx.Tools().Register(sdk.RuntimeTool{
				Name:       "runtime-shared",
				Definition: sdk.ToolDef{Name: "runtime-shared"},
				Run: func(context.Context, sdk.ToolRequest) (sdk.ToolResult, error) {
					return sdk.ToolResult{Content: "ok"}, nil
				},
			})
			if err != nil {
				return fmt.Errorf("register runtime-shared tool: %w", err)
			}

			return nil
		}, func() error {
			return toolHandle.Close()
		}), nil
	})
	sdk.RegisterRuntimeExtension("runtime-b", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.RuntimeExtension, error) {
		return sdk.NewRuntimeExtensionFunc(func(ctx sdk.ExtensionContext) error {
			tool, ok := ctx.Tools().Get("runtime-shared")
			if !ok {
				return errors.New("runtime-shared tool not found")
			}

			result, err := tool.Run(context.Background(), sdk.ToolRequest{Name: "runtime-shared"})
			if err != nil {
				return fmt.Errorf("run runtime-shared tool: %w", err)
			}

			sawTool = result.Content == "ok"

			return nil
		}), nil
	})

	bus := &BusMock{}
	wired, err := WireExtensions([]string{"runtime-a", "runtime-b"}, bus, nil)
	require.NoError(t, err)
	assert.True(t, sawTool)
	require.NotNil(t, wired.Runtime())

	_, ok := wired.Runtime().Tools().Get("runtime-shared")
	assert.True(t, ok)

	require.NoError(t, wired.Close())
	_, ok = wired.Runtime().Tools().Get("runtime-shared")
	assert.False(t, ok)
}

func TestWire_RuntimeExtensionSubscribeErrorClosesPartialRegistration(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var (
		runtime    sdk.ExtensionContext
		toolHandle sdk.HookHandle
		closed     bool
	)

	expectedErr := errors.New("register failed")

	sdk.RegisterRuntimeExtension("runtime-fail", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.RuntimeExtension, error) {
		return sdk.NewRuntimeExtensionFuncWithClose(func(ctx sdk.ExtensionContext) error {
			var err error

			runtime = ctx

			toolHandle, err = ctx.Tools().Register(sdk.RuntimeTool{
				Name:       "runtime-partial",
				Definition: sdk.ToolDef{Name: "runtime-partial"},
				Run: func(context.Context, sdk.ToolRequest) (sdk.ToolResult, error) {
					return sdk.ToolResult{Content: "partial"}, nil
				},
			})
			if err != nil {
				return fmt.Errorf("register runtime-partial tool: %w", err)
			}

			return expectedErr
		}, func() error {
			closed = true

			if toolHandle == nil {
				return nil
			}

			return toolHandle.Close()
		}), nil
	})

	bus := &BusMock{}
	wired, err := WireExtensions([]string{"runtime-fail"}, bus, nil)
	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, wired)
	assert.True(t, closed)

	_, ok := runtime.Tools().Get("runtime-partial")
	assert.False(t, ok)
}

func TestWire_SkipsUIExtension(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetUIExtensionRegistry()

	sdk.RegisterUIExtension("tui-diffview", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.UIExtension, error) {
		return stubUIExt{name: "tui-diffview"}, nil
	})

	bus := &BusMock{}

	_, err := WireExtensions([]string{"tui-diffview"}, bus, nil)
	require.NoError(t, err, "UI extension should be silently skipped")
}

type stubUIExt struct{ name string }

func (s stubUIExt) Name() string      { return s.name }
func (s stubUIExt) Register(_ sdk.UI) {}

func TestWire_PassesConfigToFactory(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var receivedCfg sdk.Config

	sdk.RegisterExtension("cfg-ext", func(cfg sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		receivedCfg = cfg
		return sdk.NewExtensionFunc("cfg-ext", func(sdk.Bus) error { return nil }), nil
	})

	cfg := sdk.FilePathConfig("/test/.weave/settings.json")
	bus := &BusMock{}

	_, err := WireExtensions([]string{"cfg-ext"}, bus, cfg)
	require.NoError(t, err)
	require.NotNil(t, receivedCfg)
	assert.Equal(t, "/test/.weave/settings.json", receivedCfg.FilePath())
}

func TestWire_FactoryError(t *testing.T) {
	sdk.ResetExtensionRegistry()

	sdk.RegisterExtension("bad", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return nil, errors.New("init failed")
	})

	bus := &BusMock{}

	_, err := WireExtensions([]string{"bad"}, bus, nil)
	require.Error(t, err)
}

func TestWire_FactoryErrorWrappingErrNotRegistered(t *testing.T) {
	sdk.ResetExtensionRegistry()

	// A registered factory that wraps ErrNotRegistered for a missing dependency
	// should be treated as a fatal error, not silently skipped.
	sdk.RegisterExtension("bad-dep", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return nil, fmt.Errorf("missing dependency: %w", sdk.ErrNotRegistered)
	})

	bus := &BusMock{}

	_, err := WireExtensions([]string{"bad-dep"}, bus, nil)
	require.Error(t, err, "factory error wrapping ErrNotRegistered should be fatal")
}

func TestWired_Close(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var closed atomic.Int32

	sdk.RegisterExtension("a", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("a", func(sdk.Bus) error { return nil }, func() error {
			closed.Add(1)
			return nil
		}), nil
	})
	sdk.RegisterExtension("b", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("b", func(sdk.Bus) error { return nil }, func() error {
			closed.Add(1)
			return nil
		}), nil
	})

	bus := &BusMock{}

	wired, err := WireExtensions([]string{"a", "b"}, bus, nil)
	require.NoError(t, err, "Wire")

	require.NoError(t, wired.Close(), "Close")
	assert.Equal(t, int32(2), closed.Load())
}

func TestWired_CloseReverseOrder(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var order []string

	sdk.RegisterExtension("first", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("first", func(sdk.Bus) error { return nil }, func() error {
			order = append(order, "first")
			return nil
		}), nil
	})
	sdk.RegisterExtension("second", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("second", func(sdk.Bus) error { return nil }, func() error {
			order = append(order, "second")
			return nil
		}), nil
	})

	bus := &BusMock{}

	wired, err := WireExtensions([]string{"first", "second"}, bus, nil)
	require.NoError(t, err, "Wire")

	require.NoError(t, wired.Close(), "Close")
	assert.Equal(t, []string{"second", "first"}, order)
}

func TestWireWithCore_MergesCoreAndOptional(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var names []string

	reg := func(n string) {
		sdk.RegisterExtension(n, func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
			return sdk.NewExtensionFunc(n, func(sdk.Bus) error {
				names = append(names, n)
				return nil
			}), nil
		})
	}
	reg("agent")
	reg("bash-tool")
	reg("file-tool")

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg(), []string{"bash-tool", "file-tool"}, bus, nil)
	require.NoError(t, err, "WireWithCore")

	want := []string{"agent", "bash-tool", "file-tool"}
	require.Len(t, names, len(want))

	for i, n := range want {
		assert.Equal(t, n, names[i])
	}
}

func TestWireWithCore_Deduplicates(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var subscribed atomic.Int32

	reg := func(n string) {
		sdk.RegisterExtension(n, func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
			return sdk.NewExtensionFunc(n, func(sdk.Bus) error {
				subscribed.Add(1)
				return nil
			}), nil
		})
	}
	reg("agent")
	reg("bash-tool")

	bus := &BusMock{}

	_, err := WireWithCore(
		coreCfg(),
		[]string{"bash-tool", "agent"},
		bus,
		nil,
	)
	require.NoError(t, err, "WireWithCore")

	assert.Equal(t, int32(2), subscribed.Load())
}

func TestWireWithCore_CoreOnly(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var names []string

	reg := func(n string) {
		sdk.RegisterExtension(n, func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
			return sdk.NewExtensionFunc(n, func(sdk.Bus) error {
				names = append(names, n)
				return nil
			}), nil
		})
	}
	reg("agent")

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg(), nil, bus, nil)
	require.NoError(t, err, "WireWithCore")

	require.Len(t, names, 1)
	assert.Equal(t, "agent", names[0])
}

func TestWireWithCore_ErrMissingAgentLoop(t *testing.T) {
	sdk.ResetExtensionRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{}, nil, bus, nil)
	require.Error(t, err)
	assert.Equal(t, "wire: agent-loop is required", err.Error())
}

func TestWireWithCore_AgentLoopNotRegistered(t *testing.T) {
	sdk.ResetExtensionRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "nonexistent"}, nil, bus, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent-loop extension \"nonexistent\" is not registered")
}

func TestWireWithCore_NoProviderRequired(t *testing.T) {
	sdk.ResetExtensionRegistry()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "agent"}, nil, bus, nil)
	require.NoError(t, err)
}

func TestWireWithCore_FactoryErrorRollback(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var closed atomic.Int32

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("agent", func(sdk.Bus) error { return nil }, func() error {
			closed.Add(1)
			return nil
		}), nil
	})
	sdk.RegisterExtension("bad", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return nil, errors.New("init failed")
	})

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg(), []string{"bad"}, bus, nil)
	require.Error(t, err)

	assert.Equal(t, int32(1), closed.Load())
}

func TestWired_CloseErrorAggregation(t *testing.T) {
	sdk.ResetExtensionRegistry()

	sdk.RegisterExtension("a", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("a", func(sdk.Bus) error { return nil }, func() error {
			return errors.New("a failed")
		}), nil
	})
	sdk.RegisterExtension("b", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("b", func(sdk.Bus) error { return nil }, func() error {
			return errors.New("b failed")
		}), nil
	})

	bus := &BusMock{}

	wired, err := WireExtensions([]string{"a", "b"}, bus, nil)
	require.NoError(t, err, "Wire")

	err = wired.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a failed")
	assert.Contains(t, err.Error(), "b failed")
}

func TestWireWithCore_PassesConfigToFactories(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var receivedCfg sdk.Config

	sdk.RegisterExtension("agent", func(cfg sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		receivedCfg = cfg
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	cfg := sdk.FilePathConfig("/test/.weave/settings.json")
	bus := &BusMock{}

	_, err := WireWithCore(coreCfg(), nil, bus, cfg)
	require.NoError(t, err, "WireWithCore")

	require.NotNil(t, receivedCfg)
	assert.Equal(t, "/test/.weave/settings.json", receivedCfg.FilePath())
}

func TestWire_ExtensionCallsBusOn(t *testing.T) {
	sdk.ResetExtensionRegistry()

	sdk.RegisterExtension("on-ext", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("on-ext", func(bus sdk.Bus) error {
			bus.On("test.topic", func(e sdk.Event) error { return nil })
			return nil
		}), nil
	})

	bus := &BusMock{}

	_, err := WireExtensions([]string{"on-ext"}, bus, nil)
	require.NoError(t, err)

	onCalls := bus.OnCalls()
	require.Len(t, onCalls, 1)
	assert.Equal(t, "test.topic", onCalls[0].Topic)
}

func TestWire_ExtensionCallsBusOnAll(t *testing.T) {
	sdk.ResetExtensionRegistry()

	sdk.RegisterExtension("onall-ext", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("onall-ext", func(bus sdk.Bus) error {
			bus.OnAll(func(e sdk.Event) error { return nil })
			return nil
		}), nil
	})

	bus := &BusMock{}

	_, err := WireExtensions([]string{"onall-ext"}, bus, nil)
	require.NoError(t, err)

	onAllCalls := bus.OnAllCalls()
	require.Len(t, onAllCalls, 1)
}

func TestWire_MultipleExtensionsRegisterHandlers(t *testing.T) {
	sdk.ResetExtensionRegistry()

	sdk.RegisterExtension("ext-1", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-1", func(bus sdk.Bus) error {
			bus.On("topic.a", func(e sdk.Event) error { return nil })
			bus.On("topic.b", func(e sdk.Event) error { return nil })

			return nil
		}), nil
	})
	sdk.RegisterExtension("ext-2", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-2", func(bus sdk.Bus) error {
			bus.OnAll(func(e sdk.Event) error { return nil })
			return nil
		}), nil
	})

	bus := &BusMock{}

	_, err := WireExtensions([]string{"ext-1", "ext-2"}, bus, nil)
	require.NoError(t, err)

	onCalls := bus.OnCalls()
	require.Len(t, onCalls, 2)
	assert.Equal(t, "topic.a", onCalls[0].Topic)
	assert.Equal(t, "topic.b", onCalls[1].Topic)

	onAllCalls := bus.OnAllCalls()
	require.Len(t, onAllCalls, 1)
}

func TestWireWithCore_PublishesAppStarted(t *testing.T) {
	sdk.ResetExtensionRegistry()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	realBus := eventbus.New()
	defer realBus.Close()

	var appStartedReceived atomic.Bool

	realBus.On("app.started", func(e sdk.Event) error {
		appStartedReceived.Store(true)
		return nil
	})

	_, err := WireWithCore(coreCfg(), nil, realBus, sdk.FilePathConfig(""))
	require.NoError(t, err, "WireWithCore")

	// Wait for the async publish to run.
	require.Eventually(t, appStartedReceived.Load, time.Second, 10*time.Millisecond, "app.started event should be published")
}

func TestWireWithCore_InvokesBusSubscribers(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetBusSubscribers()

	defer sdk.ResetBusSubscribers()

	var subscriberCalled atomic.Bool

	sdk.OnBusReady(func(bus sdk.Bus) {
		subscriberCalled.Store(true)
	})

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg(), nil, bus, nil)
	require.NoError(t, err)

	assert.True(t, subscriberCalled.Load(), "bus subscriber should be called")
}

func TestWireWithCore_ExtensionUsesBusOn(t *testing.T) {
	sdk.ResetExtensionRegistry()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(bus sdk.Bus) error {
			bus.On("agent.prompt", func(e sdk.Event) error { return nil })
			return nil
		}), nil
	})

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg(), nil, bus, nil)
	require.NoError(t, err)

	onCalls := bus.OnCalls()
	require.Len(t, onCalls, 1)
	assert.Equal(t, "agent.prompt", onCalls[0].Topic)
}

func TestWire_WireSubscribesExtensionsInProcess(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var subscribeCalled bool

	sdk.RegisterExtension("noop", func(cfg sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) error {
			subscribeCalled = true

			b.Publish(sdk.NewEvent("noop.subscribed", "hello"))

			return nil
		}), nil
	})

	realBus := eventbus.New()

	var received sdk.Event

	realBus.OnAll(func(e sdk.Event) error {
		received = e
		return nil
	})

	wired, err := WireExtensions([]string{"noop"}, realBus, nil)
	require.NoError(t, err, "Wire")

	require.True(t, subscribeCalled, "Subscribe was not called")

	require.NoError(t, wired.Close(), "Close")
	_ = realBus.Close()

	assert.Equal(t, "noop.subscribed", received.Topic)
	assert.Equal(t, "hello", received.Payload)
}

func TestMergeCoreAndOptional_AgentLoopOnly(t *testing.T) {
	result := mergeCoreAndOptional("agent", nil)
	assert.Equal(t, []string{"agent"}, result)
}

func TestMergeCoreAndOptional_WithOptionalExts(t *testing.T) {
	result := mergeCoreAndOptional("agent", []string{"bash", "read"})
	assert.Equal(t, []string{"agent", "bash", "read"}, result)
}

func TestMergeCoreAndOptional_DeduplicatesAgentLoop(t *testing.T) {
	result := mergeCoreAndOptional("agent", []string{"agent", "bash"})
	assert.Equal(t, []string{"agent", "bash"}, result)
}

func TestMergeCoreAndOptional_DeduplicatesOptExts(t *testing.T) {
	result := mergeCoreAndOptional("agent", []string{"bash", "bash", "read"})
	assert.Equal(t, []string{"agent", "bash", "read"}, result)
}

func TestMergeCoreAndOptional_EmptyOptExts(t *testing.T) {
	result := mergeCoreAndOptional("agent", []string{})
	assert.Equal(t, []string{"agent"}, result)
}

func TestMergeCoreAndOptional_FiltersAgentLoopFromOptExts(t *testing.T) {
	result := mergeCoreAndOptional("my-agent", []string{"bash", "my-agent", "read"})
	assert.Equal(t, []string{"my-agent", "bash", "read"}, result)
}

func TestMergeCoreAndOptional_FiltersDefaultLoopWhenCustomLoop(t *testing.T) {
	result := mergeCoreAndOptional("my-agent", []string{"bash", "agent", "read"})
	assert.Equal(t, []string{"my-agent", "bash", "read"}, result)
}

func TestMergeCoreAndOptional_KeepsDefaultLoopWhenDefaultLoop(t *testing.T) {
	result := mergeCoreAndOptional("agent", []string{"bash", "agent", "read"})
	assert.Equal(t, []string{"agent", "bash", "read"}, result)
}

type wireTestAuth struct {
	APIKey string `env:"WIRE_TEST_AUTH_KEY"`
}

func TestWire_SetsProviderAuthStatus(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetProviderRegistry()
	model.ResetAuthRegistry()

	sdk.RegisterProvider("wire-test-provider", func(_ sdk.Config, _ struct{}, _ wireTestAuth) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	bus := &BusMock{}
	_, err := WireExtensions(nil, bus, nil)
	require.NoError(t, err)

	assert.False(t, model.ProviderHasAuth("wire-test-provider"), "provider should not have auth without key")
}

func TestWire_SetsProviderAuthStatusWithEnvVar(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetProviderRegistry()
	model.ResetAuthRegistry()

	sdk.RegisterProvider("wire-test-provider-env", func(_ sdk.Config, _ struct{}, _ wireTestAuth) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	t.Setenv("WIRE_TEST_AUTH_KEY", "test-key")

	bus := &BusMock{}
	_, err := WireExtensions(nil, bus, nil)
	require.NoError(t, err)

	assert.True(t, model.ProviderHasAuth("wire-test-provider-env"), "provider should have auth when env var is set")
}

func TestWireWithCore_SetsProviderAuthStatus(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetProviderRegistry()
	model.ResetAuthRegistry()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	sdk.RegisterProvider("wire-test-provider-core", func(_ sdk.Config, _ struct{}, _ wireTestAuth) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	t.Setenv("WIRE_TEST_AUTH_KEY", "test-key")

	bus := &BusMock{}
	_, err := WireWithCore(coreCfg(), nil, bus, nil)
	require.NoError(t, err)

	assert.True(t, model.ProviderHasAuth("wire-test-provider-core"), "provider auth should be set during WireWithCore")
}

func TestWire_ProviderWithNoAuthStructHasNoAuth(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetProviderRegistry()
	model.ResetAuthRegistry()

	sdk.RegisterProvider("wire-test-no-auth", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	bus := &BusMock{}
	_, err := WireExtensions(nil, bus, nil)
	require.NoError(t, err, "wiring should succeed even when provider has no auth")

	assert.False(t, model.ProviderHasAuth("wire-test-no-auth"), "provider without auth struct should have no auth")
}

func TestResolveExtensions(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var created bool

	sdk.RegisterExtension("resolve-test", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		created = true
		return sdk.NewExtensionFunc("resolve-test", func(sdk.Bus) error { return nil }), nil
	})

	exts, err := resolveExtensions([]string{"resolve-test"}, nil, newRuntimeContext(&BusMock{}, nil))
	require.NoError(t, err)
	require.Len(t, exts, 1)
	assert.Equal(t, "resolve-test", exts[0].Name())
	assert.True(t, created)
}

func TestResolveExtensions_Missing(t *testing.T) {
	sdk.ResetExtensionRegistry()

	exts, err := resolveExtensions([]string{"missing"}, nil, newRuntimeContext(&BusMock{}, nil))
	require.NoError(t, err, "unregistered extension should be silently skipped")
	assert.Empty(t, exts)
}

func TestResolveExtensions_CleansUpOnError(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var closed bool

	sdk.RegisterExtension("good", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("good", func(sdk.Bus) error { return nil }, func() error {
			closed = true
			return nil
		}), nil
	})
	sdk.RegisterExtension("bad", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return nil, errors.New("init failed")
	})

	_, err := resolveExtensions([]string{"good", "bad"}, nil, newRuntimeContext(&BusMock{}, nil))
	require.Error(t, err)
	assert.True(t, closed, "resolved extension should be closed on error")
}

func TestSubscribeExtensions(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var subscribed bool

	sdk.RegisterExtension("sub-test", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("sub-test", func(bus sdk.Bus) error {
			subscribed = true
			return nil
		}), nil
	})

	bus := &BusMock{}
	exts, err := resolveExtensions([]string{"sub-test"}, nil, newRuntimeContext(bus, nil))
	require.NoError(t, err)

	require.NoError(t, subscribeExtensions(exts, bus))
	assert.True(t, subscribed)
}

func TestSubscribeExtensions_RollsBackOnError(t *testing.T) {
	sdk.ResetExtensionRegistry()

	var closed bool

	sdk.RegisterExtension("sub-ok", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("sub-ok", func(sdk.Bus) error { return nil }, func() error {
			closed = true
			return nil
		}), nil
	})
	sdk.RegisterExtension("sub-fail", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("sub-fail", func(sdk.Bus) error {
			return errors.New("subscribe failed")
		}, nil), nil
	})

	bus := &BusMock{}
	exts, err := resolveExtensions([]string{"sub-ok", "sub-fail"}, nil, newRuntimeContext(bus, nil))
	require.NoError(t, err)

	err = subscribeExtensions(exts, bus)
	require.Error(t, err)
	assert.True(t, closed, "previously subscribed extension should be closed on error")
}

func TestSetSingleTurnEnv_SetsAndRestores(t *testing.T) {
	// Ensure clean state.
	_ = os.Unsetenv("WEAVE_SINGLE_TURN")

	cleanup := setSingleTurnEnv(true)

	assert.Equal(t, "1", os.Getenv("WEAVE_SINGLE_TURN"))

	cleanup()

	assert.Empty(t, os.Getenv("WEAVE_SINGLE_TURN"))
}

func TestSetSingleTurnEnv_RestoresPreviousValue(t *testing.T) {
	t.Setenv("WEAVE_SINGLE_TURN", "old-value")

	cleanup := setSingleTurnEnv(true)

	assert.Equal(t, "1", os.Getenv("WEAVE_SINGLE_TURN"))

	cleanup()

	assert.Equal(t, "old-value", os.Getenv("WEAVE_SINGLE_TURN"))
}

func TestSetSingleTurnEnv_NoOpWhenFalse(t *testing.T) {
	t.Setenv("WEAVE_SINGLE_TURN", "existing")

	cleanup := setSingleTurnEnv(false)

	assert.Equal(t, "existing", os.Getenv("WEAVE_SINGLE_TURN"))

	cleanup()

	assert.Equal(t, "existing", os.Getenv("WEAVE_SINGLE_TURN"))
}

// ---- Session store test helpers ----

type projectDirConfig struct {
	sdk.NoopConfig
	projectDir string
}

func (c projectDirConfig) ProjectDir() string { return c.projectDir }

type mockSessionStore struct {
	listFunc func() ([]sdk.SessionInfo, error)
	loadFunc func(string) ([]sdk.Message, error)
}

func (m *mockSessionStore) ListSessions() ([]sdk.SessionInfo, error) {
	if m.listFunc != nil {
		return m.listFunc()
	}

	return nil, nil
}

func (m *mockSessionStore) LoadHistory(id string) ([]sdk.Message, error) {
	if m.loadFunc != nil {
		return m.loadFunc(id)
	}

	return nil, nil
}

type mockStoreExt struct {
	mockSessionStore
	name string
}

func (m *mockStoreExt) Name() string            { return m.name }
func (m *mockStoreExt) Subscribe(sdk.Bus) error { return nil }
func (m *mockStoreExt) Close() error            { return nil }

type runtimeStoreExt struct {
	mockSessionStore
}

func (m *runtimeStoreExt) Register(sdk.ExtensionContext) error { return nil }

func TestWireExtensions_SetsSessionStore(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetSessionStore()

	store := &mockStoreExt{name: "test-store"}

	sdk.RegisterExtension("test-store", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return store, nil
	})

	bus := &BusMock{}
	_, err := WireExtensions([]string{"test-store"}, bus, nil)
	require.NoError(t, err)

	got := sdk.GetSessionStore()
	require.NotNil(t, got)
	assert.Equal(t, store, got)
}

func TestWireExtensions_SetsRuntimeSessionStore(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetSessionStore()

	store := &runtimeStoreExt{}

	sdk.RegisterRuntimeExtension("runtime-store", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.RuntimeExtension, error) {
		return store, nil
	})

	bus := &BusMock{}
	_, err := WireExtensions([]string{"runtime-store"}, bus, nil)
	require.NoError(t, err)

	got := sdk.GetSessionStore()
	require.NotNil(t, got)
	assert.Equal(t, store, got)
}

func TestWireExtensions_NoSessionStore(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetSessionStore()

	sdk.RegisterExtension("no-store", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("no-store", func(sdk.Bus) error { return nil }), nil
	})

	bus := &BusMock{}
	_, err := WireExtensions([]string{"no-store"}, bus, nil)
	require.NoError(t, err)

	assert.Nil(t, sdk.GetSessionStore())
}

func TestResolveSession_NoStore(t *testing.T) {
	sdk.ResetSessionStore()

	bus := &BusMock{}
	_, _, err := resolveSession(true, "", bus, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no session store available")
}

func TestResolveSession_ContinueNoSessions(t *testing.T) {
	sdk.ResetSessionStore()

	store := &mockSessionStore{
		listFunc: func() ([]sdk.SessionInfo, error) {
			return nil, nil
		},
	}
	sdk.SetSessionStore(store)

	bus := &BusMock{}
	_, _, err := resolveSession(true, "", bus, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sessions found")
}

func TestResolveSession_ContinuePicksMostRecent(t *testing.T) {
	sdk.ResetSessionStore()

	cwd := continueCWD(nil)

	store := &mockSessionStore{
		listFunc: func() ([]sdk.SessionInfo, error) {
			return []sdk.SessionInfo{
				{ID: "older", CWD: cwd, UpdatedAt: time.Now().Add(-2 * time.Hour)},
				{ID: "newer", CWD: cwd, UpdatedAt: time.Now().Add(-1 * time.Hour)},
				{ID: "latest", CWD: cwd, UpdatedAt: time.Now()},
			}, nil
		},
		loadFunc: func(id string) ([]sdk.Message, error) {
			if id == "latest" {
				return []sdk.Message{
					{Role: sdk.RoleUser, Content: "hello"},
					{Role: sdk.RoleAssistant, Content: "hi"},
				}, nil
			}

			return nil, fmt.Errorf("unexpected id: %s", id)
		},
	}
	sdk.SetSessionStore(store)

	var publishedEvent sdk.Event

	bus := &BusMock{
		PublishFunc: func(e sdk.Event) {
			publishedEvent = e
		},
	}

	sessionID, messages, err := resolveSession(true, "", bus, sdk.FilePathConfig(""))
	require.NoError(t, err)
	assert.Equal(t, "latest", sessionID)
	require.Len(t, messages, 2)
	assert.Equal(t, "hello", messages[0].Content)
	assert.Equal(t, "hi", messages[1].Content)

	require.Equal(t, "session.resume", publishedEvent.Topic)
	payload, ok := publishedEvent.Payload.(sdk.SessionResumePayload)
	require.True(t, ok)
	assert.Equal(t, "latest", payload.SessionID)
	assert.Len(t, payload.Messages, 2)
}

func TestResolveSession_ContinuePicksMostRecentForCurrentCWD(t *testing.T) {
	sdk.ResetSessionStore()

	projectDir := filepath.Clean("/tmp/weave-project")

	store := &mockSessionStore{
		listFunc: func() ([]sdk.SessionInfo, error) {
			return []sdk.SessionInfo{
				{ID: "global-latest", CWD: "/tmp/other-project", UpdatedAt: time.Now()},
				{ID: "project-older", CWD: projectDir, UpdatedAt: time.Now().Add(-2 * time.Hour)},
				{ID: "project-latest", CWD: projectDir, UpdatedAt: time.Now().Add(-1 * time.Hour)},
			}, nil
		},
		loadFunc: func(id string) ([]sdk.Message, error) {
			if id == "project-latest" {
				return []sdk.Message{{Role: sdk.RoleUser, Content: "project"}}, nil
			}

			return nil, fmt.Errorf("unexpected id: %s", id)
		},
	}
	sdk.SetSessionStore(store)

	bus := &BusMock{}
	cfg := projectDirConfig{projectDir: projectDir}

	sessionID, messages, err := resolveSession(true, "", bus, cfg)
	require.NoError(t, err)
	assert.Equal(t, "project-latest", sessionID)
	require.Len(t, messages, 1)
	assert.Equal(t, "project", messages[0].Content)
}

func TestResolveSession_ContinueNoSessionsForCurrentCWD(t *testing.T) {
	sdk.ResetSessionStore()

	projectDir := filepath.Clean("/tmp/current-project")

	store := &mockSessionStore{
		listFunc: func() ([]sdk.SessionInfo, error) {
			return []sdk.SessionInfo{{ID: "other", CWD: "/tmp/other-project", UpdatedAt: time.Now()}}, nil
		},
	}
	sdk.SetSessionStore(store)

	bus := &BusMock{}
	cfg := projectDirConfig{projectDir: projectDir}

	_, _, err := resolveSession(true, "", bus, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sessions found for")
	assert.Contains(t, err.Error(), projectDir)
}

func TestResolveSession_ResumeValidID(t *testing.T) {
	sdk.ResetSessionStore()

	store := &mockSessionStore{
		loadFunc: func(id string) ([]sdk.Message, error) {
			if id == "abc123" {
				return []sdk.Message{
					{Role: sdk.RoleUser, Content: "test"},
				}, nil
			}

			return nil, errors.New("session not found")
		},
	}
	sdk.SetSessionStore(store)

	var publishedEvent sdk.Event

	bus := &BusMock{
		PublishFunc: func(e sdk.Event) {
			publishedEvent = e
		},
	}

	sessionID, messages, err := resolveSession(false, "abc123", bus, nil)
	require.NoError(t, err)
	assert.Equal(t, "abc123", sessionID)
	require.Len(t, messages, 1)

	require.Equal(t, "session.resume", publishedEvent.Topic)
	payload, ok := publishedEvent.Payload.(sdk.SessionResumePayload)
	require.True(t, ok)
	assert.Equal(t, "abc123", payload.SessionID)
}

func TestResolveSession_ResumeInvalidID(t *testing.T) {
	sdk.ResetSessionStore()

	store := &mockSessionStore{
		loadFunc: func(id string) ([]sdk.Message, error) {
			return nil, errors.New("session not found")
		},
	}
	sdk.SetSessionStore(store)

	bus := &BusMock{}
	_, _, err := resolveSession(false, "bad-id", bus, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load session bad-id")
}

func TestWireWithCore_ResumePublishesSessionEvent(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetSessionStore()
	sdk.ResetInitialSession()

	sdk.ResetBusSubscribers()
	defer sdk.ResetBusSubscribers()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	store := &mockStoreExt{name: "jsonl"}
	store.listFunc = func() ([]sdk.SessionInfo, error) {
		return []sdk.SessionInfo{{ID: "sess1", CWD: continueCWD(nil), UpdatedAt: time.Now()}}, nil
	}
	store.loadFunc = func(id string) ([]sdk.Message, error) {
		return []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}, nil
	}

	sdk.RegisterExtension("jsonl", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return store, nil
	})

	realBus := eventbus.New()
	defer realBus.Close()

	var (
		sessionReceived    atomic.Bool
		appStartedReceived atomic.Bool
	)

	realBus.On("session.resume", func(e sdk.Event) error {
		payload, ok := e.Payload.(sdk.SessionResumePayload)
		if ok && payload.SessionID == "sess1" {
			sessionReceived.Store(true)
		}

		return nil
	})
	realBus.On("app.started", func(e sdk.Event) error {
		appStartedReceived.Store(true)
		return nil
	})

	wired, err := WireWithCore(CoreWireConfig{AgentLoop: "agent", Continue: true}, []string{"jsonl"}, realBus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)

	require.Eventually(t, func() bool {
		return sessionReceived.Load() && appStartedReceived.Load()
	}, time.Second, 10*time.Millisecond, "both events should be published")
}

func TestWireWithCore_ResumeSetsInitialSession(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetSessionStore()
	sdk.ResetInitialSession()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	store := &mockStoreExt{name: "jsonl"}
	store.listFunc = func() ([]sdk.SessionInfo, error) {
		return []sdk.SessionInfo{{ID: "sess-initial", CWD: continueCWD(nil), UpdatedAt: time.Now()}}, nil
	}
	store.loadFunc = func(id string) ([]sdk.Message, error) {
		return []sdk.Message{{Role: sdk.RoleUser, Content: "restored"}}, nil
	}

	sdk.RegisterExtension("jsonl", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return store, nil
	})

	bus := eventbus.New()
	defer bus.Close()

	wired, err := WireWithCore(CoreWireConfig{AgentLoop: "agent", Continue: true}, []string{"jsonl"}, bus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)

	payload, ok := sdk.GetInitialSession()
	require.True(t, ok)
	assert.Equal(t, "sess-initial", payload.SessionID)
	require.Len(t, payload.Messages, 1)
	assert.Equal(t, "restored", payload.Messages[0].Content)
}

func TestWireWithCore_ResumeErrorHeadless(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetSessionStore()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	bus := &BusMock{}
	_, err := WireWithCore(CoreWireConfig{AgentLoop: "agent", Continue: true}, nil, bus, sdk.HeadlessConfig{Config: sdk.FilePathConfig(""), Headless: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session resume")
}

func TestWireWithCore_ResumeErrorTUI(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetSessionStore()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	bus := &BusMock{}
	wired, err := WireWithCore(CoreWireConfig{AgentLoop: "agent", Continue: true}, nil, bus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)
}

func TestWireWithCore_NoResumeWhenNotRequested(t *testing.T) {
	sdk.ResetExtensionRegistry()
	sdk.ResetSessionStore()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	bus := &BusMock{}
	wired, err := WireWithCore(coreCfg(), nil, bus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)
}
