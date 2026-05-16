package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindingRegistry_Defaults(t *testing.T) {
	r := NewBindingRegistry()

	actions := map[string]BindingAction{
		"ctrl+d":      ActionExit,
		"esc":         ActionInterrupt,
		"ctrl+l":      ActionModelSelect,
		"ctrl+p":      ActionModelCycle,
		"shift+tab":   ActionThinkingCycle,
		"shift+enter": ActionEditorNewline,
		"ctrl+j":      ActionEditorNewline,
	}

	for key, want := range actions {
		action, ok := r.Resolve(key)
		assert.True(t, ok, "expected default binding for %s", key)
		assert.Equal(t, want, action, "wrong action for key %s", key)
	}
}

func TestBindingRegistry_UnknownKey(t *testing.T) {
	r := NewBindingRegistry()

	_, ok := r.Resolve("ctrl+f")
	assert.False(t, ok)
}

func TestBindingRegistry_ExtensionOverridesDefault(t *testing.T) {
	r := NewBindingRegistry()

	// Register ctrl+d for a custom action (overrides exit)
	r.Register("app.custom", []string{"ctrl+d"}, "Custom action")

	action, ok := r.Resolve("ctrl+d")
	require.True(t, ok)
	assert.Equal(t, BindingAction("app.custom"), action)
}

func TestBindingRegistry_ExtensionRegistersNewKey(t *testing.T) {
	r := NewBindingRegistry()

	r.Register("app.search", []string{"ctrl+f"}, "Search")

	action, ok := r.Resolve("ctrl+f")
	require.True(t, ok)
	assert.Equal(t, BindingAction("app.search"), action)
}

func TestBindingRegistry_ExtensionReplacesOwnKeys(t *testing.T) {
	r := NewBindingRegistry()

	r.Register("app.custom", []string{"ctrl+f"}, "First")
	r.Register("app.custom", []string{"ctrl+g"}, "Second")

	// Old key should be gone
	_, ok := r.Resolve("ctrl+f")
	assert.False(t, ok, "old key should be removed when action is re-registered")

	// New key should work
	action, ok := r.Resolve("ctrl+g")
	require.True(t, ok)
	assert.Equal(t, BindingAction("app.custom"), action)
}

func TestBindingRegistry_UserConfigOverridesAll(t *testing.T) {
	r := NewBindingRegistry()

	// User remaps model select from ctrl+l to ctrl+k
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "keybindings.yaml")
	err := os.WriteFile(cfgPath, []byte("keybindings:\n  app.model.select:\n    - ctrl+k\n"), 0o644)
	require.NoError(t, err)

	err = r.LoadUserConfig(cfgPath)
	require.NoError(t, err)

	// ctrl+l no longer triggers model select (unless extension also registered it)
	action, ok := r.Resolve("ctrl+k")
	require.True(t, ok)
	assert.Equal(t, ActionModelSelect, action)

	// ctrl+l still resolves from defaults
	action, ok = r.Resolve("ctrl+l")
	require.True(t, ok)
	assert.Equal(t, ActionModelSelect, action)
}

func TestBindingRegistry_UserConfigRemapsKey(t *testing.T) {
	r := NewBindingRegistry()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "keybindings.yaml")
	err := os.WriteFile(cfgPath, []byte("keybindings:\n  app.exit:\n    - ctrl+q\n"), 0o644)
	require.NoError(t, err)

	err = r.LoadUserConfig(cfgPath)
	require.NoError(t, err)

	// ctrl+q now triggers exit
	action, ok := r.Resolve("ctrl+q")
	require.True(t, ok)
	assert.Equal(t, ActionExit, action)

	// ctrl+d still works from defaults
	action, ok = r.Resolve("ctrl+d")
	require.True(t, ok)
	assert.Equal(t, ActionExit, action)
}

