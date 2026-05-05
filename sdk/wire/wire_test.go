package wire

import (
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventbus "weave/bus"
	"weave/sdk"
)

func coreCfg(providers ...string) CoreWireConfig {
	return CoreWireConfig{AgentLoop: "loop", Providers: providers}
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

	sdk.RegisterExtension("ext-a", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-a", func(bus sdk.Bus) {
			subscribed.Add(1)
		}), nil
	})
	sdk.RegisterExtension("ext-b", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-b", func(bus sdk.Bus) {
			subscribed.Add(1)
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

	sdk.RegisterExtension("ext-c", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-c", func(bus sdk.Bus) {
			receivedBus = bus
		}), nil
	})

	bus := &BusMock{}

	_, err := Wire([]string{"ext-c"}, bus, nil)
	require.NoError(t, err)
	require.NotNil(t, receivedBus)
}

func TestWire_PartialMissing(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("good", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("good", func(bus sdk.Bus) {}), nil
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

	sdk.RegisterExtension("cfg-ext", func(cfg sdk.Config) (sdk.Extension, error) {
		receivedCfg = cfg
		return sdk.NewExtensionFunc("cfg-ext", func(sdk.Bus) {}), nil
	})

	cfg := sdk.FilePathConfig("/test/.weave.yaml")
	bus := &BusMock{}

	_, err := Wire([]string{"cfg-ext"}, bus, cfg)
	require.NoError(t, err)
	require.NotNil(t, receivedCfg)
	assert.Equal(t, "/test/.weave.yaml", receivedCfg.FilePath())
}

func TestWire_FactoryError(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("bad", func(sdk.Config) (sdk.Extension, error) {
		return nil, errors.New("init failed")
	})

	bus := &BusMock{}

	_, err := Wire([]string{"bad"}, bus, nil)
	require.Error(t, err)
}

func TestWired_Close(t *testing.T) {
	sdk.ResetRegistry()

	var closed atomic.Int32

	sdk.RegisterExtension("a", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("a", func(sdk.Bus) {}, func() error {
			closed.Add(1)
			return nil
		}), nil
	})
	sdk.RegisterExtension("b", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("b", func(sdk.Bus) {}, func() error {
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

	sdk.RegisterExtension("first", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("first", func(sdk.Bus) {}, func() error {
			order = append(order, "first")
			return nil
		}), nil
	})
	sdk.RegisterExtension("second", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("second", func(sdk.Bus) {}, func() error {
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
	sdk.ResetProviderRegistry()

	var names []string

	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	reg := func(n string) {
		sdk.RegisterExtension(n, func(sdk.Config) (sdk.Extension, error) {
			return sdk.NewExtensionFunc(n, func(sdk.Bus) {
				names = append(names, n)
			}), nil
		})
	}
	reg("loop")
	reg("bash-tool")
	reg("file-tool")

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg("anthropic"), []string{"bash-tool", "file-tool"}, bus, nil)
	require.NoError(t, err, "WireWithCore")

	want := []string{"loop", "bash-tool", "file-tool"}
	require.Len(t, names, len(want))

	for i, n := range want {
		assert.Equal(t, n, names[i])
	}
}

func TestWireWithCore_Deduplicates(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()

	var subscribed atomic.Int32

	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	reg := func(n string) {
		sdk.RegisterExtension(n, func(sdk.Config) (sdk.Extension, error) {
			return sdk.NewExtensionFunc(n, func(sdk.Bus) {
				subscribed.Add(1)
			}), nil
		})
	}
	reg("loop")
	reg("bash-tool")

	bus := &BusMock{}

	_, err := WireWithCore(
		coreCfg("anthropic"),
		[]string{"bash-tool", "loop"},
		bus,
		nil,
	)
	require.NoError(t, err, "WireWithCore")

	assert.Equal(t, int32(2), subscribed.Load())
}

func TestWireWithCore_CoreOnly(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()

	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	var names []string

	reg := func(n string) {
		sdk.RegisterExtension(n, func(sdk.Config) (sdk.Extension, error) {
			return sdk.NewExtensionFunc(n, func(sdk.Bus) {
				names = append(names, n)
			}), nil
		})
	}
	reg("loop")

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg("anthropic"), nil, bus, nil)
	require.NoError(t, err, "WireWithCore")

	require.Len(t, names, 1)
	assert.Equal(t, "loop", names[0])
}

func TestWireWithCore_ErrMissingAgentLoop(t *testing.T) {
	sdk.ResetRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{Providers: []string{"anthropic"}}, nil, bus, nil)
	require.Error(t, err)
	assert.Equal(t, "wire: agent-loop is required", err.Error())
}

func TestWireWithCore_ErrNoProvider(t *testing.T) {
	sdk.ResetRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop"}, nil, bus, nil)
	require.Error(t, err)
	assert.Equal(t, "wire: at least one provider is required", err.Error())
}

func TestWireWithCore_ErrEmptyProviders(t *testing.T) {
	sdk.ResetRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{}}, nil, bus, nil)
	require.Error(t, err)
}

