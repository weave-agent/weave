package tui

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"sync"

	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"
)

// BindingAction identifies what a keybinding does.
// The resolver returns an action string and the model's Update method dispatches on it.
type BindingAction string

const (
	ActionExit        BindingAction = "app.exit"
	ActionInterrupt   BindingAction = "app.interrupt"
	ActionModelSelect BindingAction = "app.model.select"
	ActionModelCycle  BindingAction = "app.model.cycle"

	ActionCursorLineStart    BindingAction = "tui.editor.cursorLineStart"
	ActionCursorLineEnd      BindingAction = "tui.editor.cursorLineEnd"
	ActionCursorWordLeft     BindingAction = "tui.editor.cursorWordLeft"
	ActionCursorWordRight    BindingAction = "tui.editor.cursorWordRight"
	ActionEditorNewline      BindingAction = "tui.editor.newline"
	ActionScrollUp           BindingAction = "tui.editor.scrollUp"
	ActionScrollDown         BindingAction = "tui.editor.scrollDown"
	ActionScrollToBottom     BindingAction = "tui.editor.scrollToBottom"
	ActionDeleteWordBackward BindingAction = "tui.editor.deleteWordBackward"
	ActionDeleteWordForward  BindingAction = "tui.editor.deleteWordForward"
	ActionDeleteToLineStart  BindingAction = "tui.editor.deleteToLineStart"
	ActionDeleteToLineEnd    BindingAction = "tui.editor.deleteToLineEnd"
	ActionSuspend            BindingAction = "app.suspend"
	ActionExternalEditor     BindingAction = "app.editor.external"
	ActionToggleToolOutput   BindingAction = "app.tools.expand"
	ActionThinkingCycle      BindingAction = "app.thinking.cycle"
	ActionNewSession         BindingAction = "app.session.new"
	ActionAttachDelete       BindingAction = "app.attachments.delete"
	ActionSandboxCycle       BindingAction = "sandbox.cycle"
	ActionPanelPicker        BindingAction = "app.panel.picker"
	ActionCopySelection      BindingAction = "app.copy.selection"
)

// Binding maps a key sequence to a named action with a description.
type Binding struct {
	Action      BindingAction
	Keys        []string
	Description string
}

// defaultBindings is the built-in keybinding set.
var defaultBindings = []Binding{
	{Action: ActionExit, Keys: []string{"ctrl+d"}, Description: "Exit weave"},
	{Action: ActionInterrupt, Keys: []string{"esc"}, Description: "Interrupt current operation"},
	{Action: ActionModelSelect, Keys: []string{"ctrl+l"}, Description: "Open model selector"},
	{Action: ActionModelCycle, Keys: []string{"ctrl+p"}, Description: "Cycle to next model"},

	// Editor navigation
	{Action: ActionCursorLineStart, Keys: []string{"ctrl+a", "home"}, Description: "Cursor to line start"},
	{Action: ActionCursorLineEnd, Keys: []string{"ctrl+e", "end"}, Description: "Cursor to line end"},
	{Action: ActionCursorWordLeft, Keys: []string{"alt+left", "ctrl+left"}, Description: "Cursor word left"},
	{Action: ActionCursorWordRight, Keys: []string{"alt+right", "ctrl+right"}, Description: "Cursor word right"},
	{Action: ActionEditorNewline, Keys: []string{"shift+enter", "ctrl+j"}, Description: "Insert newline"},
	{Action: ActionScrollUp, Keys: []string{"pgup"}, Description: "Scroll chat up"},
	{Action: ActionScrollDown, Keys: []string{"pgdown"}, Description: "Scroll chat down"},
	{Action: ActionScrollToBottom, Keys: []string{"shift+g"}, Description: "Scroll chat to bottom"},

	// Editor deletion
	{Action: ActionDeleteWordBackward, Keys: []string{"ctrl+w"}, Description: "Delete word backward"},
	{Action: ActionDeleteWordForward, Keys: []string{"alt+d"}, Description: "Delete word forward"},
	{Action: ActionDeleteToLineStart, Keys: []string{"ctrl+u"}, Description: "Delete to line start"},
	{Action: ActionDeleteToLineEnd, Keys: []string{"ctrl+k"}, Description: "Delete to line end"},

	// App control
	{Action: ActionSuspend, Keys: []string{"ctrl+z"}, Description: "Suspend weave"},
	{Action: ActionExternalEditor, Keys: []string{"ctrl+g"}, Description: "Open external editor"},

	// Display
	{Action: ActionToggleToolOutput, Keys: []string{"ctrl+o"}, Description: "Expand/collapse tool output"},
	{Action: ActionThinkingCycle, Keys: []string{"shift+tab"}, Description: "Cycle thinking level"},

	// Session
	{Action: ActionNewSession, Keys: []string{"ctrl+n"}, Description: "New session"},

	// Attachments
	{Action: ActionAttachDelete, Keys: []string{"ctrl+r"}, Description: "Toggle attachment delete mode"},

	// Panels
	{Action: ActionPanelPicker, Keys: []string{"f6"}, Description: "Open panel picker"},

	// Copy
	{Action: ActionCopySelection, Keys: []string{"ctrl+shift+c"}, Description: "Copy selection to clipboard"},
}

