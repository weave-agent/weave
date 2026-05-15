package subagent

import (
	"testing"
	"time"

	"weave/ext/ui/tui"
	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTUIExtAPI records calls made to the TUIExtAPI interface.
type mockTUIExtAPI struct {
	richRenderers map[string]tui.RichToolRenderer
	panelsShown   []tui.PanelConfig
	panelsRemoved []string
}

func newMockTUIExtAPI() *mockTUIExtAPI {
	return &mockTUIExtAPI{
		richRenderers: make(map[string]tui.RichToolRenderer),
	}
}

func (m *mockTUIExtAPI) ShowPanel(config tui.PanelConfig, drawer tui.PanelDrawer) {
	m.panelsShown = append(m.panelsShown, config)
}
func (m *mockTUIExtAPI) HidePanel(id string)                                      {}
func (m *mockTUIExtAPI) RemovePanel(id string)                                    { m.panelsRemoved = append(m.panelsRemoved, id) }
func (m *mockTUIExtAPI) PanelVisible(id string) bool                              { return false }
func (m *mockTUIExtAPI) PanelTray() tui.PanelTrayAPI                              { return nil }
func (m *mockTUIExtAPI) Theme() sdk.ThemeInfo                                     { return sdk.ThemeInfo{} }
func (m *mockTUIExtAPI) Size() (int, int)                                         { return 80, 24 }
func (m *mockTUIExtAPI) EditorText() string                                       { return "" }
func (m *mockTUIExtAPI) SetEditorText(text string)                                {}
func (m *mockTUIExtAPI) PasteToEditor(text string)                                {}
func (m *mockTUIExtAPI) RegisterRichRenderer(tool string, renderer tui.RichToolRenderer) {
	m.richRenderers[tool] = renderer
}
func (m *mockTUIExtAPI) RegisterMessageRenderer(msgType string, renderer sdk.MessageRenderer) {}
func (m *mockTUIExtAPI) SetFooter(component tui.TUIComponent)                                 {}
func (m *mockTUIExtAPI) SetHeader(component tui.TUIComponent)                                 {}
func (m *mockTUIExtAPI) OnTerminalInput(handler func(tui.KeyEvent))                           {}
func (m *mockTUIExtAPI) AddAutocomplete(provider tui.AutocompleteProvider)                    {}
func (m *mockTUIExtAPI) SetWorkingFrames(frames []string, interval time.Duration)             {}
func (m *mockTUIExtAPI) RegisterTheme(name string, theme tui.ThemeDef) error                  { return nil }

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
func (b *mockBus) Off(_ sdk.Handler)    {}
func (b *mockBus) Close() error            { return nil }

func TestSubagentExtension_Name(t *testing.T) {
	ext := &SubagentExtension{}
	assert.Equal(t, "subagent", ext.Name())
}

func TestSubagentExtension_RegisterTUI_NoPanics(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil)}
	api := newMockTUIExtAPI()

	assert.NotPanics(t, func() {
		ext.RegisterTUI(api)
	})
}

func TestSubagentExtension_RegisterTUI_NoRegistrations(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil)}
	api := newMockTUIExtAPI()

	ext.RegisterTUI(api)

	assert.Empty(t, api.richRenderers)
	assert.Empty(t, api.panelsShown)
	assert.Empty(t, api.panelsRemoved)
}

func TestSubagentExtension_Subscribe_ShowsPanelOnStarted(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil)}
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

	// Agent should be tracked
	agent := ext.tracker.Get("agent-123")
	require.NotNil(t, agent)
	assert.Equal(t, AgentRunning, agent.Status)
}

func TestSubagentExtension_Subscribe_IgnoresBadPayload(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil)}
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
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil)}
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
	ext := &SubagentExtension{tracker: NewAgentTracker(50*time.Millisecond, nil)}
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
	assert.Empty(t, api.panelsRemoved)

	// Wait for grace period
	time.Sleep(150 * time.Millisecond)

	// After grace period: agent removed from tracker, panel removed
	assert.Nil(t, ext.tracker.Get("agent-789"))
	require.Len(t, api.panelsRemoved, 1)
	assert.Equal(t, "subagent-agent-789", api.panelsRemoved[0])
}

func TestSubagentExtension_Subscribe_FailedAgent(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(50*time.Millisecond, nil)}
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
	time.Sleep(150 * time.Millisecond)

	assert.Nil(t, ext.tracker.Get("agent-fail"))
	require.Len(t, api.panelsRemoved, 1)
	assert.Equal(t, "subagent-agent-fail", api.panelsRemoved[0])
}

func TestSubagentExtension_Subscribe_MultipleAgents(t *testing.T) {
	ext := &SubagentExtension{tracker: NewAgentTracker(50*time.Millisecond, nil)}
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

	time.Sleep(150 * time.Millisecond)

	// agent-a removed, agent-b still running
	assert.Nil(t, ext.tracker.Get("agent-a"))
	require.NotNil(t, ext.tracker.Get("agent-b"))
	require.Len(t, api.panelsRemoved, 1)
	assert.Equal(t, "subagent-agent-a", api.panelsRemoved[0])
}

func TestSubagentExtension_Subscribe_BeforeRegisterTUI(t *testing.T) {
	// Bus arrives before RegisterTUI — agents should be tracked but
	// panels not shown until API is available.
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil)}
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
	ext := &SubagentExtension{tracker: NewAgentTracker(gracePeriod, nil)}
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
