package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weave-agent/weave-tui/components"
	"github.com/weave-agent/weave-tui/components/messages"
	"github.com/weave-agent/weave-tui/components/overlays"
	"github.com/weave-agent/weave/bus"
	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_FullStreamingFlow exercises the complete streaming pipeline:
// landing state -> prompt -> streaming deltas with token rate -> tool calls ->
// tool results -> turn end with scroll indicator -> overlay dismissal.
func TestIntegration_FullStreamingFlow(t *testing.T) {
	b := bus.New()
	defer b.Close()

	m := newModel(b, nil, nil, nil)
	m.width = 120
	m.height = 40

	// 1. Landing state is shown initially
	require.True(t, m.showLanding, "landing should be shown initially")
	view := m.View()
	assert.Contains(t, view.Content, "█████", "landing logo should be visible")

	// 2. Submit prompt — hides landing, publishes prompt event
	model, _ := m.onSubmit("explain Go")
	m = model.(Model)
	assert.False(t, m.showLanding, "landing should hide after first submit")
	assert.True(t, m.prompted)

	// 3. Turn starts — spinner appears
	model, _ = m.Update(TurnStartMsg{Turn: 1})
	m = model.(Model)

	// 4. Message starts — creates assistant message
	model, _ = m.Update(MessageStartMsg{})
	m = model.(Model)
	items := m.chat.Items()
	require.Len(t, items, 2) // user + assistant
	am, ok := items[1].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.True(t, am.IsStreaming())

	// 5. Stream deltas with token rate
	model, _ = m.Update(MessageUpdateMsg{Content: "Go is ", TokenRate: 50.0})
	m = model.(Model)
	assert.InDelta(t, 50.0, m.footer.TokenRate(), 0.01)

	model, _ = m.Update(MessageUpdateMsg{Content: "a great language.", TokenRate: 75.5})
	m = model.(Model)
	assert.InDelta(t, 75.5, m.footer.TokenRate(), 0.01)

	// 6. Message ends with tool calls — creates tool panels, clears token rate
	model, _ = m.Update(MessageEndMsg{
		Content: "Go is a great language.",
		ToolCalls: []sdk.ToolCall{
			{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "go version"}},
		},
	})
	m = model.(Model)
	assert.InDelta(t, 0.0, m.footer.TokenRate(), 0.001)

	items = m.chat.Items()
	require.Len(t, items, 3) // user + assistant + tool panel

	tp, ok := items[2].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Equal(t, messages.ToolPending, tp.State())

	// 7. Tool result arrives
	model, _ = m.Update(ToolResultMsg{
		ToolID: "tc1",
		Tool:   "bash",
		Result: sdk.ToolResult{Content: "go version go1.22.0", IsError: false},
	})
	m = model.(Model)

	tp = m.chat.Items()[2].(*messages.ToolPanel)
	assert.Equal(t, messages.ToolSuccess, tp.State())

	// 8. Turn ends — spinner hides
	model, _ = m.Update(TurnEndMsg{})
	m = model.(Model)
	assert.False(t, m.spinner.Visible())

	// 9. Draw into screen buffer — verify all content renders
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	assert.Contains(t, rendered, "explain Go", "user prompt should be in rendered output")
	assert.Contains(t, rendered, "Go is a great language", "assistant response should be in rendered output")
	assert.Contains(t, rendered, "go version", "tool result should be in rendered output")
}

// TestIntegration_OverlayStackWithStreaming verifies that the overlay stack
// correctly intercepts keys during streaming and that canceling returns to normal.
func TestIntegration_OverlayStackWithStreaming(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	// Start streaming
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)
	model, _ = m.Update(MessageUpdateMsg{Content: "streaming..."})
	m = model.(Model)

	// Activate a dialog (session selector)
	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}
	model, _ = m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// Dialog should take over rendering
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "Resume Session")

	// Ctrl+C should dismiss dialog (not quit)
	model, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = model.(Model)
	assert.True(t, m.dialogStack.Empty())

	// Streaming should still be active
	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.True(t, am.IsStreaming())
}