// BindingRegistry manages keybindings with priority resolution:
// user config > extension registrations > built-in defaults.
type BindingRegistry struct {
	mu        sync.RWMutex
	defaults  map[string]BindingAction
	extension map[string]BindingAction
	user      map[string]BindingAction
	actions   map[BindingAction]Binding // metadata for help text
}

// NewBindingRegistry creates a registry with built-in defaults.
func NewBindingRegistry() *BindingRegistry {
	r := &BindingRegistry{
		defaults:  make(map[string]BindingAction),
		extension: make(map[string]BindingAction),
		user:      make(map[string]BindingAction),
		actions:   make(map[BindingAction]Binding),
	}

	for _, b := range defaultBindings {
		for _, k := range b.Keys {
			r.defaults[k] = b.Action
		}

		r.actions[b.Action] = b
	}

	return r
}

// Register adds an extension keybinding. Overwrites any previous binding
// for the same action. Not safe for concurrent use with Resolve.
func (r *BindingRegistry) Register(action BindingAction, keys []string, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove old keys for this action
	var oldKeys []string

	for k, a := range r.extension {
		if a == action {
			oldKeys = append(oldKeys, k)
		}
	}

	for _, k := range oldKeys {
		delete(r.extension, k)
	}

	for _, k := range keys {
		r.extension[k] = action
	}

	r.actions[action] = Binding{Action: action, Keys: keys, Description: description}
}

// Resolve returns the action for a key press, applying priority:
// user config > extension > default. Returns ("", false) if no match.
func (r *BindingRegistry) Resolve(key string) (BindingAction, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if a, ok := r.user[key]; ok {
		return a, true
	}

	if a, ok := r.extension[key]; ok {
		return a, true
	}

	if a, ok := r.defaults[key]; ok {
		return a, true
	}

	return "", false
}

// LoadUserConfig reads keybinding overrides from a YAML file.
// Format: {"keybindings": {"app.model.cycle": ["ctrl+p"]}}
func (r *BindingRegistry) LoadUserConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("read keybindings config: %w", err)
	}

	var cfg struct {
		Keybindings map[string][]string `yaml:"keybindings"`
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse keybindings config: %w", err)
	}

	// Build new map in local variable so a validation error doesn't
	// wipe out the previous user bindings.
	newUser := make(map[string]BindingAction)

	for action, keys := range cfg.Keybindings {
		a := BindingAction(action)

		if _, ok := r.actions[a]; !ok {
			return fmt.Errorf("unknown keybinding action %q in config %s", action, path)
		}

		for _, k := range keys {
			newUser[k] = a
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.user = newUser

	return nil
}

// AllBindings returns all active bindings sorted by action name.
func (r *BindingRegistry) AllBindings() []Binding {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Build merged key map: defaults < extension < user
	merged := make(map[string]BindingAction)
	maps.Copy(merged, r.defaults)
	maps.Copy(merged, r.extension)
	maps.Copy(merged, r.user)

	// Collect unique actions
	seen := make(map[BindingAction]bool)

	var result []Binding

	for _, a := range merged {
		if seen[a] {
			continue
		}

		seen[a] = true

		if b, ok := r.actions[a]; ok {
			result = append(result, b)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Action < result[j].Action
	})

	return result
}

// keyString converts a tea.KeyPressMsg to the keystroke representation used in bindings.
// Uses Keystroke() rather than String() so that modifier+printable combos like
// alt+d and shift+g produce "alt+d" and "shift+g" instead of just "d" or "G".
func keyString(msg tea.KeyPressMsg) string {
	return msg.Keystroke()
}

// loadKeybindings finds and loads the user keybindings config.
// Searches from the config file's directory up through .weave/ directories.
func loadKeybindings(configPath string) string {
	if configPath != "" {
		dir := filepath.Dir(configPath)

		// Check next to the config file (works for .weave/settings.json)
		candidate := filepath.Join(dir, "keybindings.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		// Check .weave/ subdirectory (works when config is .weave/settings.json)
		weaveDir := filepath.Join(dir, ".weave", "keybindings.yaml")
		if _, err := os.Stat(weaveDir); err == nil {
			return weaveDir
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	global := filepath.Join(home, ".weave", "keybindings.yaml")
	if _, err := os.Stat(global); err == nil {
		return global
	}

	return ""
}
