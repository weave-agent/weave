package sdk

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndRetrieve(t *testing.T) {
	ResetExtensionRegistry()

	ext := NewExtensionFunc("test", func(bus Bus) error { return nil })

	RegisterExtension[struct{}]("test", func(Config, PreferenceReader, struct{}) (Extension, error) { return ext, nil })

	got, err := GetExtension("test", nil)
	require.NoError(t, err, "GetExtension")
	assert.Equal(t, "test", got.Name())
}

func TestRegisterExtensionWithScopeStoresSchemaInfoType(t *testing.T) {
	ResetExtensionRegistry()

	ResetSchemas()
	defer ResetSchemas()

	type extensionConfig struct {
		Enabled bool `json:"enabled" default:"true"`
	}

	RegisterExtensionWithScope("typed", "extensions", func(Config, PreferenceReader, extensionConfig) (Extension, error) {
		return NewExtensionFunc("typed", nil), nil
	})

	info := GetSchemaInfo("extensions", "typed")
	require.NotNil(t, info)
	assert.Equal(t, reflect.TypeFor[extensionConfig](), info.Type)
}

func TestDuplicateRegistration(t *testing.T) {
	ResetExtensionRegistry()

	RegisterExtension[struct{}]("dup", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return NewExtensionFunc("dup", func(bus Bus) error { return nil }), nil
	})

	// Duplicate extension registration logs a warning; first registration wins.
	RegisterExtension[struct{}]("dup", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return NewExtensionFunc("dup-v2", func(bus Bus) error { return nil }), nil
	})

	got, err := GetExtension("dup", nil)
	require.NoError(t, err)
	assert.Equal(t, "dup", got.Name(), "first registration should win")
}

func TestMissingExtension(t *testing.T) {
	ResetExtensionRegistry()

	_, err := GetExtension("nonexistent", nil)
	require.Error(t, err, "expected error for missing extension")
}

func TestGetExtension_FactoryError(t *testing.T) {
	ResetExtensionRegistry()

	RegisterExtension[struct{}]("fail", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return nil, errors.New("boom")
	})

	_, err := GetExtension("fail", nil)
	require.Error(t, err, "expected error from failing factory")
	assert.Equal(t, "boom", err.Error())
}

func TestListExtensions(t *testing.T) {
	ResetExtensionRegistry()

	RegisterExtension[struct{}]("alpha", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return NewExtensionFunc("alpha", nil), nil
	})
	RegisterExtension[struct{}]("beta", func(Config, PreferenceReader, struct{}) (Extension, error) { return NewExtensionFunc("beta", nil), nil })

	names := ListExtensions()
	sort.Strings(names)

	assert.Equal(t, []string{"alpha", "beta"}, names)
}

func TestExtensionRegistered(t *testing.T) {
	ResetExtensionRegistry()

	assert.False(t, ExtensionRegistered("test"), "unregistered extension should not be found")

	RegisterExtension[struct{}]("test", func(Config, PreferenceReader, struct{}) (Extension, error) {
		return NewExtensionFunc("test", nil), nil
	})

	assert.True(t, ExtensionRegistered("test"), "registered extension should be found")
	assert.False(t, ExtensionRegistered("other"), "different name should not be found")
}

func TestRegisterExtensionWithScopeAndWriter_ReceivesPreferenceWriter(t *testing.T) {
	ResetExtensionRegistry()

	var receivedWriter PreferenceWriter

	RegisterExtensionWithScopeAndWriter[struct{}]("privileged", "extensions", func(_ Config, pw PreferenceWriter, _ struct{}) (Extension, error) {
		receivedWriter = pw
		return NewExtensionFunc("privileged", nil), nil
	})

	ext, err := GetExtension("privileged", &mockPrefStoreConfig{})
	require.NoError(t, err)
	assert.Equal(t, "privileged", ext.Name())
	assert.NotNil(t, receivedWriter)
	assert.NoError(t, receivedWriter.SaveProviderKey("test", "key"))
}

func TestRegisterExtensionWithScopeAndWriter_FallsBackToNoop(t *testing.T) {
	ResetExtensionRegistry()

	var receivedWriter PreferenceWriter

	RegisterExtensionWithScopeAndWriter[struct{}]("fallback", "extensions", func(_ Config, pw PreferenceWriter, _ struct{}) (Extension, error) {
		receivedWriter = pw
		return NewExtensionFunc("fallback", nil), nil
	})

	_, err := GetExtension("fallback", nil)
	require.NoError(t, err)
	assert.NotNil(t, receivedWriter)
}