// TestIntegration_ProgressiveMarkdownStreaming verifies progressive rendering:
// streaming text accumulates through Append, then Finalize produces full render.
func TestIntegration_ProgressiveMarkdownStreaming(t *testing.T) {
	m := newModelNoLanding()
	m.width = 120
	m.height = 30
	m.chat = m.chat.SetSize(120, 20)

	// Start message
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	// Stream markdown content progressively
	codeBlock := "```go\nfunc main() {\n    fmt.Println"
	model, _ = m.Update(MessageUpdateMsg{Content: codeBlock})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "(\"hello\")\n}\n```"})
	m = model.(Model)

	// While streaming, content should be present
	items := m.chat.Items()
	am := items[0].(*messages.AssistantMessage)
	assert.True(t, am.IsStreaming())
	assert.Contains(t, am.Content(), "func main()")

	// Finalize
	model, _ = m.Update(MessageEndMsg{Content: codeBlock + "(\"hello\")\n}\n```"})
	m = model.(Model)

	am = m.chat.Items()[0].(*messages.AssistantMessage)
	assert.False(t, am.IsStreaming())
	assert.Contains(t, am.Content(), "func main()")

	// Render into screen buffer
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := ansi.Strip(canvas.Render())
	assert.Contains(t, rendered, "func main()")
}

// TestIntegration_TokenRateDisplayAndAutoScroll exercises token rate tracking
// and the auto-scroll/turn-end indicator flow.
func TestIntegration_TokenRateDisplayAndAutoScroll(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 15
	m.chat = m.chat.SetSize(80, 5) // small viewport to enable scrolling

	// Add enough items to make chat scrollable
	for i := range 10 {
		m.chat = m.chat.AddItem(stubItem{text: fmt.Sprintf("line %d content here", i)})
	}

	// Scroll up so we're not near bottom (beyond 3-line threshold)
	m.chat = m.chat.ScrollUp(8)
	require.False(t, m.chat.AtBottom())

	// Start streaming with token rate
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "response", TokenRate: 42.5})
	m = model.(Model)
	assert.InDelta(t, 42.5, m.footer.TokenRate(), 0.01)

	// End message — token rate should clear
	model, _ = m.Update(MessageEndMsg{Content: "response"})
	m = model.(Model)
	assert.InDelta(t, 0.0, m.footer.TokenRate(), 0.001)

	// Turn end while scrolled up — should set indicator
	model, _ = m.Update(TurnEndMsg{})
	m = model.(Model)
	assert.True(t, m.chat.TurnEndPending(), "turn end indicator should be set when scrolled up")

	// Scroll to bottom should clear indicator
	model, _ = m.dispatchBinding(ActionScrollToBottom)
	m = model.(Model)
	assert.False(t, m.chat.TurnEndPending())
	assert.True(t, m.chat.AtBottom())
}

// TestIntegration_SDKUIThroughOverlayStack verifies all sdk.UI methods
// work correctly through the overlay stack: Select, Confirm, Input, SetStatus, Notify.
func TestIntegration_SDKUIThroughOverlayStack(t *testing.T) {
	b := bus.New()
	defer b.Close()

	commands := NewCommandRegistry(b, "")
	bindings := NewBindingRegistry()
	ui := NewTUIImpl(commands, bindings)

	m := newModel(b, nil, nil, ui)
	m.width = 80
	m.height = 24

	// Test SetStatus
	ui.SetStatus("test", "active")

	model, _ := m.Update(extStatusMsg{key: "test", text: "active"})
	m = model.(Model)
	assert.Equal(t, "active", m.footer.ExtStatus()["test"])

	// Test Notify
	model, _ = m.Update(notifyMsg{message: "notification via UI"})
	m = model.(Model)
	items := m.chat.Items()
	am, ok := items[len(items)-1].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "notification via UI", am.Content())

	// Test Select via popup queue
	sender := &mockSender{}

	ui.SetProgram(sender)

	req := &overlayRequest{
		kind:   requestSelect,
		title:  "Pick an option",
		items:  []string{"alpha", "beta", "gamma"},
		result: make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req))

	model, _ = m.handlePopupPending()
	m = model.(Model)
	require.False(t, m.dialogStack.Empty(), "select dialog should be on stack")

	// Verify select dialog renders
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "Pick an option")

	// Select item
	model, _ = m.Update(overlays.SelectorSelectedMsg{Index: 1, Item: overlays.SelectorItem{Title: "beta"}})
	m = model.(Model)
	assert.True(t, m.dialogStack.Empty())

	select {
	case resp := <-req.result:
		assert.Equal(t, 1, resp.index)
		require.NoError(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	// Test Confirm via popup queue
	req2 := &overlayRequest{
		kind:    requestConfirm,
		message: "Continue?",
		result:  make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req2))

	model, _ = m.handlePopupPending()
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	model, _ = m.Update(overlays.ConfirmResultMsg{Confirmed: true})
	m = model.(Model)
	assert.True(t, m.dialogStack.Empty())

	select {
	case resp := <-req2.result:
		assert.True(t, resp.confirmed)
	default:
		t.Fatal("expected response on result channel")
	}

	// Test Input via popup queue
	req3 := &overlayRequest{
		kind:    requestInput,
		message: "Enter name:",
		result:  make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req3))

	model, _ = m.handlePopupPending()
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	model, _ = m.Update(overlays.InputResultMsg{Value: "test-value", Ok: true})
	m = model.(Model)
	assert.True(t, m.dialogStack.Empty())

	select {
	case resp := <-req3.result:
		assert.Equal(t, "test-value", resp.value)
		require.NoError(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}
}

