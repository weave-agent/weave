package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"weave/bus"
	"weave/ext/ui/tui/components"
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_HandlesMessageStart(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.Update(MessageStartMsg{})
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.True(t, am.IsStreaming())
	assert.Empty(t, am.Content())
}

func TestModel_HandlesMessageUpdate(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	// Start message first
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	// Stream deltas
	model, _ = m.Update(MessageUpdateMsg{Content: "hello "})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "world"})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "hello world", am.Content())
	assert.True(t, am.IsStreaming())
}

func TestModel_HandlesMessageEnd(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	// Start, update, end
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "streaming"})
	m = model.(Model)

	model, _ = m.Update(MessageEndMsg{Content: "final response"})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "final response", am.Content())
	assert.False(t, am.IsStreaming())
}

func TestModel_FullStreamingFlow(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// User message
	m.AddUserMessage("explain Go")

	// Assistant streaming
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "Go is "})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "a statically typed "})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "language."})
	m = model.(Model)

	model, _ = m.Update(MessageEndMsg{Content: "Go is a statically typed language."})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 2)

	// User message
	um, ok := items[0].(*messages.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "explain Go", um.Content())

	// Assistant message
	am, ok := items[1].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "Go is a statically typed language.", am.Content())
	assert.False(t, am.IsStreaming())
}

func TestModel_ViewShowsChatContent(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	m.AddUserMessage("hello")

	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "hi there"})
	m = model.(Model)

	model, _ = m.Update(MessageEndMsg{Content: "hi there"})
	m = model.(Model)

	view := m.View()
	assert.Contains(t, view, "hello")
	assert.Contains(t, view, "hi there")
}

func TestModel_UpdateWithoutStartIgnored(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	// Update without MessageStart should be ignored
	model, _ := m.Update(MessageUpdateMsg{Content: "orphan"})
	m = model.(Model)

	assert.Empty(t, m.chat.Items())
}

func TestModel_Shutdown(t *testing.T) {
	m := newModel(nil, nil, nil)
	_, cmd := m.Update(ShutdownMsg{})
	require.NotNil(t, cmd)
	// tea.Quit is a func, so we verify it produces a tea.QuitMsg
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "expected tea.QuitMsg from shutdown command")
}

func TestModel_WindowResize(t *testing.T) {
	m := newModel(nil, nil, nil)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = model.(Model)
	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)
	assert.Equal(t, 120, m.chat.Width())
	assert.Equal(t, m.chatHeight(40), m.chat.Height())
}

func TestModel_ResizeRedistributesHeight(t *testing.T) {
	m := newModel(nil, nil, nil)

	// Large terminal
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = model.(Model)
	chatH := m.chatHeight(50)
	assert.Greater(t, chatH, 30, "chat should get most of the height")
	assert.Equal(t, chatH, m.chat.Height())

	// Small terminal
	model, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m = model.(Model)
	chatH = m.chatHeight(10)
	assert.GreaterOrEqual(t, chatH, 1, "chat should always have at least 1 line")

	// Tiny terminal (below reserved space)
	model, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 5})
	m = model.(Model)
	chatH = m.chatHeight(5)
	assert.GreaterOrEqual(t, chatH, 1, "chat min is 1 even with tiny terminal")
}

func TestModel_ResizeWithSpinner(t *testing.T) {
	m := newModel(nil, nil, nil)

	// Show spinner
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)
	model, cmd := m.Update(components.SpinnerShowMsg{})
	m = model.(Model)
	// Consume the spinner tick cmd
	if cmd != nil {
		cmd()
	}

	assert.True(t, m.spinner.Visible())

	chatWithSpinner := m.chatHeight(40)

	// Hide spinner
	m.spinner = m.spinner.Hide()
	chatWithoutSpinner := m.chatHeight(40)

	assert.Equal(t, chatWithSpinner, chatWithoutSpinner-1, "spinner takes 1 line from chat")
}

