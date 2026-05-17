package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sandbox "github.com/weave-agent/weave-sandbox"
	"github.com/weave-agent/weave-tui/components"
	"github.com/weave-agent/weave-tui/components/attachments"
	"github.com/weave-agent/weave-tui/components/messages"
	"github.com/weave-agent/weave-tui/components/overlays"
	"github.com/weave-agent/weave/bus"
	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weave-agent/weave-tui/palette"
)

// executeBatchCmd handles tea.Cmd results that may be tea.BatchMsg.
// Executes all nested commands so their side effects (bus publishes, etc.) run.
func executeBatchCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()

	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			c()
		}
	}
}

// subscribeToChan creates an internal channel and registers an On handler
// for the given topic that forwards events to the channel.
func subscribeToChan(b *bus.Bus, topic string) <-chan sdk.Event {
	ch := make(chan sdk.Event, 64)

	b.On(topic, func(ev sdk.Event) error {
		select {
		case ch <- ev:
		default:
		}

		return nil
	})

	return ch
}

// newModelNoLanding creates a model with landing screen disabled.
// Use in tests that check chat view content.
func newModelNoLanding() Model {
	m := newModel(nil, nil, nil, nil)
	m.showLanding = false

	return m
}

func TestModel_NewlineKeybindingsInsertEditorNewline(t *testing.T) {
	tests := []tea.KeyPressMsg{
		{Code: tea.KeyEnter, Mod: tea.ModShift},
		{Code: 'j', Mod: tea.ModCtrl},
	}

	for _, key := range tests {
		m := newModelNoLanding()
		m.editor = m.editor.SetValue("hello")

		model, _ := m.Update(key)

		updated := model.(Model)
		assert.Equal(t, "hello\n", updated.editor.Value(), "key %s", keyString(key))
	}
}

func TestModel_HandlesMessageStart(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
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
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	m.AddUserMessage("hello")

	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "hi there"})
	m = model.(Model)

	model, _ = m.Update(MessageEndMsg{Content: "hi there"})
	m = model.(Model)

	view := m.View()
	assert.Contains(t, view.Content, "hello")
	assert.Contains(t, view.Content, "hi there")
}

func TestModel_UpdateWithoutStartIgnored(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	// Update without MessageStart should be ignored
	model, _ := m.Update(MessageUpdateMsg{Content: "orphan"})
	m = model.(Model)

	assert.Empty(t, m.chat.Items())
}

func TestModel_Shutdown(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	_, cmd := m.Update(ShutdownMsg{})
	require.NotNil(t, cmd)
	// tea.Quit is a func, so we verify it produces a tea.QuitMsg
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "expected tea.QuitMsg from shutdown command")
}

func TestModel_WindowResize(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = model.(Model)
	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)
	assert.Equal(t, 120, m.chat.Width())
	assert.Equal(t, m.chatHeight(40), m.chat.Height())
}

func TestModel_ResizeRedistributesHeight(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

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
	m := newModel(nil, nil, nil, nil)

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
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
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
	m := newModelNoLanding()
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
	assert.Contains(t, view.Content, "file1.txt")
}

func TestModel_MessageEndWithThinking(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
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
	require.Len(t, items, 2) // thinking block + assistant

	// Thinking block inserted before answer
	tb, ok := items[0].(*messages.ThinkingBlock)
	require.True(t, ok)
	assert.Equal(t, "I need to consider the deep philosophical implications...", tb.Content())

	// Assistant message finalized after thinking
	am, ok := items[1].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "The answer is 42", am.Content())
	assert.False(t, am.IsStreaming())
}

func TestModel_MessageEndWithoutThinking(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
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
	m := newModelNoLanding()
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
	assert.Contains(t, view.Content, "Thinking")
}

func TestModel_ThinkingBlockWithToolCalls(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
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
	require.Len(t, items, 3) // thinking + assistant + tool panel

	_, ok := items[0].(*messages.ThinkingBlock)
	assert.True(t, ok, "item 0 should be ThinkingBlock")

	_, ok = items[1].(*messages.AssistantMessage)
	assert.True(t, ok, "item 1 should be AssistantMessage")

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
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project/alpha", CreatedAt: time.Now()},
		{ID: "bbb11122233344455566677788899900", CWD: "/project/beta", CreatedAt: time.Now().Add(-time.Hour)},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)

	assert.False(t, m.dialogStack.Empty())
	assert.Equal(t, sessions, m.pendingSessions)

	// Verify dialog renders session selector content
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "Resume Session")
}

func TestModel_SessionListEmpty(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.Update(SessionListResultMsg{Sessions: nil})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "No sessions found")
}

func TestModel_SessionListError(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.Update(SessionListResultMsg{Err: errors.New("disk error")})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Error listing sessions")
}

func TestModel_SessionSelectorCancel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// Cancel via ctrl+c
	model, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_SessionSelectorEscape(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// Escape cancels the selector overlay
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(Model)

	// The overlay's Update produces SelectorCancelledMsg via cmd
	assert.NotNil(t, cmd)

	// Process the cancel message
	msg := cmd()
	_, ok := msg.(overlays.SelectorCancelledMsg)
	assert.True(t, ok)

	model, _ = m.Update(overlays.SelectorCancelledMsg{})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
	assert.Nil(t, m.pendingSessions)
}

func TestModel_SessionSelectorSelect(t *testing.T) {
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

	// Register a mock session store so the resume payload includes message history.
	sdk.SetSessionStore(&testSessionStore{
		history: []sdk.Message{sdk.NewUserMessage("previous question")},
	})

	defer sdk.ResetSessionStore()

	// Show selector
	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// Select first item
	model, cmd := m.Update(overlays.SelectorSelectedMsg{Index: 0, Item: overlays.SelectorItem{
		Title: "/project", Subtitle: "2026-01-01 12:00",
	}})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
	assert.Nil(t, m.pendingSessions)
	assert.True(t, m.prompted)

	// Verify chat was rebuilt with session history
	items := m.chat.Items()
	require.Len(t, items, 1)
	um, ok := items[0].(*messages.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "previous question", um.Content())

	// Execute the cmd to publish the bus event
	require.NotNil(t, cmd)
	cmd()

	// Verify session.resume event was published with message history
	evt := <-ch
	assert.Equal(t, topicSessionResume, evt.Topic)
	payload, ok := evt.Payload.(sdk.SessionResumePayload)
	require.True(t, ok)
	assert.Equal(t, sessionID, payload.SessionID)
	require.Len(t, payload.Messages, 1)
	assert.Equal(t, "previous question", payload.Messages[0].Content)
}

func TestModel_OverlayInterceptsKeys(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// Regular key press should go to overlay, not editor
	model, _ = m.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	m = model.(Model)

	// Dialog should still be active (key was a filter char)
	assert.False(t, m.dialogStack.Empty())
}

func TestModel_OverlayCtrlCDoesNotQuit(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// ctrl+c should cancel overlay, not quit
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
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

	m := newModel(nil, nil, nil, nil)
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

func TestModel_SessionResumedMsg_RebuildsChat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	sessionID := "aaa11122233344455566677788899900"

	header := sessionHeader{Type: "session", ID: sessionID, Timestamp: time.Now().UTC(), CWD: "/test"}
	headerJSON, _ := json.Marshal(header)

	e1 := map[string]any{
		"type": "message",
		"data": map[string]any{"role": "user", "content": "previous question"},
	}
	e2 := map[string]any{
		"type": "message",
		"data": map[string]any{"role": "assistant", "content": "previous answer"},
	}

	e1JSON, _ := json.Marshal(e1)
	e2JSON, _ := json.Marshal(e2)

	content := string(headerJSON) + "\n" + string(e1JSON) + "\n" + string(e2JSON) + "\n"
	err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(content), 0o644)
	require.NoError(t, err)

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)
	m.showLanding = true
	m.prompted = true

	model, _ := m.Update(SessionResumedMsg{
		SessionID: sessionID,
		Messages: []sdk.Message{
			{Role: sdk.RoleUser, Content: "previous question"},
			{Role: sdk.RoleAssistant, Content: "previous answer"},
		},
	})
	m = model.(Model)

	// Chat should be rebuilt with session history
	items := m.chat.Items()
	require.Len(t, items, 2)

	um, ok := items[0].(*messages.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "previous question", um.Content())

	am, ok := items[1].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "previous answer", am.Content())

	// Landing should be hidden
	assert.False(t, m.showLanding)
	// Prompted should be true so next submit sends agent.followup
	assert.True(t, m.prompted)
}

func TestModel_SessionResumedMsg_EmptySessionID(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.showLanding = true
	m.prompted = true

	model, _ := m.Update(SessionResumedMsg{SessionID: ""})
	m = model.(Model)

	// Nothing should change
	assert.True(t, m.showLanding)
	assert.True(t, m.prompted)
	assert.Empty(t, m.chat.Items())
}

func TestModel_SessionResumedMsg_EmptyMessages(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)
	m.showLanding = true
	m.prompted = true

	model, _ := m.Update(SessionResumedMsg{SessionID: "empty-session", Messages: []sdk.Message{}})
	m = model.(Model)

	// Chat should be cleared (empty session)
	assert.Empty(t, m.chat.Items())

	// Landing should still be hidden (we attempted a resume)
	assert.False(t, m.showLanding)
	// Prompted should be true so next submit sends agent.followup
	assert.True(t, m.prompted)
}

func TestParseToolEntry_Valid(t *testing.T) {
	input := json.RawMessage(`{"id":"tc1","tool":"bash","result":{"content":"output","is_error":false}}`)
	id, name, content, isError := parseToolEntry(input)
	assert.Equal(t, "tc1", id)
	assert.Equal(t, "bash", name)
	assert.Equal(t, "output", content)
	assert.False(t, isError)
}

func TestParseToolEntry_InvalidJSON(t *testing.T) {
	id, name, content, isError := parseToolEntry(json.RawMessage(`not json`))
	assert.Empty(t, id)
	assert.Equal(t, "tool", name)
	assert.Empty(t, content)
	assert.False(t, isError)
}

func TestParseToolEntry_MissingFields(t *testing.T) {
	id, name, content, isError := parseToolEntry(json.RawMessage(`{}`))
	assert.Empty(t, id)
	assert.Equal(t, "tool", name)
	assert.Empty(t, content)
	assert.False(t, isError)
}

func TestParseToolEntry_Empty(t *testing.T) {
	id, name, content, isError := parseToolEntry(nil)
	assert.Empty(t, id)
	assert.Equal(t, "tool", name)
	assert.Empty(t, content)
	assert.False(t, isError)
}

func TestModel_RebuildChatFromSession_WithToolCalls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	sessionID := "aaa11122233344455566677788899900"

	header := sessionHeader{Type: "session", ID: sessionID, Timestamp: time.Now().UTC(), CWD: "/test"}
	headerJSON, _ := json.Marshal(header)

	e1 := map[string]any{
		"type": "message",
		"data": map[string]any{"role": "user", "content": "run command"},
	}
	e2 := map[string]any{
		"type": "message",
		"data": map[string]any{
			"role":       "assistant",
			"content":    "",
			"tool_calls": []map[string]any{{"id": "tc1", "name": "bash", "arguments": map[string]any{"command": "echo hi"}}},
		},
	}
	e3 := map[string]any{
		"type": "message",
		"data": map[string]any{
			"role":    "tool_result",
			"content": "hi",
			"tool":    map[string]any{"id": "tc1", "tool": "bash", "result": map[string]any{"content": "hi", "is_error": false}},
		},
	}

	e1JSON, _ := json.Marshal(e1)
	e2JSON, _ := json.Marshal(e2)
	e3JSON, _ := json.Marshal(e3)

	content := string(headerJSON) + "\n" + string(e1JSON) + "\n" + string(e2JSON) + "\n" + string(e3JSON) + "\n"
	err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(content), 0o644)
	require.NoError(t, err)

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	m.rebuildChatFromSession(sessionID)

	items := m.chat.Items()
	require.Len(t, items, 3) // user + assistant + tool_panel

	um, ok := items[0].(*messages.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "run command", um.Content())

	am, ok := items[1].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Empty(t, am.Content())

	tp, ok := items[2].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Contains(t, tp.View(80), "hi")
}

func TestModel_RebuildChatFromSession_WithThinking(t *testing.T) {
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
		"data": map[string]any{
			"role":     "assistant",
			"content":  "answer",
			"thinking": "Let me think about this...",
		},
	}

	e1JSON, _ := json.Marshal(e1)
	e2JSON, _ := json.Marshal(e2)

	content := string(headerJSON) + "\n" + string(e1JSON) + "\n" + string(e2JSON) + "\n"
	err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(content), 0o644)
	require.NoError(t, err)

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	m.rebuildChatFromSession(sessionID)

	items := m.chat.Items()
	require.Len(t, items, 3) // user + thinking + assistant

	um, ok := items[0].(*messages.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "question", um.Content())

	tb, ok := items[1].(*messages.ThinkingBlock)
	require.True(t, ok)
	assert.Equal(t, "Let me think about this...", tb.Content())

	am, ok := items[2].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "answer", am.Content())
}