// TestIntegration_KeybindingPriority verifies the three-layer keybinding priority:
// user config > extension registrations > built-in defaults.
func TestIntegration_KeybindingPriority(t *testing.T) {
	// Create model with custom extension binding
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Built-in: ctrl+d -> exit
	action, ok := m.bindings.Resolve("ctrl+d")
	require.True(t, ok)
	assert.Equal(t, ActionExit, action)

	// Register extension binding that overrides ctrl+d
	m.bindings.Register("app.custom.exit", []string{"ctrl+d"}, "Custom exit")
	action, ok = m.bindings.Resolve("ctrl+d")
	require.True(t, ok)
	assert.Equal(t, BindingAction("app.custom.exit"), action, "extension should override default")

	// Load user config that overrides ctrl+d again
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "keybindings.yaml")
	err := os.WriteFile(cfgPath, []byte("keybindings:\n  app.exit:\n    - ctrl+d\n"), 0o644)
	require.NoError(t, err)
	err = m.bindings.LoadUserConfig(cfgPath)
	require.NoError(t, err)

	action, ok = m.bindings.Resolve("ctrl+d")
	require.True(t, ok)
	assert.Equal(t, ActionExit, action, "user config should override extension")

	// Verify extension binding still works for keys not overridden by user
	m.bindings.Register("app.search", []string{"ctrl+f"}, "Search")
	action, ok = m.bindings.Resolve("ctrl+f")
	require.True(t, ok)
	assert.Equal(t, BindingAction("app.search"), action)

	// Verify defaults still work for keys not overridden
	action, ok = m.bindings.Resolve("ctrl+l")
	require.True(t, ok)
	assert.Equal(t, ActionModelSelect, action)
}

// TestIntegration_SessionResumeFlow verifies the complete session resume flow:
// list sessions -> show selector -> select -> rebuild chat -> publish event.
func TestIntegration_SessionResumeFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicSessionResume)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	sessionID := "aaa11122233344455566677788899900"

	// Create session file
	header := sessionHeader{Type: "session", ID: sessionID, Timestamp: time.Now().UTC(), CWD: "/project"}
	headerJSON, err := json.Marshal(header)
	require.NoError(t, err)

	entries := []string{
		string(headerJSON),
		jsonEntry("user", "first question", nil),
		jsonEntry("assistant", "first answer", nil),
		jsonEntry("user", "follow up", nil),
		jsonEntry("assistant", "follow up answer", nil),
	}

	err = os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(joinLines(entries)), 0o644)
	require.NoError(t, err)

	// Show session list
	sessions := []SessionEntry{
		{ID: sessionID, CWD: "/project", CreatedAt: time.Now()},
	}
	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// Verify dialog renders
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "Resume Session")

	// Select session
	model, cmd := m.Update(overlays.SelectorSelectedMsg{Index: 0, Item: overlays.SelectorItem{
		Title: "/project",
	}})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
	assert.Nil(t, m.pendingSessions)
	assert.True(t, m.prompted)

	// Verify chat was rebuilt
	items := m.chat.Items()
	require.Len(t, items, 4) // user + assistant + user + assistant
	assert.Equal(t, "first question", items[0].(*messages.UserMessage).Content())
	assert.Equal(t, "first answer", items[1].(*messages.AssistantMessage).Content())
	assert.Equal(t, "follow up", items[2].(*messages.UserMessage).Content())
	assert.Equal(t, "follow up answer", items[3].(*messages.AssistantMessage).Content())

	// Execute bus publish cmd
	require.NotNil(t, cmd)
	cmd()

	// Verify bus event
	evt := <-ch
	assert.Equal(t, topicSessionResume, evt.Topic)
	payload, ok := evt.Payload.(sdk.SessionResumePayload)
	require.True(t, ok)
	assert.Equal(t, sessionID, payload.SessionID)
}

