package sdk

import (
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// --- helpers ---

func coreCfg(providers ...string) CoreWireConfig {
	return CoreWireConfig{AgentLoop: "loop", Providers: providers}
}

// --- Wire (legacy, no core) ---

func TestWire_NoExtensions(t *testing.T) {
	ResetRegistry()

	bus := &mockBus{}

	wired, err := Wire(nil, bus, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if wired == nil {
		t.Fatal("expected non-nil Wired")
	}
}

func TestWire_EmptyExtensions(t *testing.T) {
	ResetRegistry()

	bus := &mockBus{}

	wired, err := Wire([]string{}, bus, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if wired == nil {
		t.Fatal("expected non-nil Wired")
	}
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

	bus := &mockBus{}

	wired, err := Wire([]string{"ext-a", "ext-b"}, bus, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := subscribed.Load(); got != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", got)
	}

	_ = wired
}

func TestWire_MissingExtension(t *testing.T) {
	ResetRegistry()

	bus := &mockBus{}

	_, err := Wire([]string{"nonexistent"}, bus, nil)
	if err == nil {
		t.Fatal("expected error for missing extension")
	}

	if got, want := err.Error(), "wire: extension \"nonexistent\" not registered"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestWire_ReceiveBusInSubscribe(t *testing.T) {
	ResetRegistry()

	var receivedBus Bus

	RegisterExtension("ext-c", func(Config) (Extension, error) {
		return NewExtensionFunc("ext-c", func(bus Bus) {
			receivedBus = bus
		}), nil
	})

	bus := &mockBus{}

	_, err := Wire([]string{"ext-c"}, bus, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if receivedBus == nil {
		t.Fatal("expected bus to be passed to Subscribe")
	}
}

func TestWire_PartialMissing(t *testing.T) {
	ResetRegistry()

	RegisterExtension("good", func(Config) (Extension, error) {
		return NewExtensionFunc("good", func(bus Bus) {}), nil
	})

	bus := &mockBus{}

	_, err := Wire([]string{"good", "missing"}, bus, nil)
	if err == nil {
		t.Fatal("expected error for missing extension")
	}
}

func TestWire_PassesConfigToFactory(t *testing.T) {
	ResetRegistry()

	var receivedCfg Config

	RegisterExtension("cfg-ext", func(cfg Config) (Extension, error) {
		receivedCfg = cfg
		return NewExtensionFunc("cfg-ext", func(Bus) {}), nil
	})

	cfg := &mockConfig{filePath: "/test/.weave.yaml"}
	bus := &mockBus{}

	_, err := Wire([]string{"cfg-ext"}, bus, cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if receivedCfg == nil {
		t.Fatal("expected config to be passed to factory")
	}

	if receivedCfg.FilePath() != "/test/.weave.yaml" {
		t.Errorf("FilePath() = %q, want %q", receivedCfg.FilePath(), "/test/.weave.yaml")
	}
}

func TestWire_FactoryError(t *testing.T) {
	ResetRegistry()

	RegisterExtension("bad", func(Config) (Extension, error) {
		return nil, errors.New("init failed")
	})

	bus := &mockBus{}

	_, err := Wire([]string{"bad"}, bus, nil)
	if err == nil {
		t.Fatal("expected error from failing factory")
	}
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

	bus := &mockBus{}

	wired, err := Wire([]string{"a", "b"}, bus, nil)
	if err != nil {
		t.Fatalf("Wire: %v", err)
	}

	if err := wired.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := closed.Load(); got != 2 {
		t.Errorf("expected 2 Close calls, got %d", got)
	}
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

	bus := &mockBus{}

	wired, err := Wire([]string{"first", "second"}, bus, nil)
	if err != nil {
		t.Fatalf("Wire: %v", err)
	}

	if err := wired.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(order) != 2 || order[0] != "second" || order[1] != "first" {
		t.Errorf("close order = %v, want [second first]", order)
	}
}

// --- WireWithCore: merging ---

func TestWireWithCore_MergesCoreAndOptional(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	var names []string

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &mockProvider{}, nil
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

	bus := &mockBus{}

	_, err := WireWithCore(coreCfg("anthropic"), []string{"bash-tool", "file-tool"}, bus, nil)
	if err != nil {
		t.Fatalf("WireWithCore: %v", err)
	}

	// Providers are validated but not wired; only loop + optional extensions are wired.
	want := []string{"loop", "bash-tool", "file-tool"}
	if len(names) != len(want) {
		t.Fatalf("got %d extensions, want %d", len(names), len(want))
	}

	for i, n := range want {
		if names[i] != n {
			t.Errorf("names[%d] = %q, want %q", i, names[i], n)
		}
	}
}

func TestWireWithCore_Deduplicates(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	var subscribed atomic.Int32

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &mockProvider{}, nil
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

	bus := &mockBus{}

	_, err := WireWithCore(
		coreCfg("anthropic"),
		[]string{"bash-tool", "loop"},
		bus,
		nil,
	)
	if err != nil {
		t.Fatalf("WireWithCore: %v", err)
	}

	// Provider not wired, only loop + bash-tool (deduped)
	if got := subscribed.Load(); got != 2 {
		t.Errorf("expected 2 subscriptions (deduped), got %d", got)
	}
}

func TestWireWithCore_CoreOnly(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &mockProvider{}, nil
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

	bus := &mockBus{}

	_, err := WireWithCore(coreCfg("anthropic"), nil, bus, nil)
	if err != nil {
		t.Fatalf("WireWithCore: %v", err)
	}

	// Only the loop extension is wired, provider is just validated
	if len(names) != 1 || names[0] != "loop" {
		t.Errorf("expected [loop], got %v", names)
	}
}

// --- WireWithCore: validation errors ---

func TestWireWithCore_ErrMissingAgentLoop(t *testing.T) {
	ResetRegistry()

	bus := &mockBus{}

	_, err := WireWithCore(CoreWireConfig{Providers: []string{"anthropic"}}, nil, bus, nil)
	if err == nil {
		t.Fatal("expected error for missing agent-loop")
	}

	if got, want := err.Error(), "wire: agent-loop is required"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestWireWithCore_ErrNoProvider(t *testing.T) {
	ResetRegistry()

	bus := &mockBus{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop"}, nil, bus, nil)
	if err == nil {
		t.Fatal("expected error for no provider")
	}

	if got, want := err.Error(), "wire: at least one provider is required"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestWireWithCore_ErrEmptyProviders(t *testing.T) {
	ResetRegistry()

	bus := &mockBus{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{}}, nil, bus, nil)
	if err == nil {
		t.Fatal("expected error for empty providers")
	}
}

func TestWireWithCore_ErrDuplicateProviders(t *testing.T) {
	ResetRegistry()

	bus := &mockBus{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{"anthropic", "anthropic"}}, nil, bus, nil)
	if err == nil {
		t.Fatal("expected error for duplicate providers")
	}

	if !strings.Contains(err.Error(), "duplicate provider") {
		t.Errorf("error = %q, want mention of 'duplicate provider'", err.Error())
	}
}

func TestWireWithCore_FactoryErrorRollback(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &mockProvider{}, nil
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

	bus := &mockBus{}

	_, err := WireWithCore(coreCfg("anthropic"), []string{"bad"}, bus, nil)
	if err == nil {
		t.Fatal("expected error from failing factory")
	}

	if got := closed.Load(); got != 1 {
		t.Errorf("expected 1 Close call on rollback (loop only), got %d", got)
	}
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

	bus := &mockBus{}

	wired, err := Wire([]string{"a", "b"}, bus, nil)
	if err != nil {
		t.Fatalf("Wire: %v", err)
	}

	err = wired.Close()
	if err == nil {
		t.Fatal("expected error from Close")
	}

	if !strings.Contains(err.Error(), "a failed") || !strings.Contains(err.Error(), "b failed") {
		t.Errorf("error = %q, want both 'a failed' and 'b failed'", err.Error())
	}
}

func TestWireWithCore_SyncsProviderEnv(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("openai", func(Config) (Provider, error) {
		return &mockProvider{}, nil
	})

	RegisterExtension("loop", func(Config) (Extension, error) {
		return NewExtensionFunc("loop", func(Bus) {}), nil
	})

	t.Setenv("WEAVE_PROVIDER", "")

	bus := &mockBus{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{"openai"}}, nil, bus, nil)
	if err != nil {
		t.Fatalf("WireWithCore: %v", err)
	}

	if got := os.Getenv("WEAVE_PROVIDER"); got != "openai" {
		t.Errorf("WEAVE_PROVIDER = %q, want %q", got, "openai")
	}
}

func TestWireWithCore_DoesNotOverrideExistingProviderEnv(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &mockProvider{}, nil
	})
	RegisterProvider("openai", func(Config) (Provider, error) {
		return &mockProvider{}, nil
	})

	RegisterExtension("loop", func(Config) (Extension, error) {
		return NewExtensionFunc("loop", func(Bus) {}), nil
	})

	t.Setenv("WEAVE_PROVIDER", "anthropic")

	bus := &mockBus{}

	_, err := WireWithCore(CoreWireConfig{AgentLoop: "loop", Providers: []string{"openai"}}, nil, bus, nil)
	if err != nil {
		t.Fatalf("WireWithCore: %v", err)
	}

	// Existing env var should NOT be overridden
	if got := os.Getenv("WEAVE_PROVIDER"); got != "anthropic" {
		t.Errorf("WEAVE_PROVIDER = %q, want %q (should not be overridden)", got, "anthropic")
	}
}

func TestWireWithCore_PassesConfigToFactories(t *testing.T) {
	ResetRegistry()
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &mockProvider{}, nil
	})

	var receivedCfg Config

	RegisterExtension("loop", func(cfg Config) (Extension, error) {
		receivedCfg = cfg
		return NewExtensionFunc("loop", func(Bus) {}), nil
	})

	cfg := FilePathConfig("/test/.weave.yaml")
	bus := &mockBus{}

	_, err := WireWithCore(coreCfg("anthropic"), nil, bus, cfg)
	if err != nil {
		t.Fatalf("WireWithCore: %v", err)
	}

	if receivedCfg == nil {
		t.Fatal("expected config passed to factory")
	}

	if receivedCfg.FilePath() != "/test/.weave.yaml" {
		t.Errorf("FilePath() = %q, want %q", receivedCfg.FilePath(), "/test/.weave.yaml")
	}
}
