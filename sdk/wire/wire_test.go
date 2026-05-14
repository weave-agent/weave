package wire

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventbus "weave/bus"
	"weave/sdk"
	"weave/sdk/model"
)

func coreCfg() CoreWireConfig {
	return CoreWireConfig{AgentLoop: "agent"}
}

func TestWire_NoExtensions(t *testing.T) {
	sdk.ResetRegistry()

	mockBus := &BusMock{}

	wired, err := Wire(nil, mockBus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)
}

func TestWire_EmptyExtensions(t *testing.T) {
	sdk.ResetRegistry()

	mockBus := &BusMock{}

	wired, err := Wire([]string{}, mockBus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)
}

func TestWire_SubscribesAllExtensions(t *testing.T) {
	sdk.ResetRegistry()

	var subscribed atomic.Int32

	sdk.RegisterExtension("ext-a", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-a", func(bus sdk.Bus) error {
			subscribed.Add(1)
			return nil
		}), nil
	})
	sdk.RegisterExtension("ext-b", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-b", func(bus sdk.Bus) error {
			subscribed.Add(1)
			return nil
		}), nil
	})

	mockBus := &BusMock{}

	wired, err := Wire([]string{"ext-a", "ext-b"}, mockBus, nil)
	require.NoError(t, err)

	assert.Equal(t, int32(2), subscribed.Load())

	_ = wired
}

func TestWire_MissingExtension(t *testing.T) {
	sdk.ResetRegistry()

	bus := &BusMock{}

	_, err := Wire([]string{"nonexistent"}, bus, nil)
	require.Error(t, err)
	assert.Equal(t, "wire: extension \"nonexistent\" not registered", err.Error())
}

func TestWire_ReceiveBusInSubscribe(t *testing.T) {
	sdk.ResetRegistry()

	var receivedBus sdk.Bus

	sdk.RegisterExtension("ext-c", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-c", func(bus sdk.Bus) error {
			receivedBus = bus
			return nil
		}), nil
	})

	bus := &BusMock{}

	_, err := Wire([]string{"ext-c"}, bus, nil)
	require.NoError(t, err)
	require.NotNil(t, receivedBus)
}

func TestWire_PartialMissing(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("good", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("good", func(bus sdk.Bus) error { return nil }), nil
	})

	bus := &BusMock{}

	_, err := Wire([]string{"good", "missing"}, bus, nil)
	require.Error(t, err)
}

func TestWire_SkipsUIExtension(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetUIExtensionRegistry()

	sdk.RegisterUIExtension(stubUIExt{name: "diff-viewer"})

	bus := &BusMock{}

	_, err := Wire([]string{"diff-viewer"}, bus, nil)
	require.NoError(t, err, "UI extension should be silently skipped")
}

type stubUIExt struct{ name string }

func (s stubUIExt) Name() string      { return s.name }
func (s stubUIExt) Register(_ sdk.UI) {}

func TestWire_PassesConfigToFactory(t *testing.T) {
	sdk.ResetRegistry()

	var receivedCfg sdk.Config

	sdk.RegisterExtension("cfg-ext", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
		receivedCfg = cfg
		return sdk.NewExtensionFunc("cfg-ext", func(sdk.Bus) error { return nil }), nil
	})

	cfg := sdk.FilePathConfig("/test/.weave/settings.json")
	bus := &BusMock{}

	_, err := Wire([]string{"cfg-ext"}, bus, cfg)
	require.NoError(t, err)
	require.NotNil(t, receivedCfg)
	assert.Equal(t, "/test/.weave/settings.json", receivedCfg.FilePath())
}

func TestWire_FactoryError(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("bad", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return nil, errors.New("init failed")
	})

	bus := &BusMock{}

	_, err := Wire([]string{"bad"}, bus, nil)
	require.Error(t, err)
}

func TestWired_Close(t *testing.T) {
	sdk.ResetRegistry()

	var closed atomic.Int32

	sdk.RegisterExtension("a", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("a", func(sdk.Bus) error { return nil }, func() error {
			closed.Add(1)
			return nil
		}), nil
	})
	sdk.RegisterExtension("b", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("b", func(sdk.Bus) error { return nil }, func() error {
			closed.Add(1)
			return nil
		}), nil
	})

	bus := &BusMock{}

	wired, err := Wire([]string{"a", "b"}, bus, nil)
	require.NoError(t, err, "Wire")

	require.NoError(t, wired.Close(), "Close")
	assert.Equal(t, int32(2), closed.Load())
}