// TestIntegration_ModelSelectionFlow verifies model selection and cycling
// through the dialog stack, including footer updates and bus events.
func TestIntegration_ModelSelectionFlow(t *testing.T) {
	sdkmodel.ResetModelRegistry()

	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true})
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "gpt-5.5", Provider: "openai", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	b := bus.New()

	defer b.Close()

	ch := subscribeToChan(b, topicModelChange)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	// Show model list
	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}
	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// Verify dialog renders with model info
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "Select Model")
	assert.Contains(t, rendered, "[anthropic]")

	// Select a model
	model, cmd := m.Update(overlays.SelectorSelectedMsg{Index: 1, Item: overlays.SelectorItem{
		Title: "openai/gpt-5.5", Subtitle: "[openai]",
	}})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
	assert.Equal(t, "openai", m.currentModel.Provider)
	assert.Equal(t, "gpt-5.5", m.currentModel.Model)
	assert.Equal(t, "gpt-5.5", m.footer.ModelName())
	assert.Equal(t, "openai", m.footer.ProviderName())

	// Execute bus publish cmd
	require.NotNil(t, cmd)
	executeBatchCmd(t, cmd)

	// Verify bus event
	evt := <-ch
	assert.Equal(t, topicModelChange, evt.Topic)
	payload, ok := evt.Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "openai", payload["provider"])
	assert.Equal(t, "gpt-5.5", payload["model"])
}

// TestIntegration_LandingToChatAndBack exercises landing state lifecycle:
// shown initially -> hidden after submit -> re-shown on /clear and /new.
func TestIntegration_LandingToChatAndBack(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 30
	m.chat = m.chat.SetSize(80, m.chatHeight(30))

	// Landing shown initially
	require.True(t, m.showLanding)
	view := m.View()
	assert.Contains(t, view.Content, "█████")
	// Horizontal rule should be present in landing
	assert.Contains(t, view.Content, "─")

	// Submit hides landing
	model, _ := m.onSubmit("hello")
	m = model.(Model)
	assert.False(t, m.showLanding)

	// /clear re-shows landing
	model, _ = m.onSubmit("/clear")
	m = model.(Model)
	assert.True(t, m.showLanding)
	view = m.View()
	assert.Contains(t, view.Content, "█████")

	// Submit again
	model, _ = m.onSubmit("second message")
	m = model.(Model)
	assert.False(t, m.showLanding)

	// /new also re-shows landing
	model, _ = m.onSubmit("/new")
	m = model.(Model)
	assert.True(t, m.showLanding)
}

// TestIntegration_ScreenBufferLayout verifies the layout engine produces
// correct screen buffer regions for all terminal sizes.
func TestIntegration_ScreenBufferLayout(t *testing.T) {
	sizes := []struct {
		w, h int
	}{
		{200, 60},
		{120, 40},
		{80, 24},
		{40, 8},
	}

	for _, sz := range sizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			m := newModel(nil, nil, nil, nil)
			m.width = sz.w
			m.height = sz.h

			// Should not panic at any size
			assert.NotPanics(t, func() {
				canvas := uv.NewScreenBuffer(m.width, m.height)
				m.Draw(canvas, canvas.Bounds())
			})

			// With content, still shouldn't panic
			m.showLanding = false
			m.chat = m.chat.SetSize(sz.w, m.chatHeight(sz.h))
			m.AddUserMessage("test message")

			model, _ := m.Update(MessageStartMsg{})
			m = model.(Model)
			model, _ = m.Update(MessageEndMsg{Content: "response"})
			m = model.(Model)

			assert.NotPanics(t, func() {
				canvas := uv.NewScreenBuffer(m.width, m.height)
				m.Draw(canvas, canvas.Bounds())
			})
		})
	}
}

