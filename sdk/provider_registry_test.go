package sdk

import (
	"context"
	"errors"
	"sort"
	"testing"
)

type mockProvider struct {
	name string
}

func (m *mockProvider) Stream(_ context.Context, _ ProviderRequest) (<-chan ProviderEvent, error) {
	ch := make(chan ProviderEvent)
	close(ch)

	return ch, nil
}

func TestRegisterAndRetrieveProvider(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider("mock", func(Config) (Provider, error) {
		return &mockProvider{name: "mock"}, nil
	})

	got, err := GetProvider("mock", nil)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}

	if got.(*mockProvider).name != "mock" {
		t.Errorf("provider name = %q, want %q", got.(*mockProvider).name, "mock")
	}
}

func TestDuplicateProviderRegistration(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider("dup", func(Config) (Provider, error) {
		return &mockProvider{name: "first"}, nil
	})
	RegisterProvider("dup", func(Config) (Provider, error) {
		return &mockProvider{name: "second"}, nil
	})

	got, _ := GetProvider("dup", nil)
	if got.(*mockProvider).name != "second" {
		t.Errorf("after duplicate register, name = %q, want %q", got.(*mockProvider).name, "second")
	}
}

func TestMissingProvider(t *testing.T) {
	ResetProviderRegistry()

	_, err := GetProvider("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestGetProvider_FactoryError(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider("fail", func(Config) (Provider, error) {
		return nil, errors.New("factory error")
	})

	_, err := GetProvider("fail", nil)
	if err == nil {
		t.Fatal("expected error from failing factory")
	}

	if err.Error() != "factory error" {
		t.Errorf("error = %q, want %q", err.Error(), "factory error")
	}
}

func TestListProviders(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider("anthropic", func(Config) (Provider, error) {
		return &mockProvider{name: "anthropic"}, nil
	})
	RegisterProvider("openai", func(Config) (Provider, error) {
		return &mockProvider{name: "openai"}, nil
	})

	names := ListProviders()
	sort.Strings(names)

	want := []string{"anthropic", "openai"}
	if len(names) != len(want) {
		t.Fatalf("ListProviders() = %v, want %v", names, want)
	}

	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestResetProviderRegistry(t *testing.T) {
	ResetProviderRegistry()

	RegisterProvider("temp", func(Config) (Provider, error) {
		return &mockProvider{name: "temp"}, nil
	})

	ResetProviderRegistry()

	names := ListProviders()
	if len(names) != 0 {
		t.Errorf("after reset, ListProviders() = %v, want empty", names)
	}
}