func TestWired_CloseReverseOrder(t *testing.T) {
	sdk.ResetRegistry()

	var order []string

	sdk.RegisterExtension("first", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("first", func(sdk.Bus) error { return nil }, func() error {
			order = append(order, "first")
			return nil
		}), nil
	})
	sdk.RegisterExtension("second", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("second", func(sdk.Bus) error { return nil }, func() error {
			order = append(order, "second")
			return nil
		}), nil
	})

	bus := &BusMock{}

	wired, err := Wire([]string{"first", "second"}, bus, nil)
	require.NoError(t, err, "Wire")

	require.NoError(t, wired.Close(), "Close")
	assert.Equal(t, []string{"second", "first"}, order)
}

func TestWireWithCore_MergesCoreAndOptional(t *testing.T) {
	sdk.ResetRegistry()

	var names []string

	reg := func(n string) {
		sdk.RegisterExtension(n, func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
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
	sdk.ResetRegistry()

	var subscribed atomic.Int32

	reg := func(n string) {
		sdk.RegisterExtension(n, func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
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
	sdk.ResetRegistry()

	var names []string

	reg := func(n string) {
		sdk.RegisterExtension(n, func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
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
	sdk.ResetRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{}, nil, bus, nil)
	require.Error(t, err)
	assert.Equal(t, "wire: agent-loop is required", err.Error())
}

func TestWireWithCore_NoProviderRequired(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "agent"}, nil, bus, nil)
	require.NoError(t, err)
}

func TestWireWithCore_FactoryErrorRollback(t *testing.T) {
	sdk.ResetRegistry()

	var closed atomic.Int32

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("agent", func(sdk.Bus) error { return nil }, func() error {
			closed.Add(1)
			return nil
		}), nil
	})
	sdk.RegisterExtension("bad", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return nil, errors.New("init failed")
	})

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg(), []string{"bad"}, bus, nil)
	require.Error(t, err)

	assert.Equal(t, int32(1), closed.Load())
}

func TestWired_CloseErrorAggregation(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("a", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("a", func(sdk.Bus) error { return nil }, func() error {
			return errors.New("a failed")
		}), nil
	})
	sdk.RegisterExtension("b", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("b", func(sdk.Bus) error { return nil }, func() error {
			return errors.New("b failed")
		}), nil
	})

	bus := &BusMock{}

	wired, err := Wire([]string{"a", "b"}, bus, nil)
	require.NoError(t, err, "Wire")

	err = wired.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a failed")
	assert.Contains(t, err.Error(), "b failed")
}

func TestWireWithCore_PassesConfigToFactories(t *testing.T) {
	sdk.ResetRegistry()

	var receivedCfg sdk.Config

	sdk.RegisterExtension("agent", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
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
	sdk.ResetRegistry()

	sdk.RegisterExtension("on-ext", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("on-ext", func(bus sdk.Bus) error {
			bus.On("test.topic", func(e sdk.Event) error { return nil })
			return nil
		}), nil
	})

	bus := &BusMock{}

	_, err := Wire([]string{"on-ext"}, bus, nil)
	require.NoError(t, err)

	onCalls := bus.OnCalls()
	require.Len(t, onCalls, 1)
	assert.Equal(t, "test.topic", onCalls[0].Topic)
}

func TestWire_ExtensionCallsBusOnAll(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("onall-ext", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("onall-ext", func(bus sdk.Bus) error {
			bus.OnAll(func(e sdk.Event) error { return nil })
			return nil
		}), nil
	})

	bus := &BusMock{}

	_, err := Wire([]string{"onall-ext"}, bus, nil)
	require.NoError(t, err)

	onAllCalls := bus.OnAllCalls()
	require.Len(t, onAllCalls, 1)
}

func TestWire_MultipleExtensionsRegisterHandlers(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("ext-1", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-1", func(bus sdk.Bus) error {
			bus.On("topic.a", func(e sdk.Event) error { return nil })
			bus.On("topic.b", func(e sdk.Event) error { return nil })

			return nil
		}), nil
	})
	sdk.RegisterExtension("ext-2", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-2", func(bus sdk.Bus) error {
			bus.OnAll(func(e sdk.Event) error { return nil })
			return nil
		}), nil
	})

	bus := &BusMock{}

	_, err := Wire([]string{"ext-1", "ext-2"}, bus, nil)
	require.NoError(t, err)

	onCalls := bus.OnCalls()
	require.Len(t, onCalls, 2)
	assert.Equal(t, "topic.a", onCalls[0].Topic)
	assert.Equal(t, "topic.b", onCalls[1].Topic)

	onAllCalls := bus.OnAllCalls()
	require.Len(t, onAllCalls, 1)
}

func TestWireWithCore_PublishesAppStarted(t *testing.T) {
	sdk.ResetRegistry()

	sdk.ResetAppStartedHandlers()
	defer sdk.ResetAppStartedHandlers()

	var handlerCalled atomic.Bool

	sdk.OnAppStarted(func(bus sdk.Bus, cfg sdk.Config) {
		handlerCalled.Store(true)
	})

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	realBus := eventbus.New()
	defer realBus.Close()

	_, err := WireWithCore(coreCfg(), nil, realBus, sdk.FilePathConfig(""))
	require.NoError(t, err, "WireWithCore")

	// Wait for the async publish + handler to run.
	require.Eventually(t, handlerCalled.Load, 100*time.Millisecond, 5*time.Millisecond, "app.started handler should be called")
}

