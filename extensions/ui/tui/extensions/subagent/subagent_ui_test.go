package subagent

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"weave/ext/ui/tui"
	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check that mockTUIExtAPI satisfies tui.TUIExtAPI.
var _ tui.TUIExtAPI = (*mockTUIExtAPI)(nil)

// mockTUIExtAPI records calls made to the TUIExtAPI interface.
type mockTUIExtAPI struct {
	mu            sync.Mutex
	richRenderers map[string]tui.RichToolRenderer
	panelsShown   []tui.PanelConfig
	panelDrawers  []tui.PanelDrawer
	panelsRemoved []string
	removeCh      chan string
}

func newMockTUIExtAPI() *mockTUIExtAPI {
	return &mockTUIExtAPI{
		richRenderers: make(map[string]tui.RichToolRenderer),
		removeCh:      make(chan string, 10),
	}
}

func (m *mockTUIExtAPI) ShowPanel(config tui.PanelConfig, drawer tui.PanelDrawer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.panelsShown = append(m.panelsShown, config)
	m.panelDrawers = append(m.panelDrawers, drawer)
}

func (m *mockTUIExtAPI) HidePanel(id string) {}

func (m *mockTUIExtAPI) RemovePanel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.panelsRemoved = append(m.panelsRemoved, id)
	if m.removeCh != nil {
		select {
		case m.removeCh <- id:
		default:
		}
	}
}
func (m *mockTUIExtAPI) PanelVisible(id string) bool { return false }
func (m *mockTUIExtAPI) PanelTray() tui.PanelTrayAPI { return nil }
func (m *mockTUIExtAPI) Theme() sdk.ThemeInfo        { return sdk.ThemeInfo{} }
func (m *mockTUIExtAPI) Size() (int, int)            { return 80, 24 }
func (m *mockTUIExtAPI) EditorText() string          { return "" }
func (m *mockTUIExtAPI) SetEditorText(text string)   {}
func (m *mockTUIExtAPI) PasteToEditor(text string)   {}
func (m *mockTUIExtAPI) RegisterRichRenderer(tool string, renderer tui.RichToolRenderer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.richRenderers[tool] = renderer
}

func (m *mockTUIExtAPI) getPanelsRemoved() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := make([]string, len(m.panelsRemoved))
	copy(cp, m.panelsRemoved)

	return cp
}

func (m *mockTUIExtAPI) getPanelsShown() []tui.PanelConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := make([]tui.PanelConfig, len(m.panelsShown))
	copy(cp, m.panelsShown)

	return cp
}

func (m *mockTUIExtAPI) RegisterMessageRenderer(msgType string, renderer sdk.MessageRenderer) {}
func (m *mockTUIExtAPI) SetFooter(component tui.TUIComponent)                                 {}
func (m *mockTUIExtAPI) SetHeader(component tui.TUIComponent)                                 {}
func (m *mockTUIExtAPI) OnTerminalInput(handler func(tui.KeyEvent))                           {}
func (m *mockTUIExtAPI) AddAutocomplete(provider tui.AutocompleteProvider)                    {}
func (m *mockTUIExtAPI) SetWorkingFrames(frames []string, interval time.Duration)             {}
func (m *mockTUIExtAPI) RegisterTheme(name string, theme tui.ThemeDef) error                  { return nil }

// waitForPanelRemovals waits until at least count panels have been removed.
func waitForPanelRemovals(t *testing.T, api *mockTUIExtAPI, count int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		removed := api.getPanelsRemoved()
		if len(removed) >= count {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for %d panel removals, got %d", count, len(api.getPanelsRemoved()))
}

// mockBus records published events and delivers On-subscribed events.
type mockBus struct {
	published []sdk.Event
	handlers  map[string][]sdk.Handler
}

func newMockBus() *mockBus {
	return &mockBus{
		handlers: make(map[string][]sdk.Handler),
	}
}

func (b *mockBus) Publish(ev sdk.Event) {
	b.published = append(b.published, ev)
	// Deliver to subscribers
	for _, h := range b.handlers[ev.Topic] {
		_ = h(ev)
	}
}

func (b *mockBus) On(topic string, h sdk.Handler) {
	b.handlers[topic] = append(b.handlers[topic], h)
}

func (b *mockBus) OnAll(_ sdk.Handler) {}
func (b *mockBus) Off(_ sdk.Handler)   {}
func (b *mockBus) Close() error        { return nil }

func TestSubagentExtension_Name(t *testing.T) {
	ext := &SubagentExtension{renderer: &subagentRenderer{}}
	assert.Equal(t, "subagent", ext.Name())
}

func TestSubagentExtension_RegisterTUI_NoRegistrations(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()

	ext.RegisterTUI(api)

	// Rich renderers are registered for built-in agents.
	assert.Len(t, api.richRenderers, 3)
	assert.Empty(t, api.panelsShown)
	assert.Empty(t, api.panelsRemoved)
}

func TestSubagentExtension_Subscribe_ShowsPanelOnStarted(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id":   "agent-123",
		"name": "researcher",
		"mode": "background",
	}))

	require.Len(t, api.panelsShown, 1)
	assert.Equal(t, "subagent-agent-123", api.panelsShown[0].ID)
	assert.Equal(t, "researcher", api.panelsShown[0].Title)
	assert.Equal(t, tui.BelowEditor, api.panelsShown[0].Placement)
	assert.Equal(t, 6, api.panelsShown[0].Height)
	assert.NotNil(t, api.panelDrawers[0], "panel drawer should be non-nil")

	// Agent should be tracked
	agent := ext.tracker.Get("agent-123")
	require.NotNil(t, agent)
	assert.Equal(t, AgentRunning, agent.Status)
}

