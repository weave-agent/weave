package sdk

import (
	"errors"
	"sync/atomic"
	"testing"
)

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