// TestIntegration_ThinkingLevelCycleWithModelChange verifies that thinking level
// correctly cycles and clamps when switching between reasoning and non-reasoning models.
func TestIntegration_ThinkingLevelCycleWithModelChange(t *testing.T) {
	sdkmodel.ResetModelRegistry()

	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true})
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "gpt-4.1", Provider: "openai"})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}

	// Default is medium
	assert.Equal(t, sdkmodel.ThinkingMedium, m.thinkingLevel)

	// Cycle to high
	model, _ := m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingHigh, m.thinkingLevel)
	assert.Equal(t, "248", m.editor.BorderColor)

	// Switch to non-reasoning model — forces thinking off
	model, _ = m.Update(ModelChangedMsg{Entry: ModelEntry{Provider: "openai", Model: "gpt-4.1"}})
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingOff, m.thinkingLevel)
	assert.Equal(t, "240", m.editor.BorderColor)

	// Switch back to reasoning model — thinking stays off until user changes it
	model, _ = m.Update(ModelChangedMsg{Entry: ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}})
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingOff, m.thinkingLevel)

	// Cycle: off -> minimal (next distinct level after off for Sonnet)
	model, _ = m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingMinimal, m.thinkingLevel)
}

// TestIntegration_InterruptDuringStreaming verifies interrupt behavior:
// escape interrupts streaming, second escape clears editor.
func TestIntegration_InterruptDuringStreaming(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicInterrupt)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	// Start streaming
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)
	model, _ = m.Update(MessageUpdateMsg{Content: "partial response"})
	m = model.(Model)

	// First escape — interrupts
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(Model)

	// Execute the batched commands so side effects (bus publishes) run
	if cmd != nil {
		executeBatchCmd(t, cmd)
	}

	// Verify message was interrupted — the assistant message should contain the partial content
	items := m.chat.Items()
	require.Len(t, items, 1)

	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "partial response")
	assert.False(t, am.IsStreaming(), "message should no longer be streaming")

	// Verify interrupt was published on the bus
	select {
	case evt := <-ch:
		assert.Equal(t, topicInterrupt, evt.Topic)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for interrupt event on bus")
	}
}

// TestIntegration_RegisterKeybindingViaSDKUI verifies that keybindings
// registered through the sdk.UI interface appear in the binding registry.
func TestIntegration_RegisterKeybindingViaSDKUI(t *testing.T) {
	commands := NewCommandRegistry(nil, "")
	bindings := NewBindingRegistry()
	ui := NewTUIImpl(commands, bindings)

	// Register a keybinding via sdk.UI
	ui.RegisterKeybinding(sdk.Keybinding{
		Name:        "custom.search",
		Keys:        []string{"ctrl+f"},
		Description: "Search in chat",
	})

	action, ok := bindings.Resolve("ctrl+f")
	assert.True(t, ok)
	assert.Equal(t, BindingAction("custom.search"), action)
}

// TestIntegration_DrawWithOverlayAndStreaming verifies the overlay stack
// correctly takes over rendering even during streaming.
func TestIntegration_DrawWithOverlayAndStreaming(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	// Start streaming
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)
	model, _ = m.Update(MessageUpdateMsg{Content: "streaming text"})
	m = model.(Model)

	// Push a confirm dialog
	m.dialogStack = m.dialogStack.Push(overlays.NewConfirmDialog(
		"popup-confirm-1",
		overlays.NewConfirmModel("Are you sure?").SetSize(80, 24).Show(),
	))

	// Draw should show overlay, not the streaming content underneath
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "Are you sure?")

	// Resolve dialog
	model, _ = m.Update(overlays.ConfirmResultMsg{Confirmed: true})
	m = model.(Model)
	assert.True(t, m.dialogStack.Empty())

	// Now streaming content should be visible
	canvas = uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered = canvas.Render()
	assert.Contains(t, rendered, "streaming text")
}

// --- helpers ---

func jsonEntry(role, content string, _ any) string {
	e := map[string]any{
		"type": "message",
		"data": map[string]any{"role": role, "content": content},
	}

	b, err := json.Marshal(e)
	if err != nil {
		panic(err)
	}

	return string(b)
}

func joinLines(lines []string) string {
	var b strings.Builder

	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}

		b.WriteString(l)
	}

	b.WriteByte('\n')

	return b.String()
}

// compile-time interface checks
var _ components.ChatItem = stubItem{}