func TestBindingRegistry_UserConfigNonExistent(t *testing.T) {
	r := NewBindingRegistry()

	err := r.LoadUserConfig("/nonexistent/path/keybindings.yaml")
	require.NoError(t, err)

	// Defaults should still work
	action, ok := r.Resolve("ctrl+d")
	require.True(t, ok)
	assert.Equal(t, ActionExit, action)
}

func TestBindingRegistry_UserConfigPriority(t *testing.T) {
	r := NewBindingRegistry()

	// Extension registers ctrl+f for search
	r.Register("app.search", []string{"ctrl+f"}, "Search")

	// User remaps ctrl+f to something else
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "keybindings.yaml")
	err := os.WriteFile(cfgPath, []byte("keybindings:\n  app.exit:\n    - ctrl+f\n"), 0o644)
	require.NoError(t, err)

	err = r.LoadUserConfig(cfgPath)
	require.NoError(t, err)

	// User config wins: ctrl+f -> exit, not search
	action, ok := r.Resolve("ctrl+f")
	require.True(t, ok)
	assert.Equal(t, ActionExit, action)
}

func TestBindingRegistry_AllBindings(t *testing.T) {
	r := NewBindingRegistry()

	bindings := r.AllBindings()
	assert.NotEmpty(t, bindings)

	// Should contain all default actions
	names := make(map[BindingAction]bool)
	for _, b := range bindings {
		names[b.Action] = true
	}

	assert.True(t, names[ActionExit])
	assert.True(t, names[ActionInterrupt])
	assert.True(t, names[ActionModelSelect])
	assert.True(t, names[ActionModelCycle])
	assert.True(t, names[ActionThinkingCycle])
	assert.True(t, names[ActionCursorLineStart])
	assert.True(t, names[ActionSuspend])
}

func TestBindingRegistry_AllBindingsSorted(t *testing.T) {
	r := NewBindingRegistry()

	bindings := r.AllBindings()
	for i := 1; i < len(bindings); i++ {
		assert.LessOrEqual(t, bindings[i-1].Action, bindings[i].Action,
			"bindings should be sorted by action name")
	}
}

func TestKeyString(t *testing.T) {
	tests := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, "ctrl+c"},
		{tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, "ctrl+d"},
		{tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}, "ctrl+l"},
		{tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}, "ctrl+p"},
		{tea.KeyPressMsg{Code: tea.KeyEsc}, "esc"},
		// Keystroke() includes modifier prefix even when Text is set
		{tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt, Text: "a"}, "alt+a"},
		{tea.KeyPressMsg{Code: 'g', Mod: tea.ModShift, Text: "G"}, "shift+g"},
		{tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt, Text: "d"}, "alt+d"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, keyString(tt.key), "keyString(%v)", tt.key)
	}
}

func TestKeyString_EscapePassthrough(t *testing.T) {
	msg := tea.KeyPressMsg{Code: tea.KeyEsc}
	assert.Equal(t, "esc", keyString(msg))
}

func TestLoadKeybindings_ProjectConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".weave", "settings.json")

	// No keybindings file -> empty
	result := loadKeybindings(cfgPath)
	assert.Empty(t, result)
}

func TestLoadKeybindings_NearConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".weave", "settings.json")
	kbPath := filepath.Join(dir, ".weave", "keybindings.yaml")

	require.NoError(t, os.MkdirAll(filepath.Dir(cfgPath), 0o755))

	err := os.WriteFile(kbPath, []byte("keybindings: {}\n"), 0o644)
	require.NoError(t, err)

	result := loadKeybindings(cfgPath)
	assert.Equal(t, kbPath, result)
}

func TestLoadKeybindings_EmptyConfigPath(t *testing.T) {
	result := loadKeybindings("")
	assert.Empty(t, result)
}

func TestModel_BindingsRegistryInitialized(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	assert.NotNil(t, m.bindings)

	// Default keybinding should work
	action, ok := m.bindings.Resolve("ctrl+d")
	require.True(t, ok)
	assert.Equal(t, ActionExit, action)
}

func TestModel_CtrlDExitsViaBinding(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "ctrl+d should quit via keybinding")
}

