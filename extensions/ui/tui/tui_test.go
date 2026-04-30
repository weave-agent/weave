package tui

import (
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUI_ExtensionRegistration(t *testing.T) {
	sdk.ResetRegistry()
	defer sdk.ResetRegistry()

	sdk.RegisterExtension("tui", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewTUI(cfg)
	})

	ext, err := sdk.GetExtension("tui", nil)
	require.NoError(t, err)
	assert.Equal(t, "tui", ext.Name())

	_, ok := ext.(*TUI)
	require.True(t, ok, "expected *TUI, got %T", ext)
}

func TestTUI_Name(t *testing.T) {
	tui, err := NewTUI(nil)
	require.NoError(t, err)
	assert.Equal(t, "tui", tui.Name())
}

func TestTUI_CloseWithoutSubscribe(t *testing.T) {
	tui, err := NewTUI(nil)
	require.NoError(t, err)

	// Close without Subscribe should not panic or block
	require.NoError(t, tui.Close())
}

func TestModel_View(t *testing.T) {
	m := newModel(nil, nil, nil)
	// View includes: chat (empty) + editor (empty) + footer (2 lines)
	// With no size set, chat="" and editor="" and footer renders "weave" label
	view := m.View()
	// Should contain the footer's "weave" fallback
	assert.Contains(t, view.Content, "weave")
	// Should contain newlines separating sections
	assert.Contains(t, view.Content, "\n")
}

func TestModel_Init(t *testing.T) {
	m := newModel(nil, nil, nil)
	cmd := m.Init()
	assert.Nil(t, cmd)
}

func TestTUI_NoTTYError(t *testing.T) {
	// ErrNoTTY should be a sentinel error that callers can check
	require.Error(t, ErrNoTTY)
	assert.Contains(t, ErrNoTTY.Error(), "stdin")
}

// mockUIExtension records whether Register was called and with what UI.
type mockUIExtension struct {
	name           string
	registerCalled bool
	registeredUI   sdk.UI
}

func (m *mockUIExtension) Name() string { return m.name }
func (m *mockUIExtension) Register(ui sdk.UI) {
	m.registerCalled = true
	m.registeredUI = ui
}

func TestTUI_WireUIExtensions(t *testing.T) {
	sdk.ResetUIExtensionRegistry()
	defer sdk.ResetUIExtensionRegistry()

	ext := &mockUIExtension{name: "test-ext"}
	sdk.RegisterUIExtension(ext)

	tui, err := NewTUI(nil)
	require.NoError(t, err)

	tui.wireUIExtensions()

	assert.True(t, ext.registerCalled, "expected Register to be called on UI extension")
	assert.Equal(t, tui.ui, ext.registeredUI, "expected UI extension to receive TUI's UI implementation")
}

func TestTUI_WireUIExtensions_Multiple(t *testing.T) {
	sdk.ResetUIExtensionRegistry()
	defer sdk.ResetUIExtensionRegistry()

	ext1 := &mockUIExtension{name: "ext-one"}
	ext2 := &mockUIExtension{name: "ext-two"}
	sdk.RegisterUIExtension(ext1)
	sdk.RegisterUIExtension(ext2)

	tui, err := NewTUI(nil)
	require.NoError(t, err)

	tui.wireUIExtensions()

	assert.True(t, ext1.registerCalled, "expected Register to be called on ext-one")
	assert.True(t, ext2.registerCalled, "expected Register to be called on ext-two")
	assert.Equal(t, tui.ui, ext1.registeredUI)
	assert.Equal(t, tui.ui, ext2.registeredUI)
}

func TestTUI_WireUIExtensions_EmptyRegistry(t *testing.T) {
	sdk.ResetUIExtensionRegistry()
	defer sdk.ResetUIExtensionRegistry()

	tui, err := NewTUI(nil)
	require.NoError(t, err)

	// Should not panic with empty registry
	assert.NotPanics(t, func() {
		tui.wireUIExtensions()
	})
}