func TestModel_MultipleTurns(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Turn 1
	m.AddUserMessage("question 1")
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)
	model, _ = m.Update(MessageUpdateMsg{Content: "answer 1"})
	m = model.(Model)
	model, _ = m.Update(MessageEndMsg{Content: "answer 1"})
	m = model.(Model)

	// Turn 2
	m.AddUserMessage("question 2")
	model, _ = m.Update(MessageStartMsg{})
	m = model.(Model)
	model, _ = m.Update(MessageUpdateMsg{Content: "answer 2"})
	m = model.(Model)
	model, _ = m.Update(MessageEndMsg{Content: "answer 2"})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 4) // 2 user + 2 assistant

	assert.Equal(t, "question 1", items[0].(*messages.UserMessage).Content())
	assert.Equal(t, "answer 1", items[1].(*messages.AssistantMessage).Content())
	assert.Equal(t, "question 2", items[2].(*messages.UserMessage).Content())
	assert.Equal(t, "answer 2", items[3].(*messages.AssistantMessage).Content())
}

func TestChatItemInterface(t *testing.T) {
	// Verify all chat item types satisfy ChatItem interface
	var (
		_ components.ChatItem = messages.NewAssistantMessage()
		_ components.ChatItem = messages.NewUserMessage("test")
		_ components.ChatItem = messages.NewToolPanel("tc1", "bash", "")
	)
}

func TestToolPanelItemIdentity(t *testing.T) {
	// Verify ToolPanel satisfies ChatItemIdentity
	var _ components.ChatItemIdentity = messages.NewToolPanel("tc1", "bash", "")
}

func TestModel_MessageEndCreatesToolPanels(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Start assistant message
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	// End with tool calls
	model, _ = m.Update(MessageEndMsg{
		Content: "I'll run bash",
		ToolCalls: []sdk.ToolCall{
			{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "ls"}},
			{ID: "tc2", Name: "read", Arguments: map[string]any{"path": "main.go"}},
		},
	})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 3) // assistant + 2 tool panels

	// Assistant message finalized
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "I'll run bash", am.Content())
	assert.False(t, am.IsStreaming())

	// Tool panel 1
	tp1, ok := items[1].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Equal(t, "tc1", tp1.ToolID())
	assert.Equal(t, messages.ToolPending, tp1.State())

	// Tool panel 2
	tp2, ok := items[2].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Equal(t, "tc2", tp2.ToolID())

	// Check toolPanels map
	assert.Contains(t, m.toolPanels, "tc1")
	assert.Contains(t, m.toolPanels, "tc2")
}

func TestModel_ToolResultUpdatesPanel(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Start assistant message
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	// End with tool call
	model, _ = m.Update(MessageEndMsg{
		Content:   "running bash",
		ToolCalls: []sdk.ToolCall{{ID: "tc1", Name: "bash", Arguments: nil}},
	})
	m = model.(Model)

	// Tool result arrives
	model, _ = m.Update(ToolResultMsg{
		ToolID: "tc1",
		Tool:   "bash",
		Result: sdk.ToolResult{Content: "file.txt", IsError: false},
	})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 2) // assistant + tool panel

	tp, ok := items[1].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Equal(t, messages.ToolSuccess, tp.State())
	assert.Contains(t, tp.View(80), "file.txt")
}

func TestModel_ToolResultError(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageEndMsg{
		Content:   "running bash",
		ToolCalls: []sdk.ToolCall{{ID: "tc1", Name: "bash", Arguments: nil}},
	})
	m = model.(Model)

	model, _ = m.Update(ToolResultMsg{
		ToolID: "tc1",
		Tool:   "bash",
		Result: sdk.ToolResult{Content: "permission denied", IsError: true},
	})
	m = model.(Model)

	tp, ok := m.chat.Items()[1].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Equal(t, messages.ToolError, tp.State())
}

func TestModel_ToolResultUnknownID(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Result for a tool panel that wasn't created via MessageEnd
	model, _ := m.Update(ToolResultMsg{
		ToolID: "tc-unknown",
		Tool:   "bash",
		Result: sdk.ToolResult{Content: "output", IsError: false},
	})
	m = model.(Model)

	// Should have created a new panel
	items := m.chat.Items()
	require.Len(t, items, 1)
	tp, ok := items[0].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Equal(t, "tc-unknown", tp.ToolID())
	assert.Equal(t, messages.ToolSuccess, tp.State())
}