func TestModel_ViewShowsOverlayWhenActive(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	sessions := []SessionEntry{
		{ID: "aaa11122233344455566677788899900", CWD: "/project", CreatedAt: time.Now()},
	}

	normalView := m.View()
	assert.NotContains(t, normalView.Content, "Resume Session")

	model, _ := m.Update(SessionListResultMsg{Sessions: sessions})
	m = model.(Model)

	dialogView := m.View()
	assert.Contains(t, dialogView.Content, "Resume Session")
}

func TestModel_ResumeSlashCommandIntegration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	b := bus.New()
	defer b.Close()

	m := newModel(b, nil, nil, nil)
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
	assert.True(t, m.dialogStack.Empty())
}

func TestModel_InterruptStreaming(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicInterrupt)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Start streaming
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)
	model, _ = m.Update(MessageUpdateMsg{Content: "partial"})
	m = model.(Model)

	// Trigger interrupt
	model, cmd := m.interruptStreaming()
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
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	// Interrupt with no streaming message should be no-op
	model, cmd := m.dispatchBinding(ActionInterrupt)
	m = model.(Model)

	assert.Nil(t, cmd)
	assert.Empty(t, m.chat.Items())
}

func TestModel_EscapeInterruptsAwaitAgent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicInterrupt)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(MessageEndMsg{
		ToolCalls: []sdk.ToolCall{{ID: "tool-1", Name: "await_agent"}},
	})
	m = model.(Model)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	require.NotNil(t, cmd)

	executeBatchCmd(t, cmd)

	select {
	case evt := <-ch:
		assert.Equal(t, topicInterrupt, evt.Topic)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interrupt event")
	}
}

func TestModel_EscapeInterruptsActiveSubagent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicInterrupt)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(MessageEndMsg{
		ToolCalls: []sdk.ToolCall{
			{ID: "tool-1", Name: "subagent_explore"},
			{ID: "tool-2", Name: "await_agent"},
		},
	})
	m = model.(Model)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	require.NotNil(t, cmd)

	executeBatchCmd(t, cmd)

	select {
	case evt := <-ch:
		assert.Equal(t, topicInterrupt, evt.Topic)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interrupt event")
	}
}

func TestModel_EscapeInterruptsAwaitAgentAfterSubagentCompletes(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicInterrupt)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(MessageEndMsg{
		ToolCalls: []sdk.ToolCall{
			{ID: "tool-1", Name: "subagent_explore"},
			{ID: "tool-2", Name: "await_agent"},
		},
	})
	m = model.(Model)

	model, _ = m.Update(ToolResultMsg{
		ToolID: "tool-1",
		Tool:   "subagent_explore",
		Result: sdk.ToolResult{Content: "done"},
	})
	m = model.(Model)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	require.NotNil(t, cmd)

	executeBatchCmd(t, cmd)

	select {
	case evt := <-ch:
		assert.Equal(t, topicInterrupt, evt.Topic)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interrupt event")
	}
}

func TestModel_AgentEndMsg_WithError(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(AgentEndMsg{Payload: ""})
	m = model.(Model)

	assert.Empty(t, m.chat.Items())
}

func TestModel_GracefulShutdown(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	// Ctrl+D triggers exit
	_, cmd := m.dispatchBinding(ActionExit)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok)
}

func TestModel_QuitCommand(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 20

	model, cmd := m.onSubmit("/quit")
	_ = model.(Model)

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok)
}

func TestModel_DefaultThinkingLevel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	assert.Equal(t, sdkmodel.ThinkingMedium, m.thinkingLevel)
	assert.Equal(t, "medium", m.footer.ThinkingLevel())
}

func TestModel_ThinkingLevelFromEnv(t *testing.T) {
	t.Setenv("WEAVE_THINKING_LEVEL", "high")

	m := newModel(nil, nil, nil, nil)
	assert.Equal(t, sdkmodel.ThinkingHigh, m.thinkingLevel)
	assert.Equal(t, "high", m.footer.ThinkingLevel())
}

func TestModel_ThinkingLevelInvalidEnv(t *testing.T) {
	t.Setenv("WEAVE_THINKING_LEVEL", "invalid")

	m := newModel(nil, nil, nil, nil)
	assert.Equal(t, sdkmodel.ThinkingMedium, m.thinkingLevel)
}

func TestModel_CycleThinkingLevel(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}

	assert.Equal(t, sdkmodel.ThinkingMedium, m.thinkingLevel)

	model, _ := m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingHigh, m.thinkingLevel)
	assert.Equal(t, "high", m.footer.ThinkingLevel())
	assert.Equal(t, "248", m.editor.BorderColor)

	// Second press skips xhigh (clamped for Sonnet) and goes to off
	model, _ = m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingOff, m.thinkingLevel)
}

func TestModel_CycleThinkingLevelWraps(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.thinkingLevel = sdkmodel.ThinkingXHigh
	m.editor = m.editor.SetBorderColor("250")

	// Register models so xhigh clamping works
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-opus-4-7", Provider: "anthropic", Reasoning: true, SupportsXHigh: true})

	defer sdkmodel.ResetModelRegistry()

	// Set current model to one that supports xhigh (opus)
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-opus-4-7"}

	model, _ := m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingOff, m.thinkingLevel)
	assert.Equal(t, "off", m.footer.ThinkingLevel())
	assert.Equal(t, "240", m.editor.BorderColor)
}

func TestModel_CycleThinkingLevelSkipsClampedForSonnet(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}

	// Sonnet doesn't support xhigh, so the cycle skips it (xhigh is clamped to high):
	// medium -> high -> off -> minimal -> low -> medium (wraps, skipping xhigh)
	expected := []sdkmodel.ThinkingLevel{
		sdkmodel.ThinkingMedium, // start
		sdkmodel.ThinkingHigh,
		sdkmodel.ThinkingOff,
		sdkmodel.ThinkingMinimal,
		sdkmodel.ThinkingLow,
		sdkmodel.ThinkingMedium, // wraps
	}

	for _, want := range expected {
		assert.Equal(t, want, m.thinkingLevel, "thinking level mismatch")
		model, _ := m.dispatchBinding(ActionThinkingCycle)
		m = model.(Model)
	}
}

func TestModel_CycleThinkingLevelAllLevels(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-opus-4-7", Provider: "anthropic", Reasoning: true, SupportsXHigh: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-opus-4-7"}

	expected := []sdkmodel.ThinkingLevel{
		sdkmodel.ThinkingMedium, // start
		sdkmodel.ThinkingHigh,
		sdkmodel.ThinkingXHigh,
		sdkmodel.ThinkingOff,
		sdkmodel.ThinkingMinimal,
		sdkmodel.ThinkingLow,
		sdkmodel.ThinkingMedium, // wraps
	}

	for _, want := range expected {
		assert.Equal(t, want, m.thinkingLevel, "thinking level mismatch")
		model, _ := m.dispatchBinding(ActionThinkingCycle)
		m = model.(Model)
	}
}

func TestModel_CycleThinkingPublishesEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicThinkingChange)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24

	_, cmd := m.dispatchBinding(ActionThinkingCycle)

	require.NotNil(t, cmd)

	// cmd is a tea.Batch — execute all wrapped commands
	executeBatchCmd(t, cmd)

	evt := <-ch
	assert.Equal(t, topicThinkingChange, evt.Topic)

	payload, ok := evt.Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "high", payload["level"])
}

func TestModel_EditorBorderMatchesThinkingLevel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	assert.Equal(t, "246", m.editor.BorderColor) // medium = "246"
}

func TestModel_ThinkingLevelUpdatesEditorBorder(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-opus-4-7", Provider: "anthropic", Reasoning: true, SupportsXHigh: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-opus-4-7"}

	// medium -> high
	model, _ := m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingHigh, m.thinkingLevel)
	assert.Equal(t, "248", m.editor.BorderColor) // high = "248"

	// high -> xhigh
	model, _ = m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingXHigh, m.thinkingLevel)
	assert.Equal(t, "250", m.editor.BorderColor) // xhigh = "250"
}

func TestModel_ThinkingCommand(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicThinkingChange)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	assert.Equal(t, sdkmodel.ThinkingMedium, m.thinkingLevel)

	// Dispatch /thinking high
	model, cmd := m.onSubmit("/thinking high")
	m = model.(Model)

	require.NotNil(t, cmd)

	// Execute the command to get ThinkingLevelSetMsg
	msg := cmd()
	setMsg, ok := msg.(ThinkingLevelSetMsg)
	require.True(t, ok)
	assert.Equal(t, sdkmodel.ThinkingHigh, setMsg.Level)

	// Process the message
	model, updateCmd := m.Update(setMsg)
	m = model.(Model)

	assert.Equal(t, sdkmodel.ThinkingHigh, m.thinkingLevel)
	assert.Equal(t, "high", m.footer.ThinkingLevel())
	assert.Equal(t, "248", m.editor.BorderColor)

	// Execute the batch cmd to trigger bus publish
	require.NotNil(t, updateCmd)
	executeBatchCmd(t, updateCmd)

	// Verify bus event was published
	evt := <-ch
	assert.Equal(t, topicThinkingChange, evt.Topic)
	payload, ok := evt.Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "high", payload["level"])
}

func TestModel_ThinkingCommandNoArgs(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.onSubmit("/thinking")
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Usage:")
	assert.Contains(t, am.Content(), "off")
	assert.Contains(t, am.Content(), "xhigh")
}

func TestModel_ThinkingCommandInvalid(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.onSubmit("/thinking bogus")
	m = model.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "invalid thinking level")
}

func TestModel_ThinkingCommandXHighClamped(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	// Set to Sonnet (no xhigh support)
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}

	model, cmd := m.onSubmit("/thinking xhigh")
	m = model.(Model)

	require.NotNil(t, cmd)
	msg := cmd()
	setMsg, ok := msg.(ThinkingLevelSetMsg)
	require.True(t, ok)

	model, _ = m.Update(setMsg)
	m = model.(Model)

	// xhigh should be clamped to high for Sonnet
	assert.Equal(t, sdkmodel.ThinkingHigh, m.thinkingLevel)
	assert.Equal(t, "248", m.editor.BorderColor)
}

func TestModel_ThinkingCommandAllLevels(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-opus-4-7", Provider: "anthropic", Reasoning: true, SupportsXHigh: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-opus-4-7"}

	for _, level := range sdkmodel.AllThinkingLevels {
		m.chat = components.NewChatModel().SetSize(80, 10)

		model, cmd := m.onSubmit("/thinking " + string(level))
		m = model.(Model)

		require.NotNil(t, cmd, "command should return a cmd for level %s", level)
		msg := cmd()
		setMsg, ok := msg.(ThinkingLevelSetMsg)
		require.True(t, ok)
		assert.Equal(t, level, setMsg.Level)

		model, _ = m.Update(setMsg)
		m = model.(Model)

		assert.Equal(t, level, m.thinkingLevel)
	}
}

func TestModel_StartupHintsShownInitially(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	assert.True(t, m.showHints)

	view := m.View()
	assert.Contains(t, view.Content, "ctrl+p model")
	assert.Contains(t, view.Content, "ctrl+l select")
	assert.Contains(t, view.Content, "shift+tab thinking")
	assert.Contains(t, view.Content, "ctrl+t toggle")
}

func TestModel_StartupHintsDismissOnKeypress(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	assert.True(t, m.showHints)

	// Any keypress dismisses hints
	model, _ := m.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	m = model.(Model)

	assert.False(t, m.showHints)

	view := m.View()
	assert.NotContains(t, view.Content, "ctrl+p cycle model")
}

func TestModel_StartupHintsHiddenAfterPrompt(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	b := bus.New()
	defer b.Close()

	m.bus = b

	assert.True(t, m.showHints)

	// Submit a prompt
	model, _ := m.onSubmit("hello")
	m = model.(Model)

	// Hints should still be in the model but hidden from view because prompted
	assert.True(t, m.showHints)
	assert.True(t, m.prompted)

	view := m.View()
	assert.NotContains(t, view.Content, "ctrl+p cycle model")
}

func TestModel_StartupHintsHiddenAfterChat(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	m.AddUserMessage("hello")

	assert.True(t, m.showHints)

	view := m.View()
	assert.NotContains(t, view.Content, "ctrl+p cycle model")
}

