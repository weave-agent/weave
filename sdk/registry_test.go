package sdk

import (
	"sort"
	"testing"
)

func TestRegisterAndRetrieve(t *testing.T) {
	ResetRegistry()

	ext := NewExtensionFunc("test", func(bus Bus) {})

	RegisterExtension("test", func() Extension { return ext })

	got, err := GetExtension("test")
	if err != nil {
		t.Fatalf("GetExtension: %v", err)
	}

	if got.Name() != "test" {
		t.Errorf("Name() = %q, want %q", got.Name(), "test")
	}
}

func TestDuplicateRegistration(t *testing.T) {
	ResetRegistry()

	first := NewExtensionFunc("dup", func(bus Bus) {})
	second := NewExtensionFunc("dup-v2", func(bus Bus) {})

	RegisterExtension("dup", func() Extension { return first })
	RegisterExtension("dup", func() Extension { return second })

	got, _ := GetExtension("dup")
	if got.Name() != "dup-v2" {
		t.Errorf("after duplicate register, Name() = %q, want %q", got.Name(), "dup-v2")
	}
}

func TestMissingExtension(t *testing.T) {
	ResetRegistry()

	_, err := GetExtension("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing extension")
	}
}

func TestListExtensions(t *testing.T) {
	ResetRegistry()

	RegisterExtension("alpha", func() Extension { return NewExtensionFunc("alpha", nil) })
	RegisterExtension("beta", func() Extension { return NewExtensionFunc("beta", nil) })

	names := ListExtensions()
	sort.Strings(names)

	want := []string{"alpha", "beta"}
	if len(names) != len(want) {
		t.Fatalf("ListExtensions() = %v, want %v", names, want)
	}

	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, want[i])
		}
	}
}