func TestModel_ToolPanelInlineInChat(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 30
	m.chat = m.chat.SetSize(80, 30)

	// User asks a question
	m.AddUserMessage("list files")

	// Assistant response with tool use
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)
	model, _ = m.Update(MessageUpdateMsg{Content: "I'll list"})
	m = model.(Model)
	model, _ = m.Update(MessageEndMsg{
		Content:   "I'll list the files",
		ToolCalls: []sdk.ToolCall{{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "ls"}}},
	})
	m = model.(Model)

	// Tool result
	model, _ = m.Update(ToolResultMsg{
		ToolID: "tc1",
		Tool:   "bash",
		Result: sdk.ToolResult{Content: "file1.txt\nfile2.txt", IsError: false},
	})
	m = model.(Model)

	// Second assistant message with final answer
	model, _ = m.Update(MessageStartMsg{})
	m = model.(Model)
	model, _ = m.Update(MessageUpdateMsg{Content: "Here are"})
	m = model.(Model)
	model, _ = m.Update(MessageEndMsg{Content: "Here are the files"})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 4) // user + assistant + tool + assistant

	// Verify order: user -> assistant -> tool -> assistant
	_, ok := items[0].(*messages.UserMessage)
	assert.True(t, ok, "item 0 should be UserMessage")

	_, ok = items[1].(*messages.AssistantMessage)
	assert.True(t, ok, "item 1 should be AssistantMessage")

	_, ok = items[2].(*messages.ToolPanel)
	assert.True(t, ok, "item 2 should be ToolPanel")

	_, ok = items[3].(*messages.AssistantMessage)
	assert.True(t, ok, "item 3 should be AssistantMessage")

	// Verify the view contains tool output
	view := m.View()
	assert.Contains(t, view, "file1.txt")
}

func TestModel_MessageEndWithThinking(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Start assistant message
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	// End with thinking content
	model, _ = m.Update(MessageEndMsg{
		Content:  "The answer is 42",
		Thinking: "I need to consider the deep philosophical implications...",
	})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 2) // assistant + thinking block

	// Assistant message finalized
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "The answer is 42", am.Content())
	assert.False(t, am.IsStreaming())

	// Thinking block added
	tb, ok := items[1].(*messages.ThinkingBlock)
	require.True(t, ok)
	assert.Equal(t, "I need to consider the deep philosophical implications...", tb.Content())
	assert.False(t, tb.Expanded())
}

func TestModel_MessageEndWithoutThinking(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageEndMsg{
		Content:  "simple response",
		Thinking: "",
	})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1) // just assistant, no thinking block
}

func TestModel_ThinkingBlockInChatView(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageEndMsg{
		Content:  "result",
		Thinking: "deep thoughts",
	})
	m = model.(Model)

	view := m.View()
	assert.Contains(t, view, "[thinking]")
}

func TestModel_ThinkingBlockWithToolCalls(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 30
	m.chat = m.chat.SetSize(80, 30)

	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageEndMsg{
		Content:   "let me check",
		Thinking:  "I should use bash for this",
		ToolCalls: []sdk.ToolCall{{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "ls"}}},
	})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 3) // assistant + thinking + tool panel

	_, ok := items[0].(*messages.AssistantMessage)
	assert.True(t, ok, "item 0 should be AssistantMessage")

	_, ok = items[1].(*messages.ThinkingBlock)
	assert.True(t, ok, "item 1 should be ThinkingBlock")

	_, ok = items[2].(*messages.ToolPanel)
	assert.True(t, ok, "item 2 should be ToolPanel")
}

func TestModel_ResumeCommandDispatches(t *testing.T) {
	b := bus.New()
	defer b.Close()

	r := NewCommandRegistry(b, "")

	_, ok := r.Lookup("/resume")
	require.True(t, ok, "/resume command should be registered")

	handled, result := r.Dispatch("/resume")
	require.True(t, handled)
	assert.NotNil(t, result.Command)
	assert.False(t, result.Quit)
}

