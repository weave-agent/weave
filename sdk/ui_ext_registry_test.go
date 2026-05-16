package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubUIExtension struct {
	name       string
	config     stubUIExtConfig
	registered bool
}

type stubUIExtConfig struct {
	Enabled bool `json:"enabled"`
}

func (e *stubUIExtension) Name() string  { return e.name }
func (e *stubUIExtension) Register(_ UI) { e.registered = true }

func TestRegisterUIExtension(t *testing.T) {
	ResetUIExtensionRegistry()

	ext := &stubUIExtension{name: "test-ext"}

	RegisterUIExtension("test-ext", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return ext, nil
	})

	exts := GetUIExtensions(NoopConfig{})
	require.Len(t, exts, 1)
	assert.Equal(t, "test-ext", exts[0].Name())
}

func TestRegisterUIExtension_WithConfig(t *testing.T) {
	ResetUIExtensionRegistry()

	var receivedCfg stubUIExtConfig

	RegisterUIExtension("config-ext", func(_ Config, _ PreferenceReader, cfg stubUIExtConfig) (UIExtension, error) {
		receivedCfg = cfg
		return &stubUIExtension{name: "config-ext", config: cfg}, nil
	})

	ext, err := GetUIExtension("config-ext", NoopConfig{})
	require.NoError(t, err)
	assert.Equal(t, "config-ext", ext.Name())
	assert.False(t, receivedCfg.Enabled) // NoopConfig returns nil, so default value
}

func TestRegisterUIExtension_DuplicateWarns(t *testing.T) {
	ResetUIExtensionRegistry()

	first := &stubUIExtension{name: "dup"}

	RegisterUIExtension("dup", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return first, nil
	})

	// Second registration should be a no-op with a warning (no panic).
	RegisterUIExtension("dup", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "dup"}, nil
	})

	// First registration wins.
	exts := GetUIExtensions(NoopConfig{})
	require.Len(t, exts, 1)
	assert.Equal(t, "dup", exts[0].Name())
}

func TestGetUIExtensions_Empty(t *testing.T) {
	ResetUIExtensionRegistry()

	exts := GetUIExtensions(NoopConfig{})
	assert.Empty(t, exts)
}

func TestGetUIExtensions_Multiple(t *testing.T) {
	ResetUIExtensionRegistry()

	RegisterUIExtension("charlie", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "charlie"}, nil
	})
	RegisterUIExtension("alpha", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "alpha"}, nil
	})
	RegisterUIExtension("bravo", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "bravo"}, nil
	})

	exts := GetUIExtensions(NoopConfig{})
	require.Len(t, exts, 3)

	assert.Equal(t, "alpha", exts[0].Name())
	assert.Equal(t, "bravo", exts[1].Name())
	assert.Equal(t, "charlie", exts[2].Name())
}

func TestGetUIExtensions_Sorted(t *testing.T) {
	ResetUIExtensionRegistry()

	RegisterUIExtension("z-ext", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "z-ext"}, nil
	})
	RegisterUIExtension("a-ext", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "a-ext"}, nil
	})
	RegisterUIExtension("m-ext", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "m-ext"}, nil
	})

	exts := GetUIExtensions(NoopConfig{})

	assert.Equal(t, "a-ext", exts[0].Name())
	assert.Equal(t, "m-ext", exts[1].Name())
	assert.Equal(t, "z-ext", exts[2].Name())
}

func TestResetUIExtensionRegistry(t *testing.T) {
	ResetUIExtensionRegistry()

	RegisterUIExtension("temp", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "temp"}, nil
	})

	ResetUIExtensionRegistry()

	assert.Empty(t, GetUIExtensions(NoopConfig{}))
}

func TestGetUIExtension_NotRegistered(t *testing.T) {
	ResetUIExtensionRegistry()

	_, err := GetUIExtension("missing", NoopConfig{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotRegistered)
}

func TestListUIExtensions(t *testing.T) {
	ResetUIExtensionRegistry()

	RegisterUIExtension("beta", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "beta"}, nil
	})
	RegisterUIExtension("alpha", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "alpha"}, nil
	})

	names := ListUIExtensions()
	require.Len(t, names, 2)
	assert.Equal(t, "alpha", names[0])
	assert.Equal(t, "beta", names[1])
}

func TestUIExtensionRegistered(t *testing.T) {
	ResetUIExtensionRegistry()

	assert.False(t, UIExtensionRegistered("none"))

	RegisterUIExtension("exists", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "exists"}, nil
	})

	assert.True(t, UIExtensionRegistered("exists"))
	assert.False(t, UIExtensionRegistered("missing"))
}

func TestGetUIExtensions_FactoryErrorSkipped(t *testing.T) {
	ResetUIExtensionRegistry()

	RegisterUIExtension("good", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return &stubUIExtension{name: "good"}, nil
	})
	RegisterUIExtension("bad", func(_ Config, _ PreferenceReader, _ struct{}) (UIExtension, error) {
		return nil, assert.AnError
	})

	exts := GetUIExtensions(NoopConfig{})
	require.Len(t, exts, 1)
	assert.Equal(t, "good", exts[0].Name())
}
