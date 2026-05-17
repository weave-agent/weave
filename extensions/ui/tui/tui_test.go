package tui

import (
	"testing"

	"github.com/weave-agent/weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUI_ExtensionRegistration(t *testing.T) {
	sdk.ResetExtensionRegistry()
	defer sdk.ResetExtensionRegistry()

	sdk.RegisterExtensionWithScopeAndWriter("tui", "ui", func(cfg sdk.Config, _ sdk.PreferenceWriter, _ TUIConfig) (sdk.Extension, error) {
		return NewTUI(cfg, nil, TUIConfig{})
	})

	ext, err := sdk.GetExtension("tui", nil)
	require.NoError(t, err)
	assert.Equal(t, "tui", ext.Name())

	_, ok := ext.(*TUI)
	require.True(t, ok, "expected *TUI, got %T", ext)
}

func TestTUI_Name(t *testing.T) {
	tui, err := NewTUI(nil, nil, TUIConfig{})
	require.NoError(t, err)
	assert.Equal(t, "tui", tui.Name())
}

func TestTUI_CloseWithoutSubscribe(t *testing.T) {
	tui, err := NewTUI(nil, nil, TUIConfig{})
	require.NoError(t, err)

	// Close without Subscribe should not panic or block
	require.NoError(t, tui.Close())
}

func TestModel_View(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	// View includes: chat (empty) + editor (empty) + footer (2 lines)
	// With no size set, chat="" and editor="" and footer renders "weave" label
	view := m.View()
	// Should contain the footer's "weave" fallback
	assert.Contains(t, view.Content, "weave")
	// Should contain newlines separating sections
	assert.Contains(t, view.Content, "\n")
	assert.True(t, view.AltScreen)
	assert.True(t, view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes)
	assert.True(t, view.KeyboardEnhancements.ReportAssociatedText)
}

func TestModel_Init(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	cmd := m.Init()
	assert.NotNil(t, cmd)
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

	sdk.RegisterUIExtension("test-ext", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.UIExtension, error) {
		return ext, nil
	})

	tui, err := NewTUI(nil, nil, TUIConfig{})
	require.NoError(t, err)

	tui.wireUIExtensions(nil)

	assert.True(t, ext.registerCalled, "expected Register to be called on UI extension")
	assert.Equal(t, tui.ui, ext.registeredUI, "expected UI extension to receive TUI's UI implementation")
}

func TestTUI_WireUIExtensions_Multiple(t *testing.T) {
	sdk.ResetUIExtensionRegistry()
	defer sdk.ResetUIExtensionRegistry()

	ext1 := &mockUIExtension{name: "ext-one"}
	ext2 := &mockUIExtension{name: "ext-two"}

	sdk.RegisterUIExtension("ext-one", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.UIExtension, error) {
		return ext1, nil
	})
	sdk.RegisterUIExtension("ext-two", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.UIExtension, error) {
		return ext2, nil
	})

	tui, err := NewTUI(nil, nil, TUIConfig{})
	require.NoError(t, err)

	tui.wireUIExtensions(nil)

	assert.True(t, ext1.registerCalled, "expected Register to be called on ext-one")
	assert.True(t, ext2.registerCalled, "expected Register to be called on ext-two")
	assert.Equal(t, tui.ui, ext1.registeredUI)
	assert.Equal(t, tui.ui, ext2.registeredUI)
}

func TestTUI_WireUIExtensions_EmptyRegistry(t *testing.T) {
	sdk.ResetUIExtensionRegistry()
	defer sdk.ResetUIExtensionRegistry()

	tui, err := NewTUI(nil, nil, TUIConfig{})
	require.NoError(t, err)

	// Should not panic with empty registry
	assert.NotPanics(t, func() {
		tui.wireUIExtensions(nil)
	})
}

// mockUIExtensionWithBus records whether RegisterWithBus was called.
type mockUIExtensionWithBus struct {
	name             string
	registerCalled   bool
	registeredUI     sdk.UI
	busCalled        bool
	registeredUIWith sdk.UI
	registeredBus    sdk.Bus
}

// mockBus is a minimal sdk.Bus implementation for tests.
type mockBus struct{}

func (m *mockBus) Publish(_ sdk.Event)        {}
func (m *mockBus) On(_ string, _ sdk.Handler) {}
func (m *mockBus) OnAll(_ sdk.Handler)        {}
func (m *mockBus) Off(_ sdk.Handler)          {}
func (m *mockBus) Close() error               { return nil }

func (m *mockUIExtensionWithBus) Name() string { return m.name }
func (m *mockUIExtensionWithBus) Register(ui sdk.UI) {
	m.registerCalled = true
	m.registeredUI = ui
}

func (m *mockUIExtensionWithBus) RegisterWithBus(ui sdk.UI, bus sdk.Bus) {
	m.busCalled = true
	m.registeredUIWith = ui
	m.registeredBus = bus
}

func TestTUI_WireUIExtensions_WithBus(t *testing.T) {
	sdk.ResetUIExtensionRegistry()
	defer sdk.ResetUIExtensionRegistry()

	ext := &mockUIExtensionWithBus{name: "bus-ext"}

	sdk.RegisterUIExtension("bus-ext", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.UIExtension, error) {
		return ext, nil
	})

	tui, err := NewTUI(nil, nil, TUIConfig{})
	require.NoError(t, err)

	bus := &mockBus{}
	tui.wireUIExtensions(bus)

	assert.True(t, ext.registerCalled, "expected Register to be called")
	assert.True(t, ext.busCalled, "expected RegisterWithBus to be called")
	assert.Equal(t, tui.ui, ext.registeredUIWith)
	assert.Equal(t, bus, ext.registeredBus)
}

func TestTUI_WireUIExtensions_PlainExtension_NoBus(t *testing.T) {
	sdk.ResetUIExtensionRegistry()
	defer sdk.ResetUIExtensionRegistry()

	ext := &mockUIExtension{name: "plain-ext"}

	sdk.RegisterUIExtension("plain-ext", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.UIExtension, error) {
		return ext, nil
	})

	tui, err := NewTUI(nil, nil, TUIConfig{})
	require.NoError(t, err)

	// Should not panic even with a bus when extension doesn't implement UIExtensionWithBus
	assert.NotPanics(t, func() {
		tui.wireUIExtensions(&mockBus{})
	})

	assert.True(t, ext.registerCalled)
}