func TestModel_SessionListResultShowsOverlay(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project/alpha", CreatedAt: time.Now()},
		{ID: "bbb11122233344455566677788899900", CWD: "/project/beta", CreatedAt: time.Now().Add(-time.Hour)},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)

	assert.Equal(t, overlaySession, m.activeOverlay)
	assert.True(t, m.overlay.Visible())
	assert.Equal(t, sessions, m.pendingSessions)

	// Verify overlay items have CWD and timestamp
	view := m.overlay.View()
	assert.Contains(t, view, "Resume Session")
}

func TestModel_SessionListEmpty(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.Update(SessionListResultMsg{Sessions: nil})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "No sessions found")
}

func TestModel_SessionListError(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.Update(SessionListResultMsg{Err: errors.New("disk error")})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Error listing sessions")
}

func TestModel_SessionSelectorCancel(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.Equal(t, overlaySession, m.activeOverlay)

	// Cancel via ctrl+c
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)
	assert.False(t, m.overlay.Visible())
}

func TestModel_SessionSelectorEscape(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.Equal(t, overlaySession, m.activeOverlay)

	// Escape cancels the selector overlay
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)

	// The overlay's Update produces SelectorCancelledMsg via cmd
	assert.NotNil(t, cmd)

	// Process the cancel message
	msg := cmd()
	_, ok := msg.(overlays.SelectorCancelledMsg)
	assert.True(t, ok)

	model, _ = m.Update(overlays.SelectorCancelledMsg{})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)
	assert.Nil(t, m.pendingSessions)
}

func TestModel_SessionSelectorSelect(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicSessionResume)

	m := newModel(b, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	sessionID := "aaa11122233344455566677788899900"
	sessions := []SessionEntry{
		{ID: sessionID, CWD: "/project", CreatedAt: time.Now()},
	}

	// Create a session file to load
	header := sessionHeader{Type: "session", ID: sessionID, Timestamp: time.Now().UTC(), CWD: "/project"}
	headerJSON, _ := json.Marshal(header)

	entry := map[string]any{
		"type": "message",
		"data": map[string]any{"role": "user", "content": "previous question"},
	}

	eJSON, _ := json.Marshal(entry)
	content := string(headerJSON) + "\n" + string(eJSON) + "\n"
	err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(content), 0o644)
	require.NoError(t, err)

	// Show selector
	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.Equal(t, overlaySession, m.activeOverlay)

	// Select first item
	model, cmd := m.Update(overlays.SelectorSelectedMsg{Index: 0, Item: overlays.SelectorItem{
		Title: "/project", Subtitle: "2026-01-01 12:00",
	}})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)
	assert.Nil(t, m.pendingSessions)
	assert.False(t, m.prompted)

	// Verify chat was rebuilt with session history
	items := m.chat.Items()
	require.Len(t, items, 1)
	um, ok := items[0].(*messages.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "previous question", um.Content())

	// Execute the cmd to publish the bus event
	require.NotNil(t, cmd)
	cmd()

	// Verify session.resume event was published
	evt := <-ch
	assert.Equal(t, topicSessionResume, evt.Topic)
	assert.Equal(t, sessionID, evt.Payload)
}

func TestModel_OverlayInterceptsKeys(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.Equal(t, overlaySession, m.activeOverlay)

	// Regular key press should go to overlay, not editor
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = model.(Model)

	// Overlay should still be active (key was a filter char)
	assert.Equal(t, overlaySession, m.activeOverlay)
	assert.Equal(t, "a", m.overlay.Filter())
}

func TestModel_OverlayCtrlCDoesNotQuit(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.Equal(t, overlaySession, m.activeOverlay)

	// ctrl+c should cancel overlay, not quit
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)
	assert.Nil(t, cmd) // no quit command
}