func TestModel_HeaderHints_HasBackgroundTint(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	// Ensure hints are shown and landing is not
	m.showHints = true
	m.showLanding = false
	m.prompted = false

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	// Hints banner should use BackgroundTint color (234)
	assert.Contains(t, rendered, "234")
	assert.Contains(t, rendered, "ctrl+p model")
}

// --- Draw tests (screen buffer rendering) ---

func TestModel_Draw_RendersAllSections(t *testing.T) {
	m := newModelNoLanding()
	m.width = 120
	m.height = 40
	m.chat = m.chat.SetSize(120, m.chatHeight(40))

	m.AddUserMessage("test message")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	// Chat content should be rendered
	assert.Contains(t, rendered, "test message")
	// Footer should contain CWD info
	assert.Contains(t, rendered, "weave")
	// Editor border should be present
	assert.Contains(t, rendered, "│")
	// Hints banner should NOT appear after first prompt
	assert.NotContains(t, rendered, "ctrl+p model")
}

func TestModel_Draw_ShowsChatContent(t *testing.T) {
	m := newModelNoLanding()
	m.width = 120
	m.height = 30
	m.chat = m.chat.SetSize(120, m.chatHeight(30))

	m.AddUserMessage("hello world")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	assert.Contains(t, rendered, "hello world")
}

func TestModel_Draw_HintsInHeader(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 120
	m.height = 30

	require.True(t, m.showHints)
	require.Empty(t, m.chat.Items())

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	assert.Contains(t, rendered, "ctrl+p model")
	assert.Contains(t, rendered, "ctrl+t toggle")
}

func TestModel_Draw_NoHintsAfterFirstPrompt(t *testing.T) {
	m := newModelNoLanding()
	m.width = 120
	m.height = 30

	require.True(t, m.showHints)
	m.AddUserMessage("first message")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	assert.NotContains(t, rendered, "ctrl+p model")
}

func TestModel_Draw_SpinnerInPills(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 120
	m.height = 30

	// Start streaming to show spinner
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)
	model, cmd := m.Update(components.SpinnerShowMsg{})
	m = model.(Model)

	if cmd != nil {
		cmd()
	}

	require.True(t, m.spinner.Visible())

	assert.NotPanics(t, func() {
		canvas := uv.NewScreenBuffer(m.width, m.height)
		m.Draw(canvas, canvas.Bounds())
	})
}

func TestModel_Draw_StatusInPills(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 120
	m.height = 30

	m.showStatus("test status message")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	assert.Contains(t, rendered, "test status message")
}

func TestModel_Draw_OverlayFillsScreen(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Activate model selector dialog with synthetic items
	items := []overlays.SelectorItem{
		{Title: "Model A"},
		{Title: "Model B"},
	}

	sel := overlays.NewSelectorModel("Select Model", items)
	sel = sel.SetSize(80, 24).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogModelSelect, sel))

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	assert.Contains(t, rendered, "Select Model")
}

func TestModel_Draw_SmallTerminal(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 40
	m.height = 8

	assert.NotPanics(t, func() {
		canvas := uv.NewScreenBuffer(m.width, m.height)
		m.Draw(canvas, canvas.Bounds())
	})
}

func TestModel_Draw_StreamingFlow(t *testing.T) {
	m := newModelNoLanding()
	m.width = 120
	m.height = 30
	m.chat = m.chat.SetSize(120, m.chatHeight(30))

	m.AddUserMessage("question")

	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "answer"})
	m = model.(Model)

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	assert.Contains(t, rendered, "question")
	assert.Contains(t, rendered, "answer")
}

func TestModel_Draw_LayoutSyncsChatSize(t *testing.T) {
	m := newModelNoLanding()
	m.width = 100
	m.height = 20
	m.chat = m.chat.SetSize(100, 20) // oversized on purpose

	m.AddUserMessage("test")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	// Verify rendering produced content despite chat being oversized
	rendered := canvas.Render()
	assert.Contains(t, rendered, "test")
}

func TestModel_TokenRatePassedToFooter(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "hello", TokenRate: 42.5})
	m = model.(Model)

	assert.InDelta(t, 42.5, m.footer.TokenRate(), 0.01)
}

func TestModel_TokenRateClearedOnMessageEnd(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)

	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageUpdateMsg{Content: "hello", TokenRate: 42.5})
	m = model.(Model)
	assert.InDelta(t, 42.5, m.footer.TokenRate(), 0.01)

	model, _ = m.Update(MessageEndMsg{Content: "hello"})
	m = model.(Model)
	assert.InDelta(t, 0.0, m.footer.TokenRate(), 0.001)
}

func TestModel_TurnEndSetsScrollIndicator(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 5) // small viewport

	// Add enough items to make chat scrollable
	for i := range 10 {
		m.chat = m.chat.AddItem(stubItem{text: fmt.Sprintf("line%d", i)})
	}

	// Scroll up so we're not at bottom
	m.chat = m.chat.ScrollUp(3)
	require.False(t, m.chat.AtBottom())

	// TurnEndMsg should set the indicator
	model, _ := m.Update(TurnEndMsg{})
	m = model.(Model)

	assert.True(t, m.chat.TurnEndPending())
}

func TestModel_ScrollToBottomClearsIndicator(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 5)

	for i := range 10 {
		m.chat = m.chat.AddItem(stubItem{text: fmt.Sprintf("line%d", i)})
	}

	m.chat = m.chat.ScrollUp(3).SetTurnEndPending(true)
	require.True(t, m.chat.TurnEndPending())

	model, _ := m.dispatchBinding(ActionScrollToBottom)
	m = model.(Model)

	assert.False(t, m.chat.TurnEndPending())
	assert.True(t, m.chat.AtBottom())
}

// stubItem is a simple ChatItem for tests in the tui package.
type stubItem struct {
	text string
}

func (s stubItem) View(width int) string { return s.text }

// --- Attachment integration tests ---

func TestModel_PasteDetection_ConvertsToAttachment(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Simulate a large paste (>10 newlines)
	lines := make([]string, 12)
	for i := range lines {
		lines[i] = "line of content"
	}

	longPaste := strings.Join(lines, "\n")

	model, cmd := m.Update(tea.PasteMsg{Content: longPaste})
	m = model.(Model)

	assert.Len(t, m.attach.Items(), 1)
	assert.Equal(t, 12, m.attach.Items()[0].Lines)
	// Status message should be set
	assert.Contains(t, m.statusMsg, "attachment")
	// cmd should be a timer (status timeout)
	assert.NotNil(t, cmd)
}

func TestModel_PasteDetection_ShortPastePassesThrough(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Short paste — goes to editor
	model, _ := m.Update(tea.PasteMsg{Content: "short text"})
	m = model.(Model)

	assert.Empty(t, m.attach.Items())
	assert.Contains(t, m.editor.Value(), "short text")
}

func TestModel_PasteDetection_CharThreshold(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Long text without newlines (>1000 chars)
	longText := strings.Repeat("x", 1001)

	model, _ := m.Update(tea.PasteMsg{Content: longText})
	m = model.(Model)

	assert.Len(t, m.attach.Items(), 1)
}

func TestModel_AttachmentDeleteMode_Toggle(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m = addTestAttachment(m, "a.go", "content a", 1)

	// ctrl+r toggles delete mode
	action, ok := m.bindings.Resolve("ctrl+r")
	require.True(t, ok)
	assert.Equal(t, ActionAttachDelete, action)

	model, _ := m.dispatchBinding(ActionAttachDelete)
	m = model.(Model)
	assert.True(t, m.attach.InDeleteMode())
	assert.Equal(t, 0, m.attach.DeleteIdx())
}

func TestModel_AttachmentDeleteMode_NavigateAndDelete(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m = addTestAttachment(m, "a.go", "aaa", 1)
	m = addTestAttachment(m, "b.go", "bbb", 2)

	// Enter delete mode
	m.attach = m.attach.ToggleDeleteMode()
	assert.True(t, m.attach.InDeleteMode())

	// Navigate down to second attachment
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = model.(Model)
	assert.Equal(t, 1, m.attach.DeleteIdx())

	// ctrl+r (dispatch) deletes highlighted
	model, _ = m.dispatchBinding(ActionAttachDelete)
	m = model.(Model)
	assert.Len(t, m.attach.Items(), 1)
	assert.Equal(t, "a.go", m.attach.Items()[0].Path)
}

func TestModel_AttachmentDeleteMode_EscapeExits(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m = addTestAttachment(m, "a.go", "aaa", 1)
	m.attach = m.attach.ToggleDeleteMode()

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(Model)
	assert.False(t, m.attach.InDeleteMode())
}

func TestModel_AttachmentDeleteMode_UpNav(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m = addTestAttachment(m, "a.go", "aaa", 1)
	m = addTestAttachment(m, "b.go", "bbb", 2)
	m.attach = m.attach.ToggleDeleteMode()

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = model.(Model)
	// Up should wrap to last
	assert.Equal(t, 1, m.attach.DeleteIdx())
}

func TestModel_SubmitWithAttachments(t *testing.T) {
	b := bus.New()
	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24
	m = addTestAttachment(m, "test.go", "package main", 1)
	m.prompted = true // followup mode

	text := "review this"
	model, cmd := m.onSubmit(text)
	m = model.(Model)

	// Attachments should be cleared after submit
	assert.Empty(t, m.attach.Items())

	// Chat should have the combined text
	items := m.chat.Items()
	require.Len(t, items, 1)
	um, ok := items[0].(*messages.UserMessage)
	require.True(t, ok)

	content := um.Content()
	assert.Contains(t, content, "review this")
	assert.Contains(t, content, "File: test.go")
	assert.Contains(t, content, "package main")

	// Followup should be published
	require.NotNil(t, cmd)
}

func TestModel_SubmitNoAttachments(t *testing.T) {
	b := bus.New()
	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.prompted = true

	text := "hello"
	model, cmd := m.onSubmit(text)
	m = model.(Model)

	assert.Empty(t, m.attach.Items())

	items := m.chat.Items()
	require.Len(t, items, 1)
	um, ok := items[0].(*messages.UserMessage)
	require.True(t, ok)
	assert.Equal(t, "hello", um.Content())
	require.NotNil(t, cmd)
}

func TestModel_NewSessionClearsAttachments(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m = addTestAttachment(m, "a.go", "aaa", 1)

	model, _ := m.dispatchBinding(ActionNewSession)
	m = model.(Model)
	assert.Empty(t, m.attach.Items())
}

// --- Completion integration tests ---

func TestModel_RefreshEditorCompletion_Empty(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_RefreshEditorCompletion_PlainText(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("hello world")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_RefreshEditorCompletion_SlashCommand(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("/he")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionSlash, m.editor.Completion().Kind())
	assert.Equal(t, 1, m.editor.Completion().FilteredCount()) // only /help matches
}

func TestModel_RefreshEditorCompletion_SlashCommandNoFilter(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("/")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionSlash, m.editor.Completion().Kind())
	assert.Positive(t, m.editor.Completion().FilteredCount())
}