func TestSubagentExtension_Subscribe_IgnoresBadPayload(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// Non-map payload
	bus.Publish(sdk.NewEvent("subagent.started", "bad"))
	assert.Empty(t, api.panelsShown)

	// Missing id
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"name": "researcher",
	}))
	assert.Empty(t, api.panelsShown)
}

func TestSubagentExtension_Subscribe_DoneUpdatesTracker(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// Start an agent
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id":   "agent-456",
		"name": "planner",
		"mode": "background",
	}))

	// Agent is running
	agent := ext.tracker.Get("agent-456")
	require.NotNil(t, agent)
	assert.Equal(t, AgentRunning, agent.Status)

	// Complete the agent
	bus.Publish(sdk.NewEvent("subagent.done", map[string]string{
		"id":      "agent-456",
		"status":  "completed",
		"content": "task complete",
	}))

	// Agent status updated but still in tracker during grace period
	agent = ext.tracker.Get("agent-456")
	require.NotNil(t, agent)
	assert.Equal(t, AgentCompleted, agent.Status)
	assert.Equal(t, "task complete", agent.Result)
}

func TestSubagentExtension_Subscribe_FullLifecycle(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(50*time.Millisecond, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// Start
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id":   "agent-789",
		"name": "coder",
		"mode": "background",
	}))

	require.Len(t, api.panelsShown, 1)
	assert.Equal(t, "subagent-agent-789", api.panelsShown[0].ID)

	// Complete
	bus.Publish(sdk.NewEvent("subagent.done", map[string]string{
		"id":      "agent-789",
		"status":  "completed",
		"content": "done",
	}))

	// During grace period: agent still tracked, no removal yet
	assert.NotNil(t, ext.tracker.Get("agent-789"))
	assert.Empty(t, api.getPanelsRemoved())

	// Wait for grace period to expire
	waitForPanelRemovals(t, api, 1)

	// After grace period: agent removed from tracker, panel removed
	assert.Nil(t, ext.tracker.Get("agent-789"))

	removed := api.getPanelsRemoved()
	require.Len(t, removed, 1)
	assert.Equal(t, "subagent-agent-789", removed[0])
}

func TestSubagentExtension_Subscribe_FailedAgent(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(50*time.Millisecond, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// Start
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id":   "agent-fail",
		"name": "explorer",
		"mode": "background",
	}))

	// Fail
	bus.Publish(sdk.NewEvent("subagent.done", map[string]string{
		"id":      "agent-fail",
		"status":  "failed",
		"content": "error occurred",
	}))

	agent := ext.tracker.Get("agent-fail")
	require.NotNil(t, agent)
	assert.Equal(t, AgentFailed, agent.Status)

	// Wait for grace period
	waitForPanelRemovals(t, api, 1)

	assert.Nil(t, ext.tracker.Get("agent-fail"))

	removed := api.getPanelsRemoved()
	require.Len(t, removed, 1)
	assert.Equal(t, "subagent-agent-fail", removed[0])
}

func TestSubagentExtension_Subscribe_MultipleAgents(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(50*time.Millisecond, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// Start two agents
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id": "agent-a", "name": "alpha", "mode": "background",
	}))
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id": "agent-b", "name": "beta", "mode": "background",
	}))

	require.Len(t, api.panelsShown, 2)
	assert.Equal(t, "subagent-agent-a", api.panelsShown[0].ID)
	assert.Equal(t, "subagent-agent-b", api.panelsShown[1].ID)

	// Complete only agent-a
	bus.Publish(sdk.NewEvent("subagent.done", map[string]string{
		"id": "agent-a", "status": "completed", "content": "done",
	}))

	waitForPanelRemovals(t, api, 1)

	// agent-a removed, agent-b still running
	assert.Nil(t, ext.tracker.Get("agent-a"))
	require.NotNil(t, ext.tracker.Get("agent-b"))
	require.Len(t, api.getPanelsRemoved(), 1)
	assert.Equal(t, "subagent-agent-a", api.getPanelsRemoved()[0])
}