func TestModel_RebuildChatFromSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	sessionID := "aaa11122233344455566677788899900"

	header := sessionHeader{Type: "session", ID: sessionID, Timestamp: time.Now().UTC(), CWD: "/test"}
	headerJSON, _ := json.Marshal(header)

	e1 := map[string]any{
		"type": "message",
		"data": map[string]any{"role": "user", "content": "question"},
	}
	e2 := map[string]any{
		"type": "message",
		"data": map[string]any{"role": "assistant", "content": "answer"},
	}
	e3 := map[string]any{
		"type": "message",
		"data": map[string]any{"role": "tool_result", "content": "output"},
	}

	e1JSON, _ := json.Marshal(e1)
	e2JSON, _ := json.Marshal(e2)
	e3JSON, _ := json.Marshal(e3)

	content := string(headerJSON) + "\n" + string(e1JSON) + "\n" + string(e2JSON) + "\n" + string(e3JSON) + "\n"
	err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(content), 0o644)
	require.NoError(t, err)

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)
	m.prompted = true

	m.rebuildChatFromSession(sessionID)

	items := m.chat.Items()
	require.Len(t, items, 3) // user + assistant + tool_result

	um, ok := items[0].(*messages.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "question", um.Content())

	am, ok := items[1].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "answer", am.Content())
	assert.False(t, am.IsStreaming())

	tp, ok := items[2].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Contains(t, tp.View(80), "output")

	// rebuildChatFromSession should not modify prompted — it stays whatever it was before.
	assert.True(t, m.prompted)
}

func TestModel_ViewShowsOverlayWhenActive(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	normalView := m.View()
	assert.NotContains(t, normalView, "Resume Session")

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)

	overlayView := m.View()
	assert.Contains(t, overlayView, "Resume Session")
}

func TestModel_ResumeSlashCommandIntegration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	b := bus.New()
	defer b.Close()

	m := newModel(b, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	// Dispatch /resume command
	model, cmd := m.onSubmit("/resume")
	m = model.(Model)

	require.NotNil(t, cmd)

	// Execute the command to get SessionListResultMsg
	msg := cmd()
	result, ok := msg.(SessionListResultMsg)
	require.True(t, ok)
	require.NoError(t, result.Err)
	assert.Empty(t, result.Sessions) // empty dir

	// Process the result (empty sessions)
	model, _ = m.Update(result)
	m = model.(Model)

	// Should show "No sessions found" message, not overlay
	assert.Equal(t, overlayNone, m.activeOverlay)
}

func TestModel_InterruptStreaming(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicInterrupt)

	m := newModel(b, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Start streaming
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)
	model, _ = m.Update(MessageUpdateMsg{Content: "partial"})
	m = model.(Model)

	// Trigger interrupt via keybinding
	model, cmd := m.dispatchBinding(ActionInterrupt)
	m = model.(Model)

	// Verify message was interrupted
	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.False(t, am.IsStreaming())
	assert.True(t, am.Interrupted())
	assert.Contains(t, am.Content(), "partial")
	assert.Contains(t, am.Content(), "[interrupted]")

	// Verify spinner is hidden
	assert.False(t, m.spinner.Visible())

	// Verify interrupt event was published
	require.NotNil(t, cmd)
	cmd()

	evt := <-ch
	assert.Equal(t, topicInterrupt, evt.Topic)
}

func TestModel_InterruptNoStreamingMessage(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Interrupt with no streaming message should be no-op
	model, cmd := m.dispatchBinding(ActionInterrupt)
	m = model.(Model)

	assert.Nil(t, cmd)
	assert.Empty(t, m.chat.Items())
}

func TestModel_AgentEndMsg_WithError(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Simulate a provider error
	model, _ := m.Update(AgentEndMsg{Payload: "stream error: timeout"})
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "stream error: timeout")
	assert.Contains(t, am.Content(), "[error]")
}

func TestModel_AgentEndMsg_WithNilPayload(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Normal end with no error
	model, _ := m.Update(AgentEndMsg{Payload: nil})
	m = model.(Model)

	assert.Empty(t, m.chat.Items())
	assert.False(t, m.spinner.Visible())
}

func TestModel_AgentEndMsg_WithEmptyString(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(AgentEndMsg{Payload: ""})
	m = model.(Model)

	assert.Empty(t, m.chat.Items())
}

func TestModel_GracefulShutdown(t *testing.T) {
	m := newModel(nil, nil, nil)

	// Ctrl+D triggers exit
	_, cmd := m.dispatchBinding(ActionExit)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok)
}

func TestModel_QuitCommand(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 20

	model, cmd := m.onSubmit("/quit")
	_ = model.(Model)

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok)
}
