package sdk

import (
	"sync/atomic"
	"testing"
)

func TestWire_NoExtensions(t *testing.T) {
	resetRegistry()
	bus := &mockBus{}
	cfg := &mockConfig{
		slices: map[string][]string{},
	}

	err := Wire(cfg, bus)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestWire_NilExtensions(t *testing.T) {
	resetRegistry()
	bus := &mockBus{}
	cfg := &mockConfig{}

	err := Wire(cfg, bus)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestWire_SubscribesAllExtensions(t *testing.T) {
	resetRegistry()

	var subscribed atomic.Int32

	RegisterExtension("ext-a", func() Extension {
		return NewExtensionFunc("ext-a", func(bus Bus) {
			subscribed.Add(1)
		})
	})
	RegisterExtension("ext-b", func() Extension {
		return NewExtensionFunc("ext-b", func(bus Bus) {
			subscribed.Add(1)
		})
	})

	bus := &mockBus{}
	cfg := &mockConfig{
		slices: map[string][]string{
			"extensions": {"ext-a", "ext-b"},
		},
	}

	err := Wire(cfg, bus)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := subscribed.Load(); got != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", got)
	}
}

func TestWire_MissingExtension(t *testing.T) {
	resetRegistry()

	bus := &mockBus{}
	cfg := &mockConfig{
		slices: map[string][]string{
			"extensions": {"nonexistent"},
		},
	}

	err := Wire(cfg, bus)
	if err == nil {
		t.Fatal("expected error for missing extension")
	}
	if got, want := err.Error(), "wire: extension \"nonexistent\" not registered"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestWire_ReceiveBusInSubscribe(t *testing.T) {
	resetRegistry()

	var receivedBus Bus
	RegisterExtension("ext-c", func() Extension {
		return NewExtensionFunc("ext-c", func(bus Bus) {
			receivedBus = bus
		})
	})

	bus := &mockBus{}
	cfg := &mockConfig{
		slices: map[string][]string{
			"extensions": {"ext-c"},
		},
	}

	err := Wire(cfg, bus)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if receivedBus == nil {
		t.Fatal("expected bus to be passed to Subscribe")
	}
}

func TestWire_PartialMissing(t *testing.T) {
	resetRegistry()

	RegisterExtension("good", func() Extension {
		return NewExtensionFunc("good", func(bus Bus) {})
	})

	bus := &mockBus{}
	cfg := &mockConfig{
		slices: map[string][]string{
			"extensions": {"good", "missing"},
		},
	}

	err := Wire(cfg, bus)
	if err == nil {
		t.Fatal("expected error for missing extension")
	}
}
