package subagent

import (
	"time"

	"weave/ext/ui/tui"
	"weave/sdk"
)

const gracePeriod = 3 * time.Second

// SubagentExtension is a TUI extension that visualizes running subagents
// as per-agent panels in the panel tray.
type SubagentExtension struct {
	api      tui.TUIExtAPI
	tracker  *AgentTracker
	renderer *subagentRenderer
}

func init() {
	ext := &SubagentExtension{
		tracker:  NewAgentTracker(gracePeriod, nil),
		renderer: &subagentRenderer{},
	}

	sdk.OnBusReady(func(bus sdk.Bus) {
		ext.subscribe(bus)
	})

	tui.RegisterTUIExtension("subagent", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (tui.TUIExtension, error) {
		return ext, nil
	})
}

// Name returns the extension name.
func (e *SubagentExtension) Name() string { return "subagent" }

// RegisterTUI stores the TUI API and wires the tracker's remove callback
// to call RemovePanel when the grace period expires.
func (e *SubagentExtension) RegisterTUI(api tui.TUIExtAPI) {
	e.api = api
	e.tracker.onRemove = func(id string) {
		if e.api != nil {
			e.api.RemovePanel("subagent-" + id)
		}
	}

	// Register rich renderer for known built-in agents.
	for _, name := range []string{"general", "explore", "plan"} {
		api.RegisterRichRenderer("subagent_"+name, e.renderer)
	}
}

// Close stops all grace-period timers and releases resources. Safe to call
// multiple times.
func (e *SubagentExtension) Close() {
	e.api = nil
	e.tracker.Close()
}

// subscribe sets up bus event handlers for subagent lifecycle events.
func (e *SubagentExtension) subscribe(bus sdk.Bus) {
	bus.On("subagent.started", func(ev sdk.Event) error {
		payload, ok := ev.Payload.(map[string]string)
		if !ok {
			return nil
		}

		id := payload["id"]
		name := payload["name"]
		mode := payload["mode"]
		if id == "" {
			return nil
		}

		agent := e.tracker.Start(id, name, mode)

		if e.api != nil {
			// Register renderer for custom agents not covered by built-ins.
			e.api.RegisterRichRenderer("subagent_"+name, e.renderer)

			drawer := newAgentPanelDrawer(agent.ID, e.tracker, e.api.Theme())
			e.api.ShowPanel(tui.PanelConfig{
				ID:        agent.PanelID,
				Placement: tui.BelowEditor,
				Title:     name,
				Height:    6,
			}, drawer)
		}

		return nil
	})

	bus.On("subagent.done", func(ev sdk.Event) error {
		payload, ok := ev.Payload.(map[string]string)
		if !ok {
			return nil
		}

		id := payload["id"]
		status := payload["status"]
		result := payload["content"]
		if id == "" {
			return nil
		}

		e.tracker.Done(id, status, result)

		return nil
	})

	// Clean up when the TUI shuts down.
	bus.On("agent.end", func(_ sdk.Event) error {
		e.Close()
		return nil
	})
}