func TestModel_RefreshEditorCompletion_SlashCommandWithSpaceNoAcceptsFiles(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("/help ")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_RefreshEditorCompletion_SlashCommandWithSpaceAcceptsFiles(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.commands.Register("/upload", "Upload files", true, func(_ string) CommandResult {
		return CommandResult{}
	})
	m.editor = m.editor.SetValue("/upload ")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_AtTrigger(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("text @")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_AtTriggerWithFilter(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("text @go")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_AtTriggerAtStart(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("@mod")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_NoWhitespaceBeforeAt(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("hello@world")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_RefreshEditorCompletion_HidesWhenContextGone(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("/he")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())

	m.editor = m.editor.SetValue("he")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_SlashCommandsUpdatedMsg_RefreshesCompletion(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("/")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())

	initialCount := m.editor.Completion().FilteredCount()

	// Register a new command
	m.commands.Register("/newcmd", "new command", false, func(_ string) CommandResult {
		return CommandResult{}
	})

	// Send update message
	updated, _ := m.Update(slashCommandsUpdatedMsg{})
	m = updated.(Model)

	assert.True(t, m.editor.CompletionActive())
	assert.Greater(t, m.editor.Completion().FilteredCount(), initialCount)
}

func TestModel_SlashCommandsUpdatedMsg_NoCompletionInactive(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("plain text")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())

	m.commands.Register("/newcmd2", "new command", false, func(_ string) CommandResult {
		return CommandResult{}
	})

	updated, _ := m.Update(slashCommandsUpdatedMsg{})
	m = updated.(Model)

	assert.False(t, m.editor.CompletionActive())
}

func TestModel_HandleCompletionKey_WhenActive(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("/")
	m = m.refreshEditorCompletion()
	require.True(t, m.editor.CompletionActive())

	// Tab should be intercepted
	handled, _, _ := m.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.True(t, handled)
}

func TestModel_HandleCompletionKey_WhenInactive(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("plain")
	require.False(t, m.editor.CompletionActive())

	// Tab should not be intercepted
	handled, _, _ := m.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.False(t, handled)
}

func TestModel_HandleCompletionKey_RegularKey(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("/he")
	m = m.refreshEditorCompletion()
	require.True(t, m.editor.CompletionActive())

	// Regular key should not be intercepted
	handled, _, _ := m.handleCompletionKey(tea.KeyPressMsg{Text: "a", Code: 'a'})
	assert.False(t, handled)
}

func TestModel_CompletionKeyFlow_TabCycles(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Type "/" to trigger completion
	model, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = model.(Model)
	require.True(t, m.editor.CompletionActive())

	// Tab should move cursor down
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(Model)
	assert.Equal(t, 1, m.editor.Completion().Cursor())
}

func TestModel_CompletionKeyFlow_EscapeDismisses(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	model, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = model.(Model)
	require.True(t, m.editor.CompletionActive())

	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = model.(Model)
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_CompletionKeyFlow_TypingUpdatesFilter(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Type "/h"
	model, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = model.(Model)
	model, _ = m.Update(tea.KeyPressMsg{Text: "h", Code: 'h'})
	m = model.(Model)

	require.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionSlash, m.editor.Completion().Kind())
	assert.Equal(t, 1, m.editor.Completion().FilteredCount()) // "/help" matches "h"
}

func TestModel_CompletionKeyFlow_AtTrigger(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Type "text @"
	for _, ch := range "text @" {
		model, _ := m.Update(tea.KeyPressMsg{Text: string(ch), Code: ch})
		m = model.(Model)
	}

	require.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_Draw_CompletionVisible(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.editor = m.editor.SetValue("/he")
	m = m.refreshEditorCompletion()
	require.True(t, m.editor.CompletionActive())

	canvas := uv.NewScreenBuffer(m.width, m.height)

	assert.NotPanics(t, func() {
		m.Draw(canvas, canvas.Bounds())
	})

	rendered := canvas.Render()
	// Completion popup should contain the matching command
	assert.Contains(t, rendered, "help")
}

func TestModel_Draw_CompletionVisibleAtTrigger(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.editor = m.editor.SetValue("text @")
	m = m.refreshEditorCompletion()
	require.True(t, m.editor.CompletionActive())

	canvas := uv.NewScreenBuffer(m.width, m.height)

	assert.NotPanics(t, func() {
		m.Draw(canvas, canvas.Bounds())
	})
}

func TestModel_Draw_CompletionNotVisible(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.editor = m.editor.SetValue("plain text")
	m = m.refreshEditorCompletion()
	require.False(t, m.editor.CompletionActive())

	canvas := uv.NewScreenBuffer(m.width, m.height)

	assert.NotPanics(t, func() {
		m.Draw(canvas, canvas.Bounds())
	})
}

func TestModel_Draw_CompletionPopupPosition(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.editor = m.editor.SetValue("/he")
	m = m.refreshEditorCompletion()
	require.True(t, m.editor.CompletionActive())

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	// Popup should render without panic and contain filtered content
	rendered := canvas.Render()
	assert.Contains(t, rendered, "help")
}

func TestModel_Draw_CompletionWithAttachments(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m = addTestAttachment(m, "test.go", "package main", 1)

	m.editor = m.editor.SetValue("/he")
	m = m.refreshEditorCompletion()
	require.True(t, m.editor.CompletionActive())

	canvas := uv.NewScreenBuffer(m.width, m.height)

	assert.NotPanics(t, func() {
		m.Draw(canvas, canvas.Bounds())
	})
}

func TestModel_RefreshEditorCompletion_MultilineAtTrigger(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("line one\nline two @")
	// Position cursor on second line, after @
	m.editor, _ = m.editor.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_MultilineSlashCommand(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("line one\n/help")
	// Position cursor on second line, at end
	m.editor, _ = m.editor.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = m.refreshEditorCompletion()
	// Slash completion should NOT activate on non-first lines since Dispatch
	// only handles commands when the full input starts with "/"
	assert.False(t, m.editor.CompletionActive())
}

// addTestAttachment is a helper to add a test attachment to the model.
func addTestAttachment(m Model, path, content string, lines int) Model {
	m.attach = m.attach.Add(attachments.Attachment{Path: path, Content: content, Lines: lines})
	return m
}

func TestModel_ThemeChangedMsg(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Default theme should be set
	require.NotNil(t, m.theme)
	assert.Equal(t, palette.DefaultTheme().Accent, m.theme.Accent)

	// Switch to a custom theme
	customTheme := &palette.Theme{
		Accent:     "123",
		Foreground: "255",
	}
	updated, _ := m.Update(themeChangedMsg{theme: customTheme})
	m = updated.(Model)

	assert.Equal(t, "123", m.theme.Accent)
	assert.Equal(t, "255", m.theme.Foreground)
}

func TestModel_ThemeChangedMsg_NilTheme(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	original := m.theme
	updated, _ := m.Update(themeChangedMsg{theme: nil})
	m = updated.(Model)

	// Nil theme should be ignored
	assert.Equal(t, original, m.theme)
}

func TestModel_ThemeUsedInRendering(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	// Set a custom theme with recognizable colors (valid ANSI 256 range: 0-255)
	m.theme = &palette.Theme{
		Muted:          "111",
		BackgroundTint: "222",
		Foreground:     "201",
	}

	m.showHints = true
	m.showLanding = false
	m.prompted = false
	m.statusNew = false // Ensure status uses Foreground, not Muted
	m.editor = m.editor.SetValue("test input")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	rendered := canvas.Render()
	assert.Contains(t, rendered, "ctrl+p model")
	// Verify custom theme colors are actually used in rendering
	assert.Contains(t, rendered, "111", "custom Muted color should appear in rendered output")
	assert.Contains(t, rendered, "222", "custom BackgroundTint color should appear in rendered output")
	assert.Contains(t, rendered, "201", "custom Foreground color should appear in rendered output")
}

func TestModel_ThemeUsedInBackdropDimming(t *testing.T) {
	m := newModelNoLanding()
	m.width = 40
	m.height = 10
	m.chat = m.chat.SetSize(40, m.chatHeight(10))

	m.theme = &palette.Theme{
		Muted: "99",
	}

	m.AddUserMessage("test")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.drawNormalUI(canvas, canvas.Bounds(), 0)
	m.applyBackdropDimming(canvas, canvas.Bounds())

	// Check that some cell has been dimmed to the custom muted color
	foundDimmed := false
	mutedColor := lipgloss.Color("99")

	for y := range 5 {
		for x := range 40 {
			cell := canvas.CellAt(x, y)
			if cell == nil || cell.IsZero() {
				continue
			}

			if cell.Style.Fg == mutedColor {
				foundDimmed = true
				break
			}
		}

		if foundDimmed {
			break
		}
	}

	assert.True(t, foundDimmed, "expected some cells to be dimmed to custom muted color")
}

func TestModel_CycleSandboxMode(t *testing.T) {
	defer func() { setSandboxer(nil) }()

	sb := &mockSandboxer{mode: sandbox.SandboxAuto}
	setSandboxer(sb)

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	model, _ := m.dispatchBinding(ActionSandboxCycle)
	m = model.(Model)

	assert.Equal(t, sandbox.SandboxOff, sb.mode, "auto -> off")
	assert.Contains(t, m.statusMsg, "Sandbox mode: off")

	model, _ = m.dispatchBinding(ActionSandboxCycle)
	m = model.(Model)

	assert.Equal(t, sandbox.SandboxReadonly, sb.mode, "off -> readonly")
	assert.Contains(t, m.statusMsg, "Sandbox mode: readonly")
}

func TestModel_CycleSandboxMode_NoSandboxer(t *testing.T) {
	defer func() { setSandboxer(nil) }()

	setSandboxer(nil)

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	model, _ := m.dispatchBinding(ActionSandboxCycle)
	m = model.(Model)

	assert.Contains(t, m.statusMsg, "not available")
}

func TestModel_CycleSandboxMode_UpdatesMode(t *testing.T) {
	defer func() { setSandboxer(nil) }()

	b := bus.New()
	defer b.Close()

	sb := &mockSandboxer{mode: sandbox.SandboxAuto}
	setSandboxer(sb)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24

	model, _ := m.dispatchBinding(ActionSandboxCycle)
	_ = model.(Model)

	assert.Equal(t, sandbox.SandboxOff, sb.mode)
}

func TestNextSandboxMode(t *testing.T) {
	assert.Equal(t, sandbox.SandboxReadonly, sandbox.NextSandboxMode(sandbox.SandboxOff))
	assert.Equal(t, sandbox.SandboxAsk, sandbox.NextSandboxMode(sandbox.SandboxReadonly))
	assert.Equal(t, sandbox.SandboxAuto, sandbox.NextSandboxMode(sandbox.SandboxAsk))
	assert.Equal(t, sandbox.SandboxOff, sandbox.NextSandboxMode(sandbox.SandboxAuto))
	assert.Equal(t, sandbox.SandboxOff, sandbox.NextSandboxMode("unknown"))
}

type mockSandboxer struct {
	mode string
}

func (m *mockSandboxer) WrapCommand(cmd, dir string) (string, error) { return cmd, nil }
func (m *mockSandboxer) AllowWrite(path string) bool                 { return true }
func (m *mockSandboxer) AllowRead(path string) bool                  { return true }
func (m *mockSandboxer) Mode() string                                { return m.mode }
func (m *mockSandboxer) SetMode(mode string)                         { m.mode = mode }

// --- Task 6: Status message entrance animation tests ---

func TestModel_StatusEntrance_SetsStatusNew(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	assert.False(t, m.statusNew)

	m.showStatus("test message")
	assert.True(t, m.statusNew)
	assert.Equal(t, "test message", m.statusMsg)
}

func TestModel_StatusEntrance_FirstDraw_Muted(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 10

	m.showStatus("test status")
	require.True(t, m.statusNew)

	// Draw while statusNew is true should render muted
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "test status")
}

func TestModel_StatusEntrance_AfterUpdate_FullBrightness(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 10

	m.showStatus("test status")
	require.True(t, m.statusNew)

	// Any Update clears statusNew
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m = model.(Model)
	assert.False(t, m.statusNew)

	// Draw after Update should render at full brightness
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "test status")
}

// --- Task 6: Dialog backdrop dimming tests ---

func TestModel_BackdropDimming_UIRenderedUnderDialog(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	// Add some chat content
	m.AddUserMessage("hello world")

	// Open a dialog
	items := []overlays.SelectorItem{
		{Title: "Model A"},
		{Title: "Model B"},
	}
	sel := overlays.NewSelectorModel("Select Model", items)
	sel = sel.SetSize(80, 24).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogModelSelect, sel))

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	// Dialog should be visible
	assert.Contains(t, rendered, "Select Model")
}

func TestModel_BackdropDimming_UIRenderedBeforeDimming(t *testing.T) {
	m := newModelNoLanding()
	m.width = 40
	m.height = 10
	m.chat = m.chat.SetSize(40, m.chatHeight(10))

	// Add content that will be rendered
	m.AddUserMessage("test")

	canvas := uv.NewScreenBuffer(m.width, m.height)

	// Render UI first
	m.drawNormalUI(canvas, canvas.Bounds(), 0)
	rendered := canvas.Render()
	assert.Contains(t, rendered, "test")

	// Apply dimming
	m.applyBackdropDimming(canvas, canvas.Bounds())

	// Check that some cell has been dimmed (foreground changed to muted color 245)
	foundDimmed := false
	mutedColor := lipgloss.Color(palette.DefaultTheme().Muted)

	for y := range 5 {
		for x := range 40 {
			cell := canvas.CellAt(x, y)
			if cell == nil || cell.IsZero() {
				continue
			}

			if cell.Style.Fg == mutedColor {
				foundDimmed = true

				break
			}
		}

		if foundDimmed {
			break
		}
	}

	assert.True(t, foundDimmed, "expected some cells to be dimmed to muted color")
}

func TestModel_BackdropDimming_NoDialog_NoDimming(t *testing.T) {
	m := newModelNoLanding()
	m.width = 40
	m.height = 10
	m.chat = m.chat.SetSize(40, m.chatHeight(10))

	m.AddUserMessage("no dialog")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	// Content should render normally without dimming
	assert.Contains(t, rendered, "no dialog")
}

// --- Task 7: Panel system and focus chain tests ---

func TestModel_PanelManager_Initialized(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	assert.NotNil(t, m.panelManager)
	assert.NotNil(t, m.panelTray)
	assert.Equal(t, FocusEditor, m.focus)
}

func TestModel_PanelFocusChain_TabEditorToTray(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Register and show a panel so tray is visible
	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	assert.Equal(t, FocusEditor, m.focus)

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(Model)

	assert.Equal(t, FocusTray, m.focus)
	assert.True(t, m.panelTray.IsFocused())
}

func TestModel_PanelFocusChain_TabTrayToPanel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(Model)

	assert.Equal(t, FocusPanel, m.focus)
	assert.False(t, m.panelTray.IsFocused())
}

func TestModel_PanelFocusChain_TabPanelToEditor(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.focus = FocusPanel

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(Model)

	assert.Equal(t, FocusEditor, m.focus)
}

func TestModel_PanelFocusChain_ShiftTabReverses(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	// Start at editor, Tab -> tray
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(Model)
	assert.Equal(t, FocusTray, m.focus)

	// Shift+Tab -> back to editor
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = model.(Model)
	assert.Equal(t, FocusEditor, m.focus)
}

func TestModel_PanelFocusChain_ShiftTabFromEditorToPanel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	// Shift+Tab from editor goes to panel
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = model.(Model)
	assert.Equal(t, FocusPanel, m.focus)
}

