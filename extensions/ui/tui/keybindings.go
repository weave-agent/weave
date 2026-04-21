package tui

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

// BindingAction identifies what a keybinding does.
// The resolver returns an action string and the model's Update method dispatches on it.
type BindingAction string

const (
	ActionExit        BindingAction = "app.exit"
	ActionClear       BindingAction = "app.clear"
	ActionInterrupt   BindingAction = "app.interrupt"
	ActionModelSelect BindingAction = "app.model.select"
	ActionModelCycle  BindingAction = "app.model.cycle"
	ActionToolExpand  BindingAction = "app.tool.expand"
	ActionThinkToggle BindingAction = "app.thinking.toggle"
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
	{Action: ActionClear, Keys: []string{"ctrl+c"}, Description: "Clear/quit"},
	{Action: ActionInterrupt, Keys: []string{"escape"}, Description: "Interrupt current operation"},
	{Action: ActionModelSelect, Keys: []string{"ctrl+l"}, Description: "Open model selector"},
	{Action: ActionModelCycle, Keys: []string{"ctrl+p"}, Description: "Cycle to next model"},
	{Action: ActionToolExpand, Keys: []string{"ctrl+o"}, Description: "Toggle tool output expand"},
	{Action: ActionThinkToggle, Keys: []string{"ctrl+t"}, Description: "Toggle thinking block"},
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

	r.mu.Lock()
	defer r.mu.Unlock()

	r.user = make(map[string]BindingAction)

	for action, keys := range cfg.Keybindings {
		a := BindingAction(action)
		for _, k := range keys {
			r.user[k] = a
		}
	}

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

// keyString converts a tea.KeyMsg to the string representation used in bindings.
func keyString(msg tea.KeyMsg) string {
	s := msg.String()
	if s == "esc" {
		return "escape"
	}
	return s
}

// loadKeybindings finds and loads the user keybindings config.
// Searches from the config file's directory up through .weave/ directories.
func loadKeybindings(configPath string) string {
	if configPath != "" {
		dir := filepath.Dir(configPath)

		candidate := filepath.Join(dir, "keybindings.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
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
