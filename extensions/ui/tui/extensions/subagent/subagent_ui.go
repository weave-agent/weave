package subagent

import (
	"weave/ext/ui/tui"
	"weave/sdk"
)

func init() {
	tui.RegisterTUIExtension("subagent", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (tui.TUIExtension, error) {
		return &SubagentExtension{}, nil
	})
}

// SubagentExtension is a TUI extension that visualizes running subagents
// as per-agent panels in the panel tray.
type SubagentExtension struct{}

// Name returns the extension name.
func (e *SubagentExtension) Name() string { return "subagent" }

// RegisterTUI wires the subagent panel tracker into the TUI.
func (e *SubagentExtension) RegisterTUI(api tui.TUIExtAPI) {
	// Will be wired in later tasks with bus subscriptions and panel management.
}