func TestModel_PanelFocusChain_EscReturnsToEditor(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.focus = FocusPanel

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(Model)

	assert.Equal(t, FocusEditor, m.focus)
}

func TestModel_PanelFocusChain_EscFromTrayReturnsToEditor(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(Model)

	assert.Equal(t, FocusEditor, m.focus)
}

func TestModel_PanelFocusChain_NoPanels_TabIgnored(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// No panels registered - Tab should fall through to editor
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(Model)

	assert.Equal(t, FocusEditor, m.focus)
}

func TestModel_PanelChanged_LastPanelRemovedClearsTrayFocus(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.syncPanelTray()
	m.focus = FocusTray
	m.panelTray = m.panelTray.SetFocused(true)

	m.panelManager.Remove("p1")
	model, _ := m.Update(panelChangedMsg{})
	m = model.(Model)

	assert.Equal(t, FocusEditor, m.focus)
	assert.False(t, m.panelTray.IsFocused())
	assert.Equal(t, 0, m.panelTray.Len())
}

func TestModel_PanelFocusChain_CompletionActive_TabIgnored(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	// Simulate active completion
	m.editor = m.editor.SetValue("/")
	m = m.refreshEditorCompletion()
	require.True(t, m.editor.CompletionActive())

	// Tab should go to completion, not tray
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(Model)
	assert.Equal(t, FocusEditor, m.focus)
}

func TestModel_TrayNavigation_RightArrow(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "A"}, &mockPanelDrawer{})
	m.panelManager.Register(PanelConfig{ID: "p2", Title: "B"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.panelManager.Show("p2")
	m.panelManager.Show("p1") // make p1 active so we can navigate right to p2
	m.syncPanelTray()
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = model.(Model)

	assert.Equal(t, "p2", m.panelTray.ActiveID())
	assert.Equal(t, "p2", m.panelManager.Active())
}

func TestModel_TrayNavigation_LeftArrow(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "A"}, &mockPanelDrawer{})
	m.panelManager.Register(PanelConfig{ID: "p2", Title: "B"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.panelManager.Show("p2")
	m.syncPanelTray()
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = model.(Model)

	assert.Equal(t, "p1", m.panelTray.ActiveID())
	assert.Equal(t, "p1", m.panelManager.Active())
}

func TestModel_TrayNavigation_EnterFocusesPanel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "A"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(Model)

	assert.Equal(t, FocusPanel, m.focus)
	assert.False(t, m.panelTray.IsFocused())
}

func TestModel_TrayNavigation_EnterShowsSelectedSubagentPanel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	drawer := &mockPanelDrawer{}
	m.panelManager.Register(PanelConfig{ID: "subagent-agent-1", Title: "agent"}, drawer)
	m.panelManager.Show("subagent-agent-1")
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(Model)

	assert.Equal(t, "subagent-agent-1", m.panelManager.Active())
	assert.Equal(t, FocusPanel, m.focus)
	assert.False(t, m.panelTray.IsFocused())
	assert.Equal(t, 0, drawer.updateCount)
}

func TestModel_TrayNavigation_ReturnShowsSelectedSubagentPanel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	drawer := &mockPanelDrawer{}
	m.panelManager.Register(PanelConfig{ID: "subagent-agent-1", Title: "agent", Placement: BelowEditor, Height: 6}, drawer)
	m.panelManager.Show("subagent-agent-1")
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyReturn})
	m = model.(Model)

	assert.Equal(t, "subagent-agent-1", m.panelManager.Active())
	assert.Equal(t, FocusPanel, m.focus)
	assert.False(t, m.panelTray.IsFocused())

	tray, above, below := m.panelRows()
	assert.Equal(t, 1, tray)
	assert.Equal(t, 0, above)
	assert.Equal(t, 6, below)
}

func TestModel_TrayNavigation_EnterOpensTrayOnlyPanelOverlay(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	drawer := &mockPanelDrawer{}
	m.panelManager.Register(PanelConfig{ID: "subagent-agent-1", Title: "agent", Placement: TrayOnly, Width: 40, Height: 10}, drawer)
	m.panelManager.Show("subagent-agent-1")
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(Model)

	assert.Equal(t, "subagent-agent-1", m.panelManager.Active())
	assert.Equal(t, "subagent-agent-1", m.expandedPanelID)
	assert.Equal(t, FocusPanel, m.focus)
	assert.False(t, m.panelTray.IsFocused())

	tray, above, below := m.panelRows()
	assert.Equal(t, 1, tray)
	assert.Equal(t, 0, above)
	assert.Equal(t, 0, below)

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	assert.Equal(t, 1, drawer.drawCount)
	assert.Equal(t, 36, drawer.lastArea.Dx())
	assert.Equal(t, 8, drawer.lastArea.Dy())
}

func TestModel_TrayNavigation_EscClosesTrayOnlyPanelOverlay(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "subagent-agent-1", Title: "agent", Placement: TrayOnly}, &mockPanelDrawer{})
	m.panelManager.Show("subagent-agent-1")
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(Model)
	require.Equal(t, "subagent-agent-1", m.expandedPanelID)

	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(Model)

	assert.Empty(t, m.expandedPanelID)
	assert.Equal(t, FocusTray, m.focus)
	assert.True(t, m.panelTray.IsFocused())
}

func TestModel_PanelFocusChain_TabDoesNotOpenTrayOnlyPanelOverlay(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "subagent-agent-1", Title: "agent", Placement: TrayOnly}, &mockPanelDrawer{})
	m.panelManager.Show("subagent-agent-1")

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(Model)
	require.Equal(t, FocusTray, m.focus)
	assert.Empty(t, m.expandedPanelID)

	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(Model)

	assert.Equal(t, FocusEditor, m.focus)
	assert.Empty(t, m.expandedPanelID)
	assert.False(t, m.panelTray.IsFocused())
}

func TestModel_PanelFocusChain_ShiftTabDoesNotOpenTrayOnlyPanelOverlay(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "subagent-agent-1", Title: "agent", Placement: TrayOnly}, &mockPanelDrawer{})
	m.panelManager.Show("subagent-agent-1")

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = model.(Model)

	assert.Equal(t, FocusTray, m.focus)
	assert.Empty(t, m.expandedPanelID)
	assert.True(t, m.panelTray.IsFocused())
}

func TestModel_TrayNavigation_EnterShowsVisiblePanelWhenNoPanelActive(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	drawer := &mockPanelDrawer{}
	m.panelManager.Register(PanelConfig{ID: "subagent-agent-1", Title: "agent"}, drawer)
	m.panelManager.panels["subagent-agent-1"].Visible = true
	m.focus = FocusTray

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(Model)

	assert.Equal(t, "subagent-agent-1", m.panelManager.Active())
	assert.Equal(t, FocusPanel, m.focus)
	assert.Equal(t, 0, drawer.updateCount)
}

func TestModel_PanelKeyForwarding(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	md := &mockPanelDrawer{}
	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, md)
	m.panelManager.Show("p1")
	m.focus = FocusPanel

	// Send a key that the panel drawer handles
	model, _ := m.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	_ = model.(Model)

	// The drawer should have received the key
	assert.Equal(t, 1, md.updateCount)
	assert.NotNil(t, md.lastMsg)
	keyMsg, ok := md.lastMsg.(tea.KeyPressMsg)
	require.True(t, ok)
	assert.Equal(t, 'x', keyMsg.Code)
	assert.Equal(t, FocusPanel, m.focus)
}

func TestModel_PanelPickerKeybinding(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.syncPanelTray()

	// Verify the binding exists
	action, ok := m.bindings.Resolve("f6")
	require.True(t, ok)
	assert.Equal(t, ActionPanelPicker, action)

	// Dispatch the action
	model, cmd := m.dispatchBinding(ActionPanelPicker)
	m = model.(Model)

	assert.Equal(t, FocusTray, m.focus)
	assert.True(t, m.panelTray.IsFocused())
	assert.Nil(t, cmd)
}

func TestModel_PanelPicker_NoPanels(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	model, cmd := m.dispatchBinding(ActionPanelPicker)
	m = model.(Model)

	assert.Equal(t, FocusEditor, m.focus)
	assert.Contains(t, m.statusMsg, "No panels visible")
	assert.NotNil(t, cmd)
}

func TestModel_Draw_WithPanel(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Files", Placement: AboveEditor, Height: 5}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	// Should not panic and should allocate regions
	rendered := canvas.Render()
	assert.NotEmpty(t, rendered)
}

func TestModel_Draw_WithPanelBelowEditor(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Log", Placement: BelowEditor, Height: 4}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	rendered := canvas.Render()
	assert.NotEmpty(t, rendered)
}

func TestModel_Draw_WithOverlayPanel(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	drawer := &mockPanelDrawer{}
	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Overlay", Placement: AsOverlay, Width: 20, Height: 5}, drawer)
	m.panelManager.Show("p1")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	assert.Equal(t, 1, drawer.drawCount)
	assert.Equal(t, 20, drawer.lastArea.Dx())
	assert.Equal(t, 5, drawer.lastArea.Dy())
}

func TestModel_Layout_WithPanel_ShrinksMain(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 120
	m.height = 40

	// Without panels
	h1 := m.chatHeight(40)

	// With panel
	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test", Placement: AboveEditor, Height: 8}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	h2 := m.chatHeight(40)

	assert.Less(t, h2, h1, "chat should shrink when panel is visible")
}

func TestModel_Layout_WithTray_ShrinksMain(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 120
	m.height = 40

	h1 := m.chatHeight(40)

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	h2 := m.chatHeight(40)

	assert.Less(t, h2, h1, "chat should shrink when tray is visible")
}

func TestModel_PanelManager_ShowHideRemove(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "A"}, &mockPanelDrawer{})
	m.panelManager.Register(PanelConfig{ID: "p2", Title: "B"}, &mockPanelDrawer{})

	m.panelManager.Show("p1")
	m.panelManager.Show("p2")

	assert.Equal(t, []string{"p1", "p2"}, m.panelManager.VisiblePanels())

	m.panelManager.Hide("p1")
	assert.Equal(t, []string{"p2"}, m.panelManager.VisiblePanels())

	m.panelManager.Remove("p2")
	assert.Empty(t, m.panelManager.VisiblePanels())
	assert.False(t, m.panelManager.IsRegistered("p2"))
}

func TestModel_Escape_NormalBehaviorWhenEditorFocused(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.focus = FocusEditor
	m.editor = m.editor.SetValue("some text")

	// First escape should start double-press timer
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(Model)

	assert.NotNil(t, cmd)
	assert.True(t, m.escapePressed)
	assert.Equal(t, "some text", m.editor.Value())

	// Second escape should clear the editor
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(Model)

	assert.False(t, m.escapePressed)
	assert.Empty(t, m.editor.Value())
}

func TestModel_SyncPanelTray(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Files"}, &mockPanelDrawer{})
	m.panelManager.Register(PanelConfig{ID: "p2", Title: "Git"}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.panelManager.Show("p2")

	m.syncPanelTray()

	assert.Equal(t, 2, m.panelTray.Len())
	assert.Equal(t, "p2", m.panelTray.ActiveID())
}

func TestModel_SyncPanelTray_NoTitleFallsBackToID(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	m.panelManager.Register(PanelConfig{ID: "my-panel"}, &mockPanelDrawer{})
	m.panelManager.Show("my-panel")

	m.syncPanelTray()

	assert.Equal(t, "my-panel", m.panelTray.ActiveID())
}

func TestModel_SyncPanelTray_NoVisualFallbackWhenNoActivePanel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	m.panelManager.Register(PanelConfig{ID: "my-panel"}, &mockPanelDrawer{})
	m.panelManager.panels["my-panel"].Visible = true

	m.syncPanelTray()

	assert.Equal(t, 1, m.panelTray.Len())
	assert.Empty(t, m.panelTray.ActiveID())
}

