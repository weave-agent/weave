package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubUIExtension struct {
	name      string
	registered bool
}

func (e *stubUIExtension) Name() string       { return e.name }
func (e *stubUIExtension) Register(_ UI) { e.registered = true }

func TestRegisterUIExtension(t *testing.T) {
	ResetUIExtensionRegistry()

	ext := &stubUIExtension{name: "test-ext"}
	RegisterUIExtension(ext)

	exts := GetUIExtensions()
	require.Len(t, exts, 1)
	assert.Equal(t, "test-ext", exts[0].Name())
}

func TestRegisterUIExtension_DuplicatePanics(t *testing.T) {
	ResetUIExtensionRegistry()

	RegisterUIExtension(&stubUIExtension{name: "dup"})

	defer func() {
		require.NotNil(t, recover(), "expected panic on duplicate UI extension registration")
	}()

	RegisterUIExtension(&stubUIExtension{name: "dup"})
}

func TestGetUIExtensions_Empty(t *testing.T) {
	ResetUIExtensionRegistry()

	exts := GetUIExtensions()
	assert.Empty(t, exts)
}

func TestGetUIExtensions_Multiple(t *testing.T) {
	ResetUIExtensionRegistry()

	RegisterUIExtension(&stubUIExtension{name: "charlie"})
	RegisterUIExtension(&stubUIExtension{name: "alpha"})
	RegisterUIExtension(&stubUIExtension{name: "bravo"})

	exts := GetUIExtensions()
	require.Len(t, exts, 3)

	assert.Equal(t, "alpha", exts[0].Name())
	assert.Equal(t, "bravo", exts[1].Name())
	assert.Equal(t, "charlie", exts[2].Name())
}

func TestGetUIExtensions_Sorted(t *testing.T) {
	ResetUIExtensionRegistry()

	RegisterUIExtension(&stubUIExtension{name: "z-ext"})
	RegisterUIExtension(&stubUIExtension{name: "a-ext"})
	RegisterUIExtension(&stubUIExtension{name: "m-ext"})

	exts := GetUIExtensions()

	assert.Equal(t, "a-ext", exts[0].Name())
	assert.Equal(t, "m-ext", exts[1].Name())
	assert.Equal(t, "z-ext", exts[2].Name())
}

func TestResetUIExtensionRegistry(t *testing.T) {
	ResetUIExtensionRegistry()

	RegisterUIExtension(&stubUIExtension{name: "temp"})

	ResetUIExtensionRegistry()

	assert.Empty(t, GetUIExtensions())
}
