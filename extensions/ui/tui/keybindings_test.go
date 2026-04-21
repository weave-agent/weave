package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"weave/ext/ui/tui/components/messages"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindingRegistry_Defaults(t *testing.T) {
	r := NewBindingRegistry()

	actions := map[string]BindingAction{
		"ctrl+d": ActionExit,
		"ctrl+c": ActionClear,
		"escape": ActionInterrupt,
		"ctrl+l": ActionModelSelect,
		"ctrl+p": ActionModelCycle,
	}

	for key, want := range actions {
		action, ok := r.Resolve(key)
		assert.True(t, ok, "expected default binding for %s", key)
		assert.Equal(t, want, action, "wrong action for key %s", key)
	}
}

func TestBindingRegistry_UnknownKey(t *testing.T) {
	r := NewBindingRegistry()

	_, ok := r.Resolve("ctrl+z")
	assert.False(t, ok)
}

func TestBindingRegistry_ExtensionOverridesDefault(t *testing.T) {
	r := NewBindingRegistry()

	// Register ctrl+c for a custom action
	r.Register("app.custom", []string{"ctrl+c"}, "Custom action")

	action, ok := r.Resolve("ctrl+c")
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

	// Should contain all 5 default actions
	names := make(map[BindingAction]bool)
	for _, b := range bindings {
		names[b.Action] = true
	}

	assert.True(t, names[ActionExit])
	assert.True(t, names[ActionClear])
	assert.True(t, names[ActionInterrupt])
	assert.True(t, names[ActionModelSelect])
	assert.True(t, names[ActionModelCycle])
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
		key  tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyCtrlC}, "ctrl+c"},
		{tea.KeyMsg{Type: tea.KeyCtrlD}, "ctrl+d"},
		{tea.KeyMsg{Type: tea.KeyCtrlL}, "ctrl+l"},
		{tea.KeyMsg{Type: tea.KeyCtrlP}, "ctrl+p"},
		{tea.KeyMsg{Type: tea.KeyEsc}, "escape"},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}, "a"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, keyString(tt.key), "keyString(%v)", tt.key)
	}
}

func TestKeyString_EscapeNormalization(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	assert.Equal(t, "escape", keyString(msg))
}

func TestLoadKeybindings_ProjectConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".weave.yaml")

	// No keybindings file -> empty
	result := loadKeybindings(cfgPath)
	assert.Equal(t, "", result)
}

func TestLoadKeybindings_NearConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".weave.yaml")
	kbPath := filepath.Join(dir, "keybindings.yaml")

	err := os.WriteFile(kbPath, []byte("keybindings: {}\n"), 0o644)
	require.NoError(t, err)

	result := loadKeybindings(cfgPath)
	assert.Equal(t, kbPath, result)
}

func TestLoadKeybindings_EmptyConfigPath(t *testing.T) {
	result := loadKeybindings("")
	assert.Equal(t, "", result)
}

func TestModel_BindingsRegistryInitialized(t *testing.T) {
	m := newModel(nil, nil, nil)
	assert.NotNil(t, m.bindings)

	// Default keybinding should work
	action, ok := m.bindings.Resolve("ctrl+d")
	require.True(t, ok)
	assert.Equal(t, ActionExit, action)
}

func TestModel_CtrlDExitsViaBinding(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "ctrl+d should quit via keybinding")
}

func TestModel_CtrlCClearsChatViaBinding(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	// Add some state
	m.prompted = true
	m.chat = m.chat.AddItem(messages.NewAssistantMessage())
	m.toolPanels["test"] = messages.NewToolPanel("test", "tool", "")

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = model.(Model)

	// Ctrl+C should clear chat, not quit
	assert.Nil(t, cmd)
	assert.False(t, m.prompted)
	assert.Empty(t, m.toolPanels)
}

func TestModel_EscapeNoOpViaBinding(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Nil(t, cmd)
	_ = model
}

func TestModel_CtrlLOpensModelSelectorViaBinding(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	require.NotNil(t, cmd)
	_ = model
}

func TestModel_ExtensionKeybinding(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	// Register a custom extension keybinding
	customAction := BindingAction("app.test.custom")
	m.bindings.Register(customAction, []string{"ctrl+f"}, "Custom test action")

	action, ok := m.bindings.Resolve("ctrl+f")
	require.True(t, ok)
	assert.Equal(t, customAction, action)

	// Ctrl+f should not reach editor since it's bound
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	assert.Nil(t, cmd, "unhandled action should return nil cmd")
	_ = model
}

func TestModel_OverlayDismissStillWorks(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	// Activate an overlay
	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}
	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.Equal(t, overlaySession, m.activeOverlay)

	// ctrl+c should dismiss overlay, not quit
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = model.(Model)
	assert.Equal(t, overlayNone, m.activeOverlay)
	assert.Nil(t, cmd, "overlay dismiss should not quit")
}