func TestSubagentExtension_Subscribe_BeforeRegisterTUI(t *testing.T) {
	// Bus arrives before RegisterTUI — agents should be tracked but
	// panels not shown until API is available.
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil), renderer: &subagentRenderer{}}
	bus := newMockBus()

	ext.subscribe(bus)

	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id": "agent-early", "name": "early", "mode": "background",
	}))

	// Agent tracked but no panel shown (no API yet)
	agent := ext.tracker.Get("agent-early")
	require.NotNil(t, agent)
	assert.Equal(t, AgentRunning, agent.Status)

	// Now wire API
	api := newMockTUIExtAPI()
	ext.RegisterTUI(api)

	// Panel was NOT shown retroactively — only new agents get panels
	assert.Empty(t, api.panelsShown)
}

func TestSubagentExtension_Subscribe_DoneIgnoresBadPayload(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// Start an agent first
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id": "agent-x", "name": "test", "mode": "background",
	}))

	// Bad done payload — should not crash
	assert.NotPanics(t, func() {
		bus.Publish(sdk.NewEvent("subagent.done", "bad"))
	})

	// Missing id — should not affect existing agent
	assert.NotPanics(t, func() {
		bus.Publish(sdk.NewEvent("subagent.done", map[string]string{
			"status": "completed",
		}))
	})

	// Agent still running
	agent := ext.tracker.Get("agent-x")
	require.NotNil(t, agent)
	assert.Equal(t, AgentRunning, agent.Status)
}

func TestSubagentExtension_Subscribe_DoneIgnoresEmptyID(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// Start an agent first
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id": "agent-x", "name": "test", "mode": "background",
	}))

	// Empty id — should be ignored, agent still running
	bus.Publish(sdk.NewEvent("subagent.done", map[string]string{
		"id": "", "status": "completed", "content": "done",
	}))

	agent := ext.tracker.Get("agent-x")
	require.NotNil(t, agent)
	assert.Equal(t, AgentRunning, agent.Status)
}

func TestSubagentExtension_Close_ClearsAPIAndTracker(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()

	ext.RegisterTUI(api)
	require.NotNil(t, ext.api)

	// Start some agents
	ext.tracker.Start("agent-1", "test", "background")
	ext.tracker.Start("agent-2", "test", "background")
	assert.Len(t, ext.tracker.List(), 2)

	ext.Close()

	assert.Nil(t, ext.api)
	assert.Empty(t, ext.tracker.List())
}

func TestSubagentExtension_Close_Idempotent(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()

	ext.RegisterTUI(api)

	assert.NotPanics(t, func() {
		ext.Close()
		ext.Close()
		ext.Close()
	})
}

func TestSubagentExtension_AgentEnd_TriggersCleanup(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(50*time.Millisecond, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// Start two agents
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id": "agent-a", "name": "alpha", "mode": "background",
	}))
	bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
		"id": "agent-b", "name": "beta", "mode": "background",
	}))

	require.Len(t, ext.tracker.List(), 2)
	require.NotNil(t, ext.api)

	// Simulate TUI shutdown
	bus.Publish(sdk.NewEvent("agent.end", nil))

	// All agents should be cleaned up, API cleared
	assert.Empty(t, ext.tracker.List())
	assert.Nil(t, ext.api)

	// Grace-period timers were canceled — no panels should be removed.
	assert.Empty(t, api.getPanelsRemoved())
}

func TestSubagentExtension_NoPanelLeak_OnDone(t *testing.T) {
	// Verify that every ShowPanel has a corresponding RemovePanel after
	// agents complete through the grace period.
	ext := &SubagentExtension{tracker: NewAgentTracker(50*time.Millisecond, nil), renderer: &subagentRenderer{}}
	api := newMockTUIExtAPI()
	bus := newMockBus()

	ext.RegisterTUI(api)
	ext.subscribe(bus)

	// Start and complete 3 agents
	for i := range 3 {
		id := fmt.Sprintf("agent-%c", 'a'+i)
		bus.Publish(sdk.NewEvent("subagent.started", map[string]string{
			"id": id, "name": "agent", "mode": "background",
		}))
		bus.Publish(sdk.NewEvent("subagent.done", map[string]string{
			"id": id, "status": "completed", "content": "done",
		}))
	}

	require.Len(t, api.panelsShown, 3)

	// Wait for all grace periods to expire
	waitForPanelRemovals(t, api, 3)

	// Every shown panel should have been removed
	removed := api.getPanelsRemoved()
	require.Len(t, removed, 3)

	shownIDs := make(map[string]bool)
	for _, p := range api.getPanelsShown() {
		shownIDs[p.ID] = true
	}

	for _, id := range removed {
		assert.True(t, shownIDs[id], "removed panel %s was never shown", id)
	}

	// No agents left in tracker
	assert.Empty(t, ext.tracker.List())
}