func TestWireWithCore_AppStartedNotCalledWhenHeadless(t *testing.T) {
	sdk.ResetRegistry()

	sdk.ResetAppStartedHandlers()
	defer sdk.ResetAppStartedHandlers()

	var handlerCalled atomic.Bool

	sdk.OnAppStarted(func(bus sdk.Bus, cfg sdk.Config) {
		if cfg != nil && cfg.IsHeadless() {
			return
		}

		handlerCalled.Store(true)
	})

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("agent", func(sdk.Bus) error { return nil }), nil
	})

	realBus := eventbus.New()
	defer realBus.Close()

	headlessCfg := sdk.HeadlessConfig{Config: sdk.FilePathConfig(""), Headless: true}
	_, err := WireWithCore(coreCfg(), nil, realBus, headlessCfg)
	require.NoError(t, err, "WireWithCore")

	// Give the async handler a chance to run, then verify it was skipped.
	assert.Never(t, handlerCalled.Load, 50*time.Millisecond, 5*time.Millisecond, "app.started handler should NOT be called when headless")
}

func TestWireWithCore_ExtensionUsesBusOn(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
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
	sdk.ResetRegistry()

	var subscribeCalled bool

	sdk.RegisterExtension("noop", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
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

	wired, err := Wire([]string{"noop"}, realBus, nil)
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

func TestMergeCoreAndOptional_FiltersLegacyLoop(t *testing.T) {
	result := mergeCoreAndOptional("agent", []string{"bash", "loop", "read"})
	assert.Equal(t, []string{"agent", "bash", "read"}, result)
}

func TestWireWithCore_MapsLoopToAgent(t *testing.T) {
	sdk.ResetRegistry()

	var names []string

	reg := func(n string) {
		sdk.RegisterExtension(n, func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
			return sdk.NewExtensionFunc(n, func(sdk.Bus) error {
				names = append(names, n)
				return nil
			}), nil
		})
	}
	reg("agent")

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop"}, nil, bus, nil)
	require.NoError(t, err, "WireWithCore")

	require.Len(t, names, 1)
	assert.Equal(t, "agent", names[0])
}

func TestWireWithCore_MapsLoopToAgentAndFiltersLoopFromOptExts(t *testing.T) {
	sdk.ResetRegistry()

	var names []string

	reg := func(n string) {
		sdk.RegisterExtension(n, func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
			return sdk.NewExtensionFunc(n, func(sdk.Bus) error {
				names = append(names, n)
				return nil
			}), nil
		})
	}
	reg("agent")
	reg("loop")
	reg("bash")

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop"}, []string{"loop", "bash"}, bus, nil)
	require.NoError(t, err, "WireWithCore")

	require.Len(t, names, 2)
	assert.Contains(t, names, "agent")
	assert.Contains(t, names, "bash")
	assert.NotContains(t, names, "loop")
}

type wireTestAuth struct {
	APIKey string `env:"WIRE_TEST_AUTH_KEY"`
}

func TestWire_SetsProviderAuthStatus(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()
	model.ResetAuthRegistry()

	sdk.RegisterProvider("wire-test-provider", func(_ sdk.Config, _ struct{}, _ wireTestAuth) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	bus := &BusMock{}
	_, err := Wire(nil, bus, nil)
	require.NoError(t, err)

	assert.False(t, model.ProviderHasAuth("wire-test-provider"), "provider should not have auth without key")
}

func TestWire_SetsProviderAuthStatusWithEnvVar(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()
	model.ResetAuthRegistry()

	sdk.RegisterProvider("wire-test-provider-env", func(_ sdk.Config, _ struct{}, _ wireTestAuth) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	t.Setenv("WIRE_TEST_AUTH_KEY", "test-key")

	bus := &BusMock{}
	_, err := Wire(nil, bus, nil)
	require.NoError(t, err)

	assert.True(t, model.ProviderHasAuth("wire-test-provider-env"), "provider should have auth when env var is set")
}

func TestWireWithCore_SetsProviderAuthStatus(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()
	model.ResetAuthRegistry()

	sdk.RegisterExtension("agent", func(_ sdk.Config, _ struct{}) (sdk.Extension, error) {
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
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()
	model.ResetAuthRegistry()

	sdk.RegisterProvider("wire-test-no-auth", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	bus := &BusMock{}
	_, err := Wire(nil, bus, nil)
	require.NoError(t, err, "wiring should succeed even when provider has no auth")

	assert.False(t, model.ProviderHasAuth("wire-test-no-auth"), "provider without auth struct should have no auth")
}

var _ = strings.TrimSpace
