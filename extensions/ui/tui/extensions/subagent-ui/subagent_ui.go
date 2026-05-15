package subagent

import (
	"strings"
	"sync"
	"time"

	"weave/ext/ui/tui"
	"weave/sdk"
)

const gracePeriod = 3 * time.Second

// SubagentExtension is a TUI extension that visualizes running subagents
// as per-agent panels in the panel tray.
type SubagentExtension struct {
	mu        sync.Mutex
	api       tui.TUIExtAPI
	tracker   *AgentTracker
	renderer  *subagentRenderer
	done      chan struct{}
	closeOnce sync.Once
	tickOnce  sync.Once
}

func init() {
	ext := &SubagentExtension{
		tracker:  NewAgentTracker(gracePeriod, nil),
		renderer: &subagentRenderer{},
		done:     make(chan struct{}),
	}

	sdk.OnBusReady(func(bus sdk.Bus) {
		ext.subscribe(bus)
	})

	tui.RegisterTUIExtension("subagent-ui", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (tui.TUIExtension, error) {
		return ext, nil
	})
}

// Name returns the extension name.
func (e *SubagentExtension) Name() string { return "subagent-ui" }

// RegisterTUI stores the TUI API and wires the tracker's remove callback
// to call RemovePanel when the grace period expires.
func (e *SubagentExtension) RegisterTUI(api tui.TUIExtAPI) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.api = api
	e.tracker.SetOnRemove(func(id string) {
		e.mu.Lock()
		a := e.api
		e.mu.Unlock()

		if a != nil {
			a.RemovePanel("subagent-" + id)
		}
	})

	// Register rich renderer for all subagent tools (built-in and custom).
	for _, name := range sdk.ListTools() {
		if strings.HasPrefix(name, "subagent_") {
			api.RegisterRichRenderer(name, e.renderer)
		}
	}

	// Start background ticker once to update elapsed time in panels.
	e.tickOnce.Do(func() {
		go e.tickLoop()
	})
}

// Close stops all grace-period timers and releases resources. Safe to call
// multiple times.
func (e *SubagentExtension) Close() {
	e.closeOnce.Do(func() {
		if e.done != nil {
			close(e.done)
		}
	})

	e.mu.Lock()
	e.api = nil
	e.mu.Unlock()
	e.tracker.Close()
}

// tickLoop ticks every second and requests a TUI redraw while agents are
// tracked so elapsed time updates are visible.
func (e *SubagentExtension) tickLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.mu.Lock()
			api := e.api
			e.mu.Unlock()

			if api != nil && len(e.tracker.List()) > 0 {
				api.RequestRedraw()
			}
		case <-e.done:
			return
		}
	}
}

// subscribe sets up bus event handlers for subagent lifecycle events.
// Uses a single OnAll handler so started and done are processed in order.
func (e *SubagentExtension) subscribe(bus sdk.Bus) {
	bus.OnAll(func(ev sdk.Event) error {
		switch ev.Topic {
		case "subagent.started":
			return e.handleStarted(ev)
		case "subagent.done":
			return e.handleDone(ev)
		case "agent.end":
			e.Close()
			return nil
		}

		return nil
	})
}

func (e *SubagentExtension) handleStarted(ev sdk.Event) error {
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

	e.mu.Lock()
	api := e.api
	e.mu.Unlock()

	if api != nil {
		drawer := newAgentPanelDrawer(agent.ID, e.tracker, api.Theme())
		api.ShowPanel(tui.PanelConfig{
			ID:        agent.PanelID,
			Placement: tui.BelowEditor,
			Title:     name,
			Height:    6,
		}, drawer)
	}

	return nil
}

func (e *SubagentExtension) handleDone(ev sdk.Event) error {
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

	e.mu.Lock()
	api := e.api
	e.mu.Unlock()

	if api != nil {
		api.RequestRedraw()
	}

	return nil
}