func TestModel_PanelRows_NoPanels(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	tray, above, below := m.panelRows()

	assert.Equal(t, 0, tray)
	assert.Equal(t, 0, above)
	assert.Equal(t, 0, below)
}

func TestModel_PanelRows_WithPanels(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test", Placement: AboveEditor, Height: 6}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	tray, above, below := m.panelRows()

	assert.Equal(t, 1, tray)
	assert.Equal(t, 6, above)
	assert.Equal(t, 0, below)
}

func TestModel_PanelRows_BelowEditor(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test", Placement: BelowEditor, Height: 4}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	tray, above, below := m.panelRows()

	assert.Equal(t, 1, tray)
	assert.Equal(t, 0, above)
	assert.Equal(t, 4, below)
}

func TestModel_PanelRows_TrayOnly(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test", Placement: TrayOnly, Height: 6}, &mockPanelDrawer{})
	m.panelManager.Show("p1")

	tray, above, below := m.panelRows()

	assert.Equal(t, 1, tray)
	assert.Equal(t, 0, above)
	assert.Equal(t, 0, below)
}

func TestModel_PanelChanged_RemovedPanelClearsExpandedOverlay(t *testing.T) {
	m := newModel(nil, nil, nil, nil)

	m.panelManager.Register(PanelConfig{ID: "p1", Title: "Test", Placement: TrayOnly}, &mockPanelDrawer{})
	m.panelManager.Show("p1")
	m.expandedPanelID = "p1"
	m.focus = FocusPanel
	m.panelManager.Remove("p1")

	model, _ := m.Update(panelChangedMsg{})
	m = model.(Model)

	assert.Empty(t, m.expandedPanelID)
	assert.Equal(t, FocusEditor, m.focus)
}

func TestModel_PanelChanged_TrayOnlyResizesChatViewport(t *testing.T) {
	m := newModelNoLanding()
	m.width = 40
	m.height = 24
	m.showHints = false
	m.chat = m.chat.SetSize(40, m.chatHeight(24))

	for i := range 20 {
		m.AddUserMessage(fmt.Sprintf("message %02d", i))
	}

	require.True(t, m.chat.AutoScroll())
	beforeHeight := m.chat.Height()
	beforeScroll := m.chat.ScrollOffset()

	m.panelManager.Register(PanelConfig{ID: "subagent-agent-1", Title: "agent", Placement: TrayOnly}, &mockPanelDrawer{})
	m.panelManager.Show("subagent-agent-1")
	model, _ := m.Update(panelChangedMsg{})
	m = model.(Model)

	assert.Equal(t, beforeHeight-1, m.chat.Height())
	assert.Greater(t, m.chat.ScrollOffset(), beforeScroll)
	assert.Equal(t, m.chatHeight(m.height), m.chat.Height())
}

func TestModel_SpinnerWithTrayResizesChatViewport(t *testing.T) {
	m := newModelNoLanding()
	m.width = 40
	m.height = 24
	m.showHints = false
	m.chat = m.chat.SetSize(40, m.chatHeight(24))

	for i := range 20 {
		m.AddUserMessage(fmt.Sprintf("message %02d", i))
	}

	m.panelManager.Register(PanelConfig{ID: "subagent-agent-1", Title: "agent", Placement: TrayOnly}, &mockPanelDrawer{})
	m.panelManager.Show("subagent-agent-1")
	model, _ := m.Update(panelChangedMsg{})
	m = model.(Model)
	trayHeight := m.chat.Height()
	trayScroll := m.chat.ScrollOffset()

	model, _ = m.Update(components.SpinnerShowMsg{})
	m = model.(Model)

	assert.Equal(t, trayHeight-1, m.chat.Height())
	assert.Greater(t, m.chat.ScrollOffset(), trayScroll)
	assert.Equal(t, m.chatHeight(m.height), m.chat.Height())

	headerRows, pillRows := m.countLayoutRows()
	trayRows, aboveRows, belowRows := m.panelRows()
	lt := m.layout.ComputeWithPanels(m.width, m.height, m.editor.Height()+m.attach.Height(), headerRows, pillRows, m.dockedRows(), trayRows, aboveRows, belowRows)
	assert.Equal(t, lt.Pills.Max.Y, lt.PanelTray.Min.Y)
}

// --- Task 9: TUIExtAPI model-level integration tests ---

func TestModel_SetEditorTextMsg_UpdatesEditor(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("old text")

	updated, _ := m.Update(setEditorTextMsg{text: "new text"})
	m = updated.(Model)

	assert.Equal(t, "new text", m.editor.Value())
}

func TestModel_PasteToEditorMsg_PastesText(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("hello ")

	updated, _ := m.Update(pasteToEditorMsg{text: "world"})
	m = updated.(Model)

	assert.Contains(t, m.editor.Value(), "world")
}

func TestModel_EditorTextRequestMsg_RespondsWithValue(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.editor = m.editor.SetValue("editor contents")

	resp := make(chan string, 1)
	_, _ = m.Update(editorTextRequestMsg{response: resp})

	select {
	case text := <-resp:
		assert.Equal(t, "editor contents", text)
	default:
		t.Fatal("expected response on channel")
	}
}

func TestModel_SetFooterMsg_SetsCustomFooter(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	comp := &mockTUIComponent{}

	updated, _ := m.Update(setFooterMsg{component: comp})
	m = updated.(Model)

	assert.Equal(t, comp, m.customFooter)
}

func TestModel_SetHeaderMsg_SetsCustomHeader(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	comp := &mockTUIComponent{}

	updated, _ := m.Update(setHeaderMsg{component: comp})
	m = updated.(Model)

	assert.Equal(t, comp, m.customHeader)
}

func TestModel_SetWorkingFramesMsg_UpdatesSpinner(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	frames := []string{"|", "/", "-", "\\"}

	updated, _ := m.Update(setWorkingFramesMsg{frames: frames, interval: 100 * time.Millisecond})
	m = updated.(Model)

	// Verify custom frames were applied by making spinner visible and checking View
	m.spinner = m.spinner.Show()
	view := m.spinner.View()
	assert.NotEmpty(t, view)
	// View should contain one of the custom frames (after ANSI style prefix)
	assert.True(t, strings.Contains(view, "|") || strings.Contains(view, "/") || strings.Contains(view, "-") || strings.Contains(view, "\\"))
}

func TestModel_CustomFooterDrawn(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24

	var drawn bool

	m.customFooter = &mockDrawComponent{drawFn: func(_ uv.Screen, _ uv.Rectangle) {
		drawn = true
	}}

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	assert.True(t, drawn, "custom footer should be drawn")
}

func TestModel_CustomHeaderDrawn(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24

	var drawn bool

	m.customHeader = &mockDrawComponent{drawFn: func(_ uv.Screen, _ uv.Rectangle) {
		drawn = true
	}}

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	assert.True(t, drawn, "custom header should be drawn")
}

func TestModel_InputHandlersInvokedOnKeyPress(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.ui = ui

	var received KeyEvent

	var receivedMu sync.Mutex

	ui.OnTerminalInput(func(ev KeyEvent) {
		receivedMu.Lock()
		received = ev
		receivedMu.Unlock()
	})

	_, _ = m.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})

	// Handlers are dispatched asynchronously; wait briefly for the goroutine.
	require.Eventually(t, func() bool {
		receivedMu.Lock()
		defer receivedMu.Unlock()

		return received.Code == 'x'
	}, 100*time.Millisecond, 10*time.Millisecond)
}

func TestModel_AutocompleteProviderQueried(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	provider := &mockAutocompleteProvider{}
	ui.AddAutocomplete(provider)

	m.editor = m.editor.SetValue("test")
	m = m.refreshEditorCompletion()

	// Should not panic and completion should work normally
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_RichRendererPreferredOverStandard(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 20
	m.chat = m.chat.SetSize(80, 20)
	m.ui = ui

	// Register both a rich renderer and standard renderer for the same tool
	ui.RegisterRichRenderer("test-tool", &mockRichRenderer{})
	ui.RegisterRenderer("test-tool", &mockRenderer{})

	// Start and end a message with a tool call
	model, _ := m.Update(MessageStartMsg{})
	m = model.(Model)

	model, _ = m.Update(MessageEndMsg{
		Content:   "using tool",
		ToolCalls: []sdk.ToolCall{{ID: "tc1", Name: "test-tool", Arguments: nil}},
	})
	m = model.(Model)

	// Tool panel should exist; rich renderer is preferred
	items := m.chat.Items()
	require.Len(t, items, 2)
	tp, ok := items[1].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Equal(t, "tc1", tp.ToolID())
}

func TestModel_RawInputHandler_PanicRecovery(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, ui)
	m.width = 80
	m.height = 24

	panicTriggered := make(chan struct{}, 1)

	ui.OnTerminalInput(func(_ KeyEvent) {
		panicTriggered <- struct{}{}

		panic("intentional test panic")
	})

	// Simulate a keypress — the handler runs in a goroutine and panics,
	// but the model update should complete without crashing.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a'})
	_ = updated.(Model)

	// Wait briefly for the goroutine to execute
	select {
	case <-panicTriggered:
		// expected
	case <-time.After(time.Second):
		require.FailNow(t, "handler goroutine did not execute")
	}

	// Small delay to ensure panic recovery has run
	time.Sleep(50 * time.Millisecond)
}

// mockDrawComponent is a TUIComponent that records draw calls.
type mockDrawComponent struct {
	drawFn func(scr uv.Screen, area uv.Rectangle)
}

func (m *mockDrawComponent) Draw(scr uv.Screen, area uv.Rectangle) {
	if m.drawFn != nil {
		m.drawFn(scr, area)
	}
}

// --- Task 4: Mouse selection tests ---

func TestModel_MouseClick_InChatArea_StartsSelection(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	// Compute the chat area to find a coordinate inside it
	area := m.chatArea()
	require.Positive(t, area.Dy(), "chat area should have height")

	// Click in the middle of the chat area
	clickX := area.Min.X + 2
	clickY := area.Min.Y + 1

	model, _ := m.Update(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})
	m = model.(Model)

	assert.True(t, m.chat.MouseDown(), "mouse should be down after click")
	// Single click creates a zero-width selection; HasSelection is false until drag
	sl, sc, el, ec := m.chat.SelectionBounds()
	assert.Equal(t, sl, el, "start and end line should be same after click")
	assert.Equal(t, sc, ec, "start and end column should be same after click")
}

func TestModel_MouseClick_OutsideChatArea_Ignored(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	// Click below the chat area (in the editor/footer region)
	model, _ := m.Update(tea.MouseClickMsg{X: 5, Y: m.height - 2, Button: tea.MouseLeft})
	m = model.(Model)

	assert.False(t, m.chat.MouseDown(), "mouse should not be down after click outside chat")
	assert.False(t, m.chat.HasSelection(), "selection should not be active")
}

func TestModel_MouseClick_RightButton_Ignored(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	clickX := area.Min.X + 2
	clickY := area.Min.Y + 1

	model, _ := m.Update(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseRight})
	m = model.(Model)

	assert.False(t, m.chat.MouseDown(), "right click should not start selection")
}

func TestModel_MouseClick_LandingScreen_Ignored(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.showLanding = true

	model, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft})
	m = model.(Model)

	assert.False(t, m.chat.MouseDown(), "click on landing screen should not start selection")
}

func TestModel_MouseDrag_ExtendsSelection(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	startX := area.Min.X + 2
	startY := area.Min.Y + 1
	endX := area.Min.X + 8
	endY := area.Min.Y + 1

	// Start selection with click
	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)

	// Drag to extend
	model, _ = m.Update(tea.MouseMotionMsg{X: endX, Y: endY, Button: tea.MouseLeft})
	m = model.(Model)

	sl, sc, el, ec := m.chat.SelectionBounds()
	assert.True(t, m.chat.HasSelection(), "selection should be active after drag")
	assert.Equal(t, sl, el, "drag on same line should have same start/end line")
	assert.Less(t, sc, ec, "end column should be greater than start column")
}

func TestModel_MouseDrag_NoButtonHeld_Ignored(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	startX := area.Min.X + 2
	startY := area.Min.Y + 1

	// Start selection
	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)

	// Motion without button held should not extend
	model, _ = m.Update(tea.MouseMotionMsg{X: startX + 5, Y: startY, Button: tea.MouseNone})
	m = model.(Model)

	sl, sc, el, ec := m.chat.SelectionBounds()
	assert.Equal(t, sc, ec, "selection should not extend without button held")
	assert.Equal(t, sl, el, "selection should stay on same line")
}

