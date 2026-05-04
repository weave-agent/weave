package sdk

import (
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func coreCfg(providers ...string) CoreWireConfig {
	return CoreWireConfig{AgentLoop: "loop", Providers: providers}
}

// --- Wire (legacy, no core) ---

func TestWire_NoExtensions(t *testing.T) {
	ResetRegistry()

	bus := &BusMock{}

	wired, err := Wire(nil, bus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)
}

func TestWire_EmptyExtensions(t *testing.T) {
	ResetRegistry()

	bus := &BusMock{}

	wired, err := Wire([]string{}, bus, nil)
	require.NoError(t, err)
	require.NotNil(t, wired)
}

func TestWire_SubscribesAllExtensions(t *testing.T) {
	ResetRegistry()

	var subscribed atomic.Int32

	RegisterExtension("ext-a", func(Config) (Extension, error) {
		return NewExtensionFunc("ext-a", func(bus Bus) {
			subscribed.Add(1)
		}), nil
	})
	RegisterExtension("ext-b", func(Config) (Extension, error) {
		return NewExtensionFunc("ext-b", func(bus Bus) {
			subscribed.Add(1)
		}), nil
	})

	bus := &BusMock{}

	wired, err := Wire([]string{"ext-a", "ext-b"}, bus, nil)
	require.NoError(t, err)

	assert.Equal(t, int32(2), subscribed.Load())

	_ = wired
}

func TestWire_MissingExtension(t *testing.T) {
	ResetRegistry()

	bus := &BusMock{}

	_, err := Wire([]string{"nonexistent"}, bus, nil)
	require.Error(t, err)
	assert.Equal(t, "wire: extension \"nonexistent\" not registered", err.Error())
}

func TestWire_ReceiveBusInSubscribe(t *testing.T) {
	ResetRegistry()

	var receivedBus Bus

	RegisterExtension("ext-c", func(Config) (Extension, error) {
		return NewExtensionFunc("ext-c", func(bus Bus) {
			receivedBus = bus
		}), nil
	})

	bus := &BusMock{}

	_, err := Wire([]string{"ext-c"}, bus, nil)
	require.NoError(t, err)
	require.NotNil(t, receivedBus)
}

func TestWire_PartialMissing(t *testing.T) {
	ResetRegistry()

	RegisterExtension("good", func(Config) (Extension, error) {
		return NewExtensionFunc("good", func(bus Bus) {}), nil
	})

	bus := &BusMock{}

	_, err := Wire([]string{"good", "missing"}, bus, nil)
	require.Error(t, err)
}

func TestWire_PassesConfigToFactory(t *testing.T) {
	ResetRegistry()

	var receivedCfg Config

	RegisterExtension("cfg-ext", func(cfg Config) (Extension, error) {
		receivedCfg = cfg
		return NewExtensionFunc("cfg-ext", func(Bus) {}), nil
	})

	cfg := FilePathConfig("/test/.weave.yaml")
	bus := &BusMock{}

	_, err := Wire([]string{"cfg-ext"}, bus, cfg)
	require.NoError(t, err)
	require.NotNil(t, receivedCfg)
	assert.Equal(t, "/test/.weave.yaml", receivedCfg.FilePath())
}

func TestWire_FactoryError(t *testing.T) {
	ResetRegistry()

	RegisterExtension("bad", func(Config) (Extension, error) {
		return nil, errors.New("init failed")
	})

	bus := &BusMock{}

	_, err := Wire([]string{"bad"}, bus, nil)
	require.Error(t, err)
}

func TestWired_Close(t *testing.T) {
	ResetRegistry()

	var closed atomic.Int32

	RegisterExtension("a", func(Config) (Extension, error) {
		return NewExtensionFuncWithClose("a", func(Bus) {}, func() error {
			closed.Add(1)
			return nil
		}), nil
	})
	RegisterExtension("b", func(Config) (Extension, error) {
		return NewExtensionFuncWithClose("b", func(Bus) {}, func() error {
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
	ResetRegistry()

	var order []string

	RegisterExtension("first", func(Config) (Extension, error) {
		return NewExtensionFuncWithClose("first", func(Bus) {}, func() error {
			order = append(order, "first")
			return nil
		}), nil
	})
	RegisterExtension("second", func(Config) (Extension, error) {
		return NewExtensionFuncWithClose("second", func(Bus) {}, func() error {
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

// --- WireWithCore: merging ---

func TestWireWithCore_MergesCoreAndOptional(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	var names []string

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	reg := func(n string) {
		RegisterExtension(n, func(Config) (Extension, error) {
			return NewExtensionFunc(n, func(Bus) {
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
	ResetRegistry()
	ResetProviderRegistry()

	var subscribed atomic.Int32

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	reg := func(n string) {
		RegisterExtension(n, func(Config) (Extension, error) {
			return NewExtensionFunc(n, func(Bus) {
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
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	var names []string

	reg := func(n string) {
		RegisterExtension(n, func(Config) (Extension, error) {
			return NewExtensionFunc(n, func(Bus) {
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

// --- WireWithCore: validation errors ---

func TestWireWithCore_ErrMissingAgentLoop(t *testing.T) {
	ResetRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{Providers: []string{"anthropic"}}, nil, bus, nil)
	require.Error(t, err)
	assert.Equal(t, "wire: agent-loop is required", err.Error())
}

func TestWireWithCore_ErrNoProvider(t *testing.T) {
	ResetRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop"}, nil, bus, nil)
	require.Error(t, err)
	assert.Equal(t, "wire: at least one provider is required", err.Error())
}

func TestWireWithCore_ErrEmptyProviders(t *testing.T) {
	ResetRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{}}, nil, bus, nil)
	require.Error(t, err)
}

func TestWireWithCore_ErrDuplicateProviders(t *testing.T) {
	ResetRegistry()

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{"anthropic", "anthropic"}}, nil, bus, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate provider")
}

func TestWireWithCore_FactoryErrorRollback(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	var closed atomic.Int32

	RegisterExtension("loop", func(Config) (Extension, error) {
		return NewExtensionFuncWithClose("loop", func(Bus) {}, func() error {
			closed.Add(1)
			return nil
		}), nil
	})
	RegisterExtension("bad", func(Config) (Extension, error) {
		return nil, errors.New("init failed")
	})

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg("anthropic"), []string{"bad"}, bus, nil)
	require.Error(t, err)

	assert.Equal(t, int32(1), closed.Load())
}

func TestWired_CloseErrorAggregation(t *testing.T) {
	ResetRegistry()

	RegisterExtension("a", func(Config) (Extension, error) {
		return NewExtensionFuncWithClose("a", func(Bus) {}, func() error {
			return errors.New("a failed")
		}), nil
	})
	RegisterExtension("b", func(Config) (Extension, error) {
		return NewExtensionFuncWithClose("b", func(Bus) {}, func() error {
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
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("openai", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	RegisterExtension("loop", func(Config) (Extension, error) {
		return NewExtensionFunc("loop", func(Bus) {}), nil
	})

	t.Setenv("WEAVE_PROVIDER", "")

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{"openai"}}, nil, bus, nil)
	require.NoError(t, err, "WireWithCore")

	assert.Equal(t, "openai", os.Getenv("WEAVE_PROVIDER"))
}

func TestWireWithCore_DoesNotOverrideExistingProviderEnv(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})
	RegisterProvider("openai", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	RegisterExtension("loop", func(Config) (Extension, error) {
		return NewExtensionFunc("loop", func(Bus) {}), nil
	})

	t.Setenv("WEAVE_PROVIDER", "anthropic")

	bus := &BusMock{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{"openai"}}, nil, bus, nil)
	require.NoError(t, err, "WireWithCore")

	assert.Equal(t, "anthropic", os.Getenv("WEAVE_PROVIDER"))
}

func TestWireWithCore_PassesConfigToFactories(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	var receivedCfg Config

	RegisterExtension("loop", func(cfg Config) (Extension, error) {
		receivedCfg = cfg
		return NewExtensionFunc("loop", func(Bus) {}), nil
	})

	cfg := FilePathConfig("/test/.weave.yaml")
	bus := &BusMock{}

	_, err := WireWithCore(coreCfg("anthropic"), nil, bus, cfg)
	require.NoError(t, err, "WireWithCore")

	require.NotNil(t, receivedCfg)
	assert.Equal(t, "/test/.weave.yaml", receivedCfg.FilePath())
}

// --- Wire with callback-based Bus mock (On/Off/OnAll) ---

func TestWire_ExtensionCallsBusOn(t *testing.T) {
	ResetRegistry()

	RegisterExtension("on-ext", func(Config) (Extension, error) {
		return NewExtensionFunc("on-ext", func(bus Bus) {
			bus.On("test.topic", func(e Event) error { return nil })
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
	ResetRegistry()

	RegisterExtension("onall-ext", func(Config) (Extension, error) {
		return NewExtensionFunc("onall-ext", func(bus Bus) {
			bus.OnAll(func(e Event) error { return nil })
		}), nil
	})

	bus := &BusMock{}

	_, err := Wire([]string{"onall-ext"}, bus, nil)
	require.NoError(t, err)

	onAllCalls := bus.OnAllCalls()
	require.Len(t, onAllCalls, 1)
}

func TestWire_MultipleExtensionsRegisterHandlers(t *testing.T) {
	ResetRegistry()

	RegisterExtension("ext-1", func(Config) (Extension, error) {
		return NewExtensionFunc("ext-1", func(bus Bus) {
			bus.On("topic.a", func(e Event) error { return nil })
			bus.On("topic.b", func(e Event) error { return nil })
		}), nil
	})
	RegisterExtension("ext-2", func(Config) (Extension, error) {
		return NewExtensionFunc("ext-2", func(bus Bus) {
			bus.OnAll(func(e Event) error { return nil })
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
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &ProviderMock{}, nil
	})

	RegisterExtension("loop", func(Config) (Extension, error) {
		return NewExtensionFunc("loop", func(bus Bus) {
			bus.On("agent.prompt", func(e Event) error { return nil })
		}), nil
	})

	bus := &BusMock{}

	_, err := WireWithCore(coreCfg("anthropic"), nil, bus, nil)
	require.NoError(t, err)

	onCalls := bus.OnCalls()
	require.Len(t, onCalls, 1)
	assert.Equal(t, "agent.prompt", onCalls[0].Topic)
}

// Suppress unused import warning.
var _ = strings.TrimSpace