func TestWireWithCore_ErrDuplicateProviders(t *testing.T) {
	sdk.ResetRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{"anthropic", "anthropic"}}, nil, bus, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate provider")
}

func TestWireWithCore_FactoryErrorRollback(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()

	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	var closed atomic.Int32

	sdk.RegisterExtension("loop", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("loop", func(sdk.Bus) {}, func() error {
			closed.Add(1)
			return nil
		}), nil
	})
	sdk.RegisterExtension("bad", func(sdk.Config) (sdk.Extension, error) {
		return nil, errors.New("init failed")
	})

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg("anthropic"), []string{"bad"}, bus, nil)
	require.Error(t, err)

	assert.Equal(t, int32(1), closed.Load())
}

func TestWired_CloseErrorAggregation(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("a", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("a", func(sdk.Bus) {}, func() error {
			return errors.New("a failed")
		}), nil
	})
	sdk.RegisterExtension("b", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFuncWithClose("b", func(sdk.Bus) {}, func() error {
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

func TestWireWithCore_SyncsProviderEnv(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()

	sdk.RegisterProvider("openai", func(sdk.Config) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	var capturedProvider string

	sdk.RegisterExtension("loop", func(sdk.Config) (sdk.Extension, error) {
		capturedProvider = os.Getenv("WEAVE_PROVIDER")
		return sdk.NewExtensionFunc("loop", func(sdk.Bus) {}), nil
	})

	t.Setenv("WEAVE_PROVIDER", "")

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{"openai"}}, nil, bus, nil)
	require.NoError(t, err, "WireWithCore")

	assert.Equal(t, "openai", capturedProvider)
	assert.Empty(t, os.Getenv("WEAVE_PROVIDER"))
}

func TestWireWithCore_DoesNotOverrideExistingProviderEnv(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()

	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})
	sdk.RegisterProvider("openai", func(sdk.Config) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	var capturedProvider string

	sdk.RegisterExtension("loop", func(sdk.Config) (sdk.Extension, error) {
		capturedProvider = os.Getenv("WEAVE_PROVIDER")
		return sdk.NewExtensionFunc("loop", func(sdk.Bus) {}), nil
	})

	t.Setenv("WEAVE_PROVIDER", "anthropic")

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{"openai"}}, nil, bus, nil)
	require.NoError(t, err, "WireWithCore")

	assert.Equal(t, "anthropic", capturedProvider)
	assert.Equal(t, "anthropic", os.Getenv("WEAVE_PROVIDER"))
}

func TestWireWithCore_PassesConfigToFactories(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()

	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	var receivedCfg sdk.Config

	sdk.RegisterExtension("loop", func(cfg sdk.Config) (sdk.Extension, error) {
		receivedCfg = cfg
		return sdk.NewExtensionFunc("loop", func(sdk.Bus) {}), nil
	})

	cfg := sdk.FilePathConfig("/test/.weave.yaml")
	bus := &BusMock{}

	_, err := WireWithCore(coreCfg("anthropic"), nil, bus, cfg)
	require.NoError(t, err, "WireWithCore")

	require.NotNil(t, receivedCfg)
	assert.Equal(t, "/test/.weave.yaml", receivedCfg.FilePath())
}

func TestWire_ExtensionCallsBusOn(t *testing.T) {
	sdk.ResetRegistry()

	sdk.RegisterExtension("on-ext", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("on-ext", func(bus sdk.Bus) {
			bus.On("test.topic", func(e sdk.Event) error { return nil })
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

	sdk.RegisterExtension("onall-ext", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("onall-ext", func(bus sdk.Bus) {
			bus.OnAll(func(e sdk.Event) error { return nil })
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

	sdk.RegisterExtension("ext-1", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-1", func(bus sdk.Bus) {
			bus.On("topic.a", func(e sdk.Event) error { return nil })
			bus.On("topic.b", func(e sdk.Event) error { return nil })
		}), nil
	})
	sdk.RegisterExtension("ext-2", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("ext-2", func(bus sdk.Bus) {
			bus.OnAll(func(e sdk.Event) error { return nil })
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

func TestWireWithCore_ExtensionUsesBusOn(t *testing.T) {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()

	sdk.RegisterProvider("anthropic", func(sdk.Config) (sdk.Provider, error) {
		return &ProviderMock{}, nil
	})

	sdk.RegisterExtension("loop", func(sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("loop", func(bus sdk.Bus) {
			bus.On("agent.prompt", func(e sdk.Event) error { return nil })
		}), nil
	})

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg("anthropic"), nil, bus, nil)
	require.NoError(t, err)

	onCalls := bus.OnCalls()
	require.Len(t, onCalls, 1)
	assert.Equal(t, "agent.prompt", onCalls[0].Topic)
}

func TestWire_WireSubscribesExtensionsInProcess(t *testing.T) {
	sdk.ResetRegistry()

	var subscribeCalled bool

	sdk.RegisterExtension("noop", func(cfg sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) {
			subscribeCalled = true

			b.Publish(sdk.NewEvent("noop.subscribed", "hello"))
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

var _ = strings.TrimSpace