func TestModel_MouseDrag_OutsideChatArea_Ignored(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	startX := area.Min.X + 2
	startY := area.Min.Y + 1

	// Start selection
	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)

	// Drag outside chat area
	model, _ = m.Update(tea.MouseMotionMsg{X: 5, Y: m.height - 2, Button: tea.MouseLeft})
	m = model.(Model)

	// Selection should still have the original start point
	sl, sc, el, ec := m.chat.SelectionBounds()
	assert.Equal(t, sl, el, "selection should stay on original line")
	assert.Equal(t, sc, ec, "selection should not extend outside chat")
}

func TestModel_MouseRelease_EndsSelection(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	startX := area.Min.X + 2
	startY := area.Min.Y + 1
	endX := area.Min.X + 8
	endY := area.Min.Y + 1

	// Click and drag
	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseMotionMsg{X: endX, Y: endY, Button: tea.MouseLeft})
	m = model.(Model)

	assert.True(t, m.chat.MouseDown(), "mouse should be down before release")

	// Release
	model, _ = m.Update(tea.MouseReleaseMsg{X: endX, Y: endY, Button: tea.MouseNone})
	m = model.(Model)

	assert.False(t, m.chat.MouseDown(), "mouse should be up after release")
	assert.True(t, m.chat.HasSelection(), "selection should still exist after release")
}

func TestModel_KeyPress_ClearsSelection(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	startX := area.Min.X + 2
	startY := area.Min.Y + 1
	endX := area.Min.X + 8
	endY := area.Min.Y + 1

	// Create a selection
	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseMotionMsg{X: endX, Y: endY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseReleaseMsg{X: endX, Y: endY, Button: tea.MouseNone})
	m = model.(Model)

	assert.True(t, m.chat.HasSelection(), "selection should exist before key press")

	// Any key press should clear selection
	model, _ = m.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	m = model.(Model)

	assert.False(t, m.chat.HasSelection(), "selection should be cleared after key press")
}

func TestModel_MessageStart_ClearsSelection(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	startX := area.Min.X + 2
	startY := area.Min.Y + 1
	endX := area.Min.X + 8
	endY := area.Min.Y + 1

	// Create a selection
	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseMotionMsg{X: endX, Y: endY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseReleaseMsg{X: endX, Y: endY, Button: tea.MouseNone})
	m = model.(Model)

	assert.True(t, m.chat.HasSelection(), "selection should exist before new message")

	// New assistant message should clear selection
	model, _ = m.Update(MessageStartMsg{})
	m = model.(Model)

	assert.False(t, m.chat.HasSelection(), "selection should be cleared on new message")
}

func TestModel_MouseClick_DialogOpen_Ignored(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	// Open a dialog
	items := []overlays.SelectorItem{{Title: "Item A"}}
	sel := overlays.NewSelectorModel("Test", items)
	sel = sel.SetSize(80, 24).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog("test-dialog", sel))

	// Click anywhere should be handled by dialog, not chat
	model, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft})
	m = model.(Model)

	assert.False(t, m.chat.MouseDown(), "click should not start selection when dialog is open")
}

// --- Layout helper tests ---

func TestPointInArea(t *testing.T) {
	area := uv.Rect(10, 5, 20, 8) // Min(10,5) Max(30,13)

	assert.True(t, pointInArea(10, 5, area), "top-left corner should be inside")
	assert.True(t, pointInArea(15, 8, area), "center should be inside")
	assert.True(t, pointInArea(29, 12, area), "bottom-right corner (exclusive) should be inside")
	assert.False(t, pointInArea(9, 5, area), "left of area should be outside")
	assert.False(t, pointInArea(10, 4, area), "above area should be outside")
	assert.False(t, pointInArea(30, 5, area), "right of area should be outside")
	assert.False(t, pointInArea(10, 13, area), "below area should be outside")
}

func TestChatArea(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	area := m.chatArea()

	assert.Positive(t, area.Dy(), "chat area should have positive height")
	assert.Equal(t, 0, area.Min.X, "chat area should start at left edge")
	assert.Less(t, area.Max.Y, m.height, "chat area should end before bottom")
}

func TestChatContentPos(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("line one")
	m.AddUserMessage("line two")

	area := m.chatArea()

	// First visible line at top of chat area
	line, col := m.chatContentPos(area.Min.X, area.Min.Y, area)
	assert.Equal(t, m.chat.ScrollOffset(), line, "top of chat area maps to scroll offset")
	assert.Equal(t, 0, col, "left edge maps to column 0")

	// Second visible line
	line, col = m.chatContentPos(area.Min.X+3, area.Min.Y+1, area)
	assert.Equal(t, m.chat.ScrollOffset()+1, line, "one row down maps to next line")
	assert.Equal(t, 3, col, "column offset should match x offset")
}

func TestModel_MouseSelection_MultiLine(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	// Add multiple items to create multiple lines
	m.AddUserMessage("first message")
	m.AddUserMessage("second message")

	area := m.chatArea()

	// Click on first line, drag to second line
	startX := area.Min.X + 2
	startY := area.Min.Y + 1
	endX := area.Min.X + 5
	endY := area.Min.Y + 3

	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseMotionMsg{X: endX, Y: endY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseReleaseMsg{X: endX, Y: endY, Button: tea.MouseNone})
	m = model.(Model)

	sl, _, el, _ := m.chat.SelectionBounds()
	assert.True(t, m.chat.HasSelection(), "should have multi-line selection")
	assert.Less(t, sl, el, "selection should span multiple lines")
}

func TestModel_MouseSelection_WheelScroll_DoesNotAffectSelection(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, m.chatHeight(10))

	for i := range 20 {
		m.AddUserMessage(fmt.Sprintf("message %d", i))
	}

	// Scroll up first so wheel down has room to move
	m.chat = m.chat.ScrollUp(5)
	oldScroll := m.chat.ScrollOffset()
	require.Positive(t, oldScroll, "should have scrolled up")

	// Scroll down with wheel
	model, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = model.(Model)

	assert.Greater(t, m.chat.ScrollOffset(), oldScroll, "wheel should scroll down")
	assert.False(t, m.chat.MouseDown(), "wheel should not affect selection state")
}

// --- Task 5: Clipboard integration tests ---

func TestModel_DispatchBinding_CopySelection_WithSelection(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	startX := area.Min.X + 2
	startY := area.Min.Y + 0
	endX := area.Min.X + 8
	endY := area.Min.Y + 0

	// Create a selection via click and drag
	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseMotionMsg{X: endX, Y: endY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseReleaseMsg{X: endX, Y: endY, Button: tea.MouseNone})
	m = model.(Model)

	require.True(t, m.chat.HasSelection(), "should have a selection")

	// Trigger copy binding
	_, cmd := m.dispatchBinding(ActionCopySelection)
	require.NotNil(t, cmd, "copy binding should return a command when selection exists")

	// Execute the command and verify it's a batch containing clipboard operations
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok, "expected tea.BatchMsg, got %T", msg)
	require.Len(t, batch, 2, "batch should contain 2 commands")

	// The second command should return a notifyTypedMsg
	result := batch[1]()
	_, ok = result.(notifyTypedMsg)
	require.True(t, ok, "expected notifyTypedMsg, got %T", result)
}

func TestModel_DispatchBinding_CopySelection_NoSelection(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	// Trigger copy binding without selection
	model, cmd := m.dispatchBinding(ActionCopySelection)
	m = model.(Model)

	assert.NotNil(t, cmd, "copy binding should return a status timer command")
	assert.Equal(t, "Nothing selected", m.statusMsg)
}

func TestModel_MouseRelease_WithSelection_TriggersCopy(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	startX := area.Min.X + 2
	startY := area.Min.Y + 0
	endX := area.Min.X + 8
	endY := area.Min.Y + 0

	// Click and drag to create a selection
	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)
	model, _ = m.Update(tea.MouseMotionMsg{X: endX, Y: endY, Button: tea.MouseLeft})
	m = model.(Model)

	// Release should return a copy command
	_, cmd := m.Update(tea.MouseReleaseMsg{X: endX, Y: endY, Button: tea.MouseNone})
	require.NotNil(t, cmd, "mouse release with selection should return a copy command")

	// Execute the command and verify it's a batch containing clipboard operations
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok, "expected tea.BatchMsg, got %T", msg)
	require.Len(t, batch, 2, "batch should contain 2 commands")

	// The second command should return a notifyTypedMsg
	result := batch[1]()
	_, ok = result.(notifyTypedMsg)
	require.True(t, ok, "expected notifyTypedMsg, got %T", result)
}

func TestModel_MouseRelease_NoSelection_NoCopy(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.AddUserMessage("hello world")

	area := m.chatArea()
	startX := area.Min.X + 2
	startY := area.Min.Y + 0

	// Click only (no drag = no real selection)
	model, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	m = model.(Model)

	// Release without drag should not trigger copy
	_, cmd := m.Update(tea.MouseReleaseMsg{X: startX, Y: startY, Button: tea.MouseNone})
	assert.Nil(t, cmd, "mouse release without selection should not return a command")
}

func TestCopySelectionCmd(t *testing.T) {
	cmd := copySelectionCmd("test text")
	require.NotNil(t, cmd)

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok, "expected tea.BatchMsg, got %T", msg)
	require.Len(t, batch, 2, "batch should contain 2 commands")

	// The second command should return a notifyTypedMsg (success or error)
	result := batch[1]()
	_, ok = result.(notifyTypedMsg)
	require.True(t, ok, "expected notifyTypedMsg, got %T", result)
}

func TestKeybindings_CopySelection_IsRegistered(t *testing.T) {
	bindings := NewBindingRegistry()

	action, ok := bindings.Resolve("ctrl+shift+c")
	assert.True(t, ok, "ctrl+shift+c should be registered")
	assert.Equal(t, ActionCopySelection, action)
}

// testSessionStore is a minimal sdk.SessionStore implementation for tests.
type testSessionStore struct {
	history []sdk.Message
}

func (s *testSessionStore) ListSessions() ([]sdk.SessionInfo, error) { return nil, nil }
func (s *testSessionStore) LoadHistory(_ string) ([]sdk.Message, error) {
	return s.history, nil
}

func TestModel_LoginListResult_Empty(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	model, _ := m.onLoginListResult(LoginListResultMsg{Providers: nil})
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "No providers available")
}

func TestModel_LoginListResult_ShowsDialog(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24

	providers := []LoginProviderEntry{
		{Name: "Test Provider", ID: "test", IsOAuth: false, HasAuth: false},
	}

	model, _ := m.onLoginListResult(LoginListResultMsg{Providers: providers})
	m2 := model.(Model)

	assert.False(t, m2.dialogStack.Empty())
	assert.Len(t, m2.pendingLoginProviders, 1)
}

func TestModel_LogoutListResult_Empty(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	model, _ := m.onLogoutListResult(LogoutListResultMsg{Providers: nil})
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "No providers currently authenticated")
}

func TestModel_LogoutListResult_ShowsDialog(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24

	providers := []LogoutProviderEntry{
		{Name: "Test Provider", ID: "test"},
	}

	model, _ := m.onLogoutListResult(LogoutListResultMsg{Providers: providers})
	m2 := model.(Model)

	assert.False(t, m2.dialogStack.Empty())
	assert.Len(t, m2.pendingLogoutProviders, 1)
}

func TestModel_OnLoginDialogDone_OAuthProvider(t *testing.T) {
	// Register a test OAuth provider.
	//nolint:gosec // Test URLs, not real credentials
	sdk.RegisterOAuthProvider(sdk.OAuthProvider{
		ID:       "test-oauth",
		Name:     "Test OAuth",
		AuthURL:  "https://example.com/authorize",
		TokenURL: "https://example.com/token",
		Scopes:   []string{"read"},
		FlowType: sdk.AuthorizationCode,
	})

	defer sdk.ResetOAuthRegistry()

	m := newModelNoLanding()
	m.width = 80
	m.height = 24

	m.pendingLoginProviders = []LoginProviderEntry{
		{Name: "Test OAuth", ID: "test-oauth", IsOAuth: true},
	}

	result := overlays.DialogResult{Index: 0}
	model, cmd := m.onLoginDialogDone(result, nil)
	m2 := model.(Model)

	assert.Nil(t, m2.pendingLoginProviders)
	// Login dialog should be pushed onto the stack.
	assert.False(t, m2.dialogStack.Empty())
	assert.Equal(t, dialogLoginOAuth, m2.dialogStack.Peek().ID())
	// A command should be returned to run the OAuth flow.
	assert.NotNil(t, cmd)
}

