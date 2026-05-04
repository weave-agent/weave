package sdk

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndRetrieve(t *testing.T) {
	ResetRegistry()

	ext := NewExtensionFunc("test", func(bus Bus) {})

	RegisterExtension("test", func(Config) (Extension, error) { return ext, nil })

	got, err := GetExtension("test", nil)
	require.NoError(t, err, "GetExtension")
	assert.Equal(t, "test", got.Name())
}

func TestDuplicateRegistration(t *testing.T) {
	ResetRegistry()

	RegisterExtension("dup", func(Config) (Extension, error) {
		return NewExtensionFunc("dup", func(bus Bus) {}), nil
	})

	// Duplicate extension registration logs a warning; first registration wins.
	RegisterExtension("dup", func(Config) (Extension, error) {
		return NewExtensionFunc("dup-v2", func(bus Bus) {}), nil
	})

	got, err := GetExtension("dup", nil)
	require.NoError(t, err)
	assert.Equal(t, "dup", got.Name(), "first registration should win")
}

func TestMissingExtension(t *testing.T) {
	ResetRegistry()

	_, err := GetExtension("nonexistent", nil)
	require.Error(t, err, "expected error for missing extension")
}

func TestGetExtension_FactoryError(t *testing.T) {
	ResetRegistry()

	RegisterExtension("fail", func(Config) (Extension, error) {
		return nil, errors.New("boom")
	})

	_, err := GetExtension("fail", nil)
	require.Error(t, err, "expected error from failing factory")
	assert.Equal(t, "boom", err.Error())
}

func TestListExtensions(t *testing.T) {
	ResetRegistry()

	RegisterExtension("alpha", func(Config) (Extension, error) { return NewExtensionFunc("alpha", nil), nil })
	RegisterExtension("beta", func(Config) (Extension, error) { return NewExtensionFunc("beta", nil), nil })

	names := ListExtensions()
	sort.Strings(names)

	assert.Equal(t, []string{"alpha", "beta"}, names)
}
