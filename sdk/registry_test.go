package sdk

import (
	"errors"
	"sort"
	"testing"
)

func TestRegisterAndRetrieve(t *testing.T) {
	ResetRegistry()

	ext := NewExtensionFunc("test", func(bus Bus) {})

	RegisterExtension("test", func(Config) (Extension, error) { return ext, nil })

	got, err := GetExtension("test", nil)
	if err != nil {
		t.Fatalf("GetExtension: %v", err)
	}

	if got.Name() != "test" {
		t.Errorf("Name() = %q, want %q", got.Name(), "test")
	}
}

func TestDuplicateRegistration(t *testing.T) {
	ResetRegistry()

	RegisterExtension("dup", func(Config) (Extension, error) {
		return NewExtensionFunc("dup", func(bus Bus) {}), nil
	})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()

	RegisterExtension("dup", func(Config) (Extension, error) {
		return NewExtensionFunc("dup-v2", func(bus Bus) {}), nil
	})
}

func TestMissingExtension(t *testing.T) {
	ResetRegistry()

	_, err := GetExtension("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for missing extension")
	}
}

func TestGetExtension_FactoryError(t *testing.T) {
	ResetRegistry()

	RegisterExtension("fail", func(Config) (Extension, error) {
		return nil, errors.New("boom")
	})

	_, err := GetExtension("fail", nil)
	if err == nil {
		t.Fatal("expected error from failing factory")
	}

	if err.Error() != "boom" {
		t.Errorf("error = %q, want %q", err.Error(), "boom")
	}
}

func TestListExtensions(t *testing.T) {
	ResetRegistry()

	RegisterExtension("alpha", func(Config) (Extension, error) { return NewExtensionFunc("alpha", nil), nil })
	RegisterExtension("beta", func(Config) (Extension, error) { return NewExtensionFunc("beta", nil), nil })

	names := ListExtensions()
	sort.Strings(names)

	want := []string{"alpha", "beta"}
	if len(names) != len(want) {
		t.Fatalf("ListExtensions() = %v, want %v", names, want)
	}

	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}