func TestModel_OnLoginDialogDone_OAuthProvider_NotFound(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	m.pendingLoginProviders = []LoginProviderEntry{
		{Name: "Unknown OAuth", ID: "unknown-oauth", IsOAuth: true},
	}

	result := overlays.DialogResult{Index: 0}
	model, _ := m.onLoginDialogDone(result, nil)
	m2 := model.(Model)

	assert.Nil(t, m2.pendingLoginProviders)
	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "not configured")
}

//nolint:gosec // "TEST-CODE" is a mock user code for testing, not a credential
func TestModel_OnLoginDialogDone_DeviceCodeProvider(t *testing.T) {
	// Start a mock device code endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"device_code":      "dc-test",
			"user_code":        "TEST-CODE",
			"verification_uri": "https://example.com/verify",
			"expires_in":       900,
			"interval":         5,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	sdk.RegisterOAuthProvider(sdk.OAuthProvider{
		ID:            "test-device-code",
		Name:          "Test Device Code",
		DeviceCodeURL: server.URL,
		TokenURL:      "https://example.com/token",
		FlowType:      sdk.DeviceCode,
	})

	defer sdk.ResetOAuthRegistry()

	m := newModelNoLanding()
	m.width = 80
	m.height = 24

	m.pendingLoginProviders = []LoginProviderEntry{
		{Name: "Test Device Code", ID: "test-device-code", IsOAuth: true},
	}

	result := overlays.DialogResult{Index: 0}
	model, cmd := m.onLoginDialogDone(result, nil)
	m2 := model.(Model)

	assert.Nil(t, m2.pendingLoginProviders)
	assert.False(t, m2.dialogStack.Empty())
	assert.Equal(t, dialogLoginOAuth, m2.dialogStack.Peek().ID())
	assert.NotNil(t, cmd)

	// Verify the login dialog shows the user code.
	ld, ok := m2.dialogStack.Peek().(*overlays.LoginDialog)
	require.True(t, ok)
	assert.Contains(t, ld.Model().View(), "TEST-CODE")
	assert.Contains(t, ld.Model().View(), "https://example.com/verify")
}

//nolint:gosec // test OAuth provider registration with mock endpoints
func TestModel_OnLoginDialogDone_DeviceCodeProvider_RequestFails(t *testing.T) {
	// Start a mock endpoint that returns an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)

		resp := map[string]any{
			"error":             "invalid_client",
			"error_description": "Bad client",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	sdk.RegisterOAuthProvider(sdk.OAuthProvider{
		ID:            "test-device-code-fail",
		Name:          "Test Device Code Fail",
		DeviceCodeURL: server.URL,
		TokenURL:      "https://example.com/token",
		FlowType:      sdk.DeviceCode,
	})

	defer sdk.ResetOAuthRegistry()

	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	m.pendingLoginProviders = []LoginProviderEntry{
		{Name: "Test Device Code Fail", ID: "test-device-code-fail", IsOAuth: true},
	}

	result := overlays.DialogResult{Index: 0}
	model, _ := m.onLoginDialogDone(result, nil)
	m2 := model.(Model)

	assert.Nil(t, m2.pendingLoginProviders)
	// Dialog should not be pushed on error.
	assert.True(t, m2.dialogStack.Empty())
	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Failed to start device code flow")
}

func TestModel_OnLoginDialogDone_RegularProvider(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24

	m.pendingLoginProviders = []LoginProviderEntry{
		{Name: "Anthropic", ID: "anthropic", IsOAuth: false},
	}

	result := overlays.DialogResult{Index: 0}
	model, _ := m.onLoginDialogDone(result, nil)
	m2 := model.(Model)

	assert.Nil(t, m2.pendingLoginProviders)
	assert.Equal(t, "anthropic", m2.providerTarget)
	assert.False(t, m2.dialogStack.Empty())
}

func TestModel_OnLoginDialogDone_Cancel(t *testing.T) {
	m := newModelNoLanding()
	m.pendingLoginProviders = []LoginProviderEntry{
		{Name: "Test", ID: "test"},
	}

	result := overlays.DialogResult{Err: assert.AnError}
	model, _ := m.onLoginDialogDone(result, nil)
	m2 := model.(Model)

	assert.Nil(t, m2.pendingLoginProviders)
	assert.Empty(t, m2.providerTarget)
}

func TestModel_OnLogoutDialogDone_ClearsAuth(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	m.pendingLogoutProviders = []LogoutProviderEntry{
		{Name: "Test", ID: "test"},
	}

	result := overlays.DialogResult{Index: 0}
	model, _ := m.onLogoutDialogDone(result, nil)
	m2 := model.(Model)

	assert.Nil(t, m2.pendingLogoutProviders)
	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Logged out")
}

func TestModel_OnLogoutDialogDone_Cancel(t *testing.T) {
	m := newModelNoLanding()
	m.pendingLogoutProviders = []LogoutProviderEntry{
		{Name: "Test", ID: "test"},
	}

	result := overlays.DialogResult{Err: assert.AnError}
	model, _ := m.onLogoutDialogDone(result, nil)
	m2 := model.(Model)

	assert.Nil(t, m2.pendingLogoutProviders)
}

func TestModel_HandleDialogDone_LoginLogout(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	// Test login dialog
	m.pendingLoginProviders = []LoginProviderEntry{
		{Name: "OpenAI", ID: "openai", IsOAuth: true},
	}
	sel := overlays.NewSelectorModel("Login", []overlays.SelectorItem{{Title: "OpenAI"}})
	sel = sel.Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogLoginSelect, sel))

	// Simulate dialog completion via Update
	model, _ := m.Update(overlays.SelectorSelectedMsg{Index: 0})
	m2 := model.(Model)
	assert.Nil(t, m2.pendingLoginProviders)
}

func TestModel_HandleDialogForceCancel_LoginLogout(t *testing.T) {
	m := newModelNoLanding()
	m.pendingLoginProviders = []LoginProviderEntry{{Name: "Test", ID: "test"}}
	m.pendingLogoutProviders = []LogoutProviderEntry{{Name: "Test", ID: "test"}}

	sel := overlays.NewSelectorModel("Login", []overlays.SelectorItem{{Title: "Test"}})
	sel = sel.Show()
	loginDlg := overlays.NewSelectorDialog(dialogLoginSelect, sel)

	model, _ := m.handleDialogForceCancel(loginDlg)
	m2 := model.(Model)
	assert.Nil(t, m2.pendingLoginProviders)
	assert.NotNil(t, m2.pendingLogoutProviders) // logout should still be pending

	sel2 := overlays.NewSelectorModel("Logout", []overlays.SelectorItem{{Title: "Test"}})
	sel2 = sel2.Show()
	logoutDlg := overlays.NewSelectorDialog(dialogLogoutSelect, sel2)

	model, _ = m2.handleDialogForceCancel(logoutDlg)
	m3 := model.(Model)
	assert.Nil(t, m3.pendingLogoutProviders)
}

func TestModel_OnLoginDialogDone_RegularProvider_Masked(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24

	m.pendingLoginProviders = []LoginProviderEntry{
		{Name: "Anthropic", ID: "anthropic", IsOAuth: false},
	}

	result := overlays.DialogResult{Index: 0}
	model, _ := m.onLoginDialogDone(result, nil)
	m2 := model.(Model)

	assert.Nil(t, m2.pendingLoginProviders)
	assert.Equal(t, "anthropic", m2.providerTarget)
	assert.False(t, m2.dialogStack.Empty())

	dlg := m2.dialogStack.Peek()
	require.NotNil(t, dlg)
	inputDlg, ok := dlg.(*overlays.InputDialog)
	require.True(t, ok)
	assert.Equal(t, '*', inputDlg.Model().Mask())
}

func TestModel_OnProviderDialogDone_Masked(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24

	m.pendingProviders = []ProviderEntry{
		{Name: "Anthropic", HasKey: false},
	}

	result := overlays.DialogResult{Index: 0}
	model, _ := m.onProviderDialogDone(result, nil)
	m2 := model.(Model)

	assert.Equal(t, "Anthropic", m2.providerTarget)
	assert.False(t, m2.dialogStack.Empty())

	dlg := m2.dialogStack.Peek()
	require.NotNil(t, dlg)
	inputDlg, ok := dlg.(*overlays.InputDialog)
	require.True(t, ok)
	assert.Equal(t, '*', inputDlg.Model().Mask())
}

// trackingPS records SaveProviderKey calls for testing.
type trackingPS struct {
	*mockConfig
	savedProvider string
	savedKey      string
	saveErr       error
}

func (t *trackingPS) SaveProviderKey(provider, key string) error {
	t.savedProvider = provider
	t.savedKey = key

	return t.saveErr
}

func TestModel_OnKeyInputDialogDone_SavesKeyAndUpdatesAuth(t *testing.T) {
	sdkmodel.ResetAuthRegistry()
	defer sdkmodel.ResetAuthRegistry()

	ps := &trackingPS{mockConfig: &mockConfig{}}
	m := newModel(nil, ps, ps, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.providerTarget = "anthropic"

	result := overlays.DialogResult{Value: "sk-test-key-123"}
	model, _ := m.onKeyInputDialogDone(result, nil)
	m2 := model.(Model)

	assert.Empty(t, m2.providerTarget)
	assert.Equal(t, "anthropic", ps.savedProvider)
	assert.Equal(t, "sk-test-key-123", ps.savedKey)
	assert.True(t, sdkmodel.ProviderHasAuth("anthropic"))

	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "API key saved")
}

func TestModel_OnKeyInputDialogDone_PublishesLoginSuccess(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicAuthLoginSuccess)

	ps := &trackingPS{mockConfig: &mockConfig{}}
	m := newModel(b, ps, ps, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.providerTarget = "openai"

	result := overlays.DialogResult{Value: "sk-openai-key"}
	model, cmd := m.onKeyInputDialogDone(result, nil)
	_ = model.(Model)

	require.NotNil(t, cmd)
	executeBatchCmd(t, cmd)

	evt := <-ch
	assert.Equal(t, topicAuthLoginSuccess, evt.Topic)

	payload, ok := evt.Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "openai", payload["provider"])
}

func TestModel_OnKeyInputDialogDone_EmptyKey(t *testing.T) {
	ps := &trackingPS{mockConfig: &mockConfig{}}
	m := newModel(nil, ps, ps, nil)
	m.providerTarget = "anthropic"

	result := overlays.DialogResult{Value: "   "}
	model, _ := m.onKeyInputDialogDone(result, nil)
	m2 := model.(Model)

	assert.Empty(t, ps.savedProvider)
	assert.Empty(t, m2.chat.Items())
}

func TestModel_OnKeyInputDialogDone_NoConfig(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.providerTarget = "anthropic"

	result := overlays.DialogResult{Value: "sk-test-key"}
	model, _ := m.onKeyInputDialogDone(result, nil)
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "No config available")
}

func TestModel_OnKeyInputDialogDone_NoPS(t *testing.T) {
	m := newModel(nil, &mockConfig{}, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))
	m.providerTarget = "anthropic"

	result := overlays.DialogResult{Value: "sk-test-key"}
	model, _ := m.onKeyInputDialogDone(result, nil)
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "No preference store available")
}

func TestModel_OnLoginFlowResult_OAuthError(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	msg := LoginFlowResultMsg{
		Provider: "openai",
		Error:    errors.New("user denied access"),
	}

	model, _ := m.onLoginFlowResult(msg)
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "OAuth login failed")
	assert.Contains(t, am.Content(), "user denied access")
}

func TestModel_OnLoginFlowResult_Success(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicAuthLoginSuccess)

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, m.chatHeight(24))

	msg := LoginFlowResultMsg{
		Provider: "openai",
		Credential: sdk.OAuthCredential{
			AccessToken: "at-test",
			TokenType:   "bearer",
		},
	}

	model, cmd := m.onLoginFlowResult(msg)
	m2 := model.(Model)

	items := m2.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Successfully logged in")

	require.NotNil(t, cmd)
	executeBatchCmd(t, cmd)

	evt := <-ch
	assert.Equal(t, topicAuthLoginSuccess, evt.Topic)

	payload, ok := evt.Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "openai", payload["provider"])
}

func TestModel_HandleDialogForceCancel_OAuthLogin(t *testing.T) {
	m := newModelNoLanding()

	canceled := false
	m.oauthCancel = func() {
		canceled = true
	}

	loginDlg := overlays.NewLoginDialog(dialogLoginOAuth, overlays.NewLoginModel("Test", "https://example.com"))
	model, _ := m.handleDialogForceCancel(loginDlg)
	m2 := model.(Model)

	assert.True(t, canceled)
	assert.Nil(t, m2.oauthCancel)
}
