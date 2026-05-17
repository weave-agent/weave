package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weave-agent/weave/sdk"
)

type stubTUIExtension struct {
	name       string
	config     stubTUIExtConfig
	registered bool
}

type stubTUIExtConfig struct {
	Enabled bool `json:"enabled"`
}

func (e *stubTUIExtension) Name() string            { return e.name }
func (e *stubTUIExtension) RegisterTUI(_ TUIExtAPI) { e.registered = true }

func TestRegisterTUIExtension(t *testing.T) {
	ResetTUIExtensionRegistry()

	ext := &stubTUIExtension{name: "test-ext"}

	RegisterTUIExtension("test-ext", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return ext, nil
	})

	exts := GetTUIExtensions(sdk.NoopConfig{})
	require.Len(t, exts, 1)
	assert.Equal(t, "test-ext", exts[0].Name())
}

func TestRegisterTUIExtension_WithConfig(t *testing.T) {
	ResetTUIExtensionRegistry()

	var receivedCfg stubTUIExtConfig

	RegisterTUIExtension("config-ext", func(_ sdk.Config, _ sdk.PreferenceReader, cfg stubTUIExtConfig) (TUIExtension, error) {
		receivedCfg = cfg
		return &stubTUIExtension{name: "config-ext", config: cfg}, nil
	})

	ext, err := GetTUIExtension("config-ext", sdk.NoopConfig{})
	require.NoError(t, err)
	assert.Equal(t, "config-ext", ext.Name())
	assert.False(t, receivedCfg.Enabled)
}

func TestRegisterTUIExtension_DuplicateWarns(t *testing.T) {
	ResetTUIExtensionRegistry()

	first := &stubTUIExtension{name: "dup"}

	RegisterTUIExtension("dup", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return first, nil
	})

	// Second registration should be a no-op with a warning (no panic).
	RegisterTUIExtension("dup", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return &stubTUIExtension{name: "dup"}, nil
	})

	// First registration wins.
	exts := GetTUIExtensions(sdk.NoopConfig{})
	require.Len(t, exts, 1)
	assert.Equal(t, "dup", exts[0].Name())
}

func TestGetTUIExtensions_Empty(t *testing.T) {
	ResetTUIExtensionRegistry()

	exts := GetTUIExtensions(sdk.NoopConfig{})
	assert.Empty(t, exts)
}

func TestGetTUIExtensions_Multiple(t *testing.T) {
	ResetTUIExtensionRegistry()

	RegisterTUIExtension("charlie", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return &stubTUIExtension{name: "charlie"}, nil
	})
	RegisterTUIExtension("alpha", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return &stubTUIExtension{name: "alpha"}, nil
	})
	RegisterTUIExtension("bravo", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return &stubTUIExtension{name: "bravo"}, nil
	})

	exts := GetTUIExtensions(sdk.NoopConfig{})
	require.Len(t, exts, 3)

	assert.Equal(t, "alpha", exts[0].Name())
	assert.Equal(t, "bravo", exts[1].Name())
	assert.Equal(t, "charlie", exts[2].Name())
}

func TestResetTUIExtensionRegistry(t *testing.T) {
	ResetTUIExtensionRegistry()

	RegisterTUIExtension("temp", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return &stubTUIExtension{name: "temp"}, nil
	})

	ResetTUIExtensionRegistry()

	assert.Empty(t, GetTUIExtensions(sdk.NoopConfig{}))
}

func TestGetTUIExtension_NotRegistered(t *testing.T) {
	ResetTUIExtensionRegistry()

	_, err := GetTUIExtension("missing", sdk.NoopConfig{})
	require.Error(t, err)
	assert.ErrorIs(t, err, sdk.ErrNotRegistered)
}

func TestListTUIExtensions(t *testing.T) {
	ResetTUIExtensionRegistry()

	RegisterTUIExtension("beta", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return &stubTUIExtension{name: "beta"}, nil
	})
	RegisterTUIExtension("alpha", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return &stubTUIExtension{name: "alpha"}, nil
	})

	names := ListTUIExtensions()
	require.Len(t, names, 2)
	assert.Equal(t, "alpha", names[0])
	assert.Equal(t, "beta", names[1])
}

func TestTUIExtensionRegistered(t *testing.T) {
	ResetTUIExtensionRegistry()

	assert.False(t, TUIExtensionRegistered("none"))

	RegisterTUIExtension("exists", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return &stubTUIExtension{name: "exists"}, nil
	})

	assert.True(t, TUIExtensionRegistered("exists"))
	assert.False(t, TUIExtensionRegistered("missing"))
}

func TestGetTUIExtensions_FactoryErrorSkipped(t *testing.T) {
	ResetTUIExtensionRegistry()

	RegisterTUIExtension("good", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return &stubTUIExtension{name: "good"}, nil
	})
	RegisterTUIExtension("bad", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (TUIExtension, error) {
		return nil, assert.AnError
	})

	exts := GetTUIExtensions(sdk.NoopConfig{})
	require.Len(t, exts, 1)
	assert.Equal(t, "good", exts[0].Name())
}