func TestModel_CtrlCClearsEditorOnFirstPress(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Add text to editor
	m.editor = m.editor.SetValue("some text")

	model, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = model.(Model)

	// First ctrl+c clears editor, doesn't quit
	assert.NotNil(t, cmd) // timeout cmd for double-press window
	assert.Empty(t, m.editor.Value())
}

func TestModel_EscapeNoOpViaBinding(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	assert.NotNil(t, cmd) // timeout cmd for double-press window

	_ = model
}

func TestModel_CtrlLOpensModelSelectorViaBinding(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	model, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)

	_ = model
}

func TestModel_ExtensionKeybinding(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Register a custom extension keybinding
	customAction := BindingAction("app.test.custom")
	m.bindings.Register(customAction, []string{"ctrl+f"}, "Custom test action")

	action, ok := m.bindings.Resolve("ctrl+f")
	require.True(t, ok)
	assert.Equal(t, customAction, action)

	// Ctrl+f should not reach editor since it's bound
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	assert.Nil(t, cmd, "unhandled action should return nil cmd")

	_ = model
}

func TestModel_OverlayDismissStillWorks(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Activate an overlay
	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}
	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	assert.False(t, m.dialogStack.Empty())

	// ctrl+c should dismiss overlay, not quit
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = model.(Model)
	assert.True(t, m.dialogStack.Empty())
	assert.Nil(t, cmd, "overlay dismiss should not quit")
}

func TestKeyString_V2ModifierCombinations(t *testing.T) {
	tests := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		// Ctrl keys
		{tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, "ctrl+c"},
		{tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}, "ctrl+z"},
		// Alt modifier always produces keystroke form with Keystroke()
		{tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt, Text: "a"}, "alt+a"},
		{tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt}, "alt+a"},
		// Shift modifier with special keys
		{tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, "shift+tab"},
		{tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}, "shift+enter"},
		{tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}, "ctrl+j"},
		// Plain printable chars
		{tea.KeyPressMsg{Code: 'x', Text: "x"}, "x"},
		// Special keys
		{tea.KeyPressMsg{Code: tea.KeyEnter}, "enter"},
		{tea.KeyPressMsg{Code: tea.KeyBackspace}, "backspace"},
		{tea.KeyPressMsg{Code: tea.KeyUp}, "up"},
		{tea.KeyPressMsg{Code: tea.KeyDown}, "down"},
		{tea.KeyPressMsg{Code: tea.KeyPgUp}, "pgup"},
		{tea.KeyPressMsg{Code: tea.KeyHome}, "home"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, keyString(tt.key), "keyString(%+v)", tt.key)
	}
}

func TestModel_V2KeyDispatch_CtrlD(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Ctrl+D via v2 KeyPressMsg should quit
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "ctrl+d should quit via keybinding")
}

func TestModel_V2KeyDispatch_CtrlCClearsEditor(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.editor = m.editor.SetValue("some text")

	// Ctrl+C via v2 KeyPressMsg should clear editor
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = model.(Model)

	assert.NotNil(t, cmd) // timeout cmd for double-press window
	assert.Empty(t, m.editor.Value())
}

func TestModel_V2KeyDispatch_EscapeInterrupts(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Escape via v2 KeyPressMsg (Code = KeyEsc)
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	assert.NotNil(t, cmd) // timeout cmd for double-press window

	_ = model
}

func TestModel_V2KeyDispatch_PrintableChars(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Printable char via v2 KeyPressMsg should go to editor
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = model.(Model)

	assert.NotNil(t, cmd) // virtual cursor blink cmd
	assert.Equal(t, "h", m.editor.Value())
}

func TestModel_V2KeyDispatch_MultiRuneText(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Multi-rune text via v2 KeyPressMsg (e.g., paste)
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "abc"})
	m = model.(Model)

	assert.NotNil(t, cmd) // virtual cursor blink cmd
	assert.Equal(t, "abc", m.editor.Value())
}
