package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"weave/bus"
	"weave/ext/ui/tui/components"
	"weave/ext/ui/tui/components/attachments"
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"
	"weave/sdk"
	sdkmodel "weave/sdk/model"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	m := newModel(nil, nil, nil)
	m.showLanding = false

	return m
}

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
	assert.Contains(t, view.Content, "[thinking]")
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

	assert.False(t, m.dialogStack.Empty())
	assert.Equal(t, sessions, m.pendingSessions)

	// Verify dialog renders session selector content
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "Resume Session")
}

func TestModel_SessionListEmpty(t *testing.T) {
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	require.False(t, m.dialogStack.Empty())

	// Select first item
	model, cmd := m.Update(overlays.SelectorSelectedMsg{Index: 0, Item: overlays.SelectorItem{
		Title: "/project", Subtitle: "2026-01-01 12:00",
	}})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
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
	require.False(t, m.dialogStack.Empty())

	// Regular key press should go to overlay, not editor
	model, _ = m.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	m = model.(Model)

	// Dialog should still be active (key was a filter char)
	assert.False(t, m.dialogStack.Empty())
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
	assert.True(t, m.dialogStack.Empty())
}

func TestModel_InterruptStreaming(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicInterrupt)

	m := newModel(b, nil, nil)
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

func TestModel_DefaultThinkingLevel(t *testing.T) {
	m := newModel(nil, nil, nil)
	assert.Equal(t, sdkmodel.ThinkingMedium, m.thinkingLevel)
	assert.Equal(t, "medium", m.footer.ThinkingLevel())
}

func TestModel_ThinkingLevelFromEnv(t *testing.T) {
	t.Setenv("WEAVE_THINKING_LEVEL", "high")

	m := newModel(nil, nil, nil)
	assert.Equal(t, sdkmodel.ThinkingHigh, m.thinkingLevel)
	assert.Equal(t, "high", m.footer.ThinkingLevel())
}

func TestModel_ThinkingLevelInvalidEnv(t *testing.T) {
	t.Setenv("WEAVE_THINKING_LEVEL", "invalid")

	m := newModel(nil, nil, nil)
	assert.Equal(t, sdkmodel.ThinkingMedium, m.thinkingLevel)
}

func TestModel_CycleThinkingLevel(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}

	assert.Equal(t, sdkmodel.ThinkingMedium, m.thinkingLevel)

	model, _ := m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingHigh, m.thinkingLevel)
	assert.Equal(t, "high", m.footer.ThinkingLevel())
	assert.Equal(t, "139", m.editor.BorderColor)

	// Second press skips xhigh (clamped for Sonnet) and goes to off
	model, _ = m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingOff, m.thinkingLevel)
}

func TestModel_CycleThinkingLevelWraps(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.thinkingLevel = sdkmodel.ThinkingXHigh
	m.editor = m.editor.SetBorderColor("177")

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

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}

	// Sonnet doesn't support xhigh, so the cycle skips it:
	// medium -> high -> off -> minimal -> low -> medium (wraps)
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

	m := newModel(nil, nil, nil)
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

	m := newModel(b, nil, nil)
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
	m := newModel(nil, nil, nil)
	assert.Equal(t, "99", m.editor.BorderColor) // medium = "99"
}

func TestModel_ThinkingLevelUpdatesEditorBorder(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-opus-4-7", Provider: "anthropic", Reasoning: true, SupportsXHigh: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-opus-4-7"}

	// medium -> high
	model, _ := m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingHigh, m.thinkingLevel)
	assert.Equal(t, "139", m.editor.BorderColor) // high = "139"

	// high -> xhigh
	model, _ = m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)
	assert.Equal(t, sdkmodel.ThinkingXHigh, m.thinkingLevel)
	assert.Equal(t, "177", m.editor.BorderColor) // xhigh = "177"
}

func TestModel_ThinkingCommand(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicThinkingChange)

	m := newModel(b, nil, nil)
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
	assert.Equal(t, "139", m.editor.BorderColor)

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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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

	m := newModel(nil, nil, nil)
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
	assert.Equal(t, "139", m.editor.BorderColor)
}

func TestModel_ThinkingCommandAllLevels(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-opus-4-7", Provider: "anthropic", Reasoning: true, SupportsXHigh: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	m.AddUserMessage("hello")

	assert.True(t, m.showHints)

	view := m.View()
	assert.NotContains(t, view.Content, "ctrl+p cycle model")
}

// --- Draw tests (screen buffer rendering) ---

func TestModel_Draw_RendersAllSections(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 120
	m.height = 40

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	// Footer should contain model info (at minimum the provider name)
	assert.NotEmpty(t, m.footer.View())
	// Editor border should be present
	assert.Contains(t, rendered, "│")
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.width = 120
	m.height = 30

	m.showStatus("test status message")

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	assert.Contains(t, rendered, "test status message")
}

func TestModel_Draw_OverlayFillsScreen(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	// Activate model selector dialog
	models := listModels()
	if len(models) > 1 {
		items := make([]overlays.SelectorItem, len(models))
		for i, model := range models {
			items[i] = overlays.SelectorItem{Title: model.DisplayName()}
		}

		sel := overlays.NewSelectorModel("Select Model", items)
		sel = sel.SetSize(80, 24).Show()
		m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogModelSelect, sel))

		canvas := uv.NewScreenBuffer(m.width, m.height)
		m.Draw(canvas, canvas.Bounds())
		rendered := canvas.Render()

		assert.Contains(t, rendered, "Select Model")
	}
}

func TestModel_Draw_SmallTerminal(t *testing.T) {
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	// Short paste — goes to editor
	model, _ := m.Update(tea.PasteMsg{Content: "short text"})
	m = model.(Model)

	assert.Empty(t, m.attach.Items())
	assert.Contains(t, m.editor.Value(), "short text")
}

func TestModel_PasteDetection_CharThreshold(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	// Long text without newlines (>1000 chars)
	longText := strings.Repeat("x", 1001)

	model, _ := m.Update(tea.PasteMsg{Content: longText})
	m = model.(Model)

	assert.Len(t, m.attach.Items(), 1)
}

func TestModel_AttachmentDeleteMode_Toggle(t *testing.T) {
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m = addTestAttachment(m, "a.go", "aaa", 1)
	m.attach = m.attach.ToggleDeleteMode()

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(Model)
	assert.False(t, m.attach.InDeleteMode())
}

func TestModel_AttachmentDeleteMode_UpNav(t *testing.T) {
	m := newModel(nil, nil, nil)
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
	m := newModel(b, nil, nil)
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
	assert.Contains(t, content, `<file name="test.go">`)
	assert.Contains(t, content, "package main")

	// Followup should be published
	require.NotNil(t, cmd)
}

func TestModel_SubmitNoAttachments(t *testing.T) {
	b := bus.New()
	m := newModel(b, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m = addTestAttachment(m, "a.go", "aaa", 1)

	model, _ := m.dispatchBinding(ActionNewSession)
	m = model.(Model)
	assert.Empty(t, m.attach.Items())
}

// --- Completion integration tests ---

func TestModel_RefreshEditorCompletion_Empty(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_RefreshEditorCompletion_PlainText(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("hello world")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_RefreshEditorCompletion_SlashCommand(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("/he")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionSlash, m.editor.Completion().Kind())
	assert.Equal(t, 1, m.editor.Completion().FilteredCount()) // only /help matches
}

func TestModel_RefreshEditorCompletion_SlashCommandNoFilter(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("/")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionSlash, m.editor.Completion().Kind())
	assert.Positive(t, m.editor.Completion().FilteredCount())
}

func TestModel_RefreshEditorCompletion_SlashCommandWithSpaceNoAcceptsFiles(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("/help ")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_RefreshEditorCompletion_SlashCommandWithSpaceAcceptsFiles(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.commands.Register("/upload", "Upload files", true, func(_ string) CommandResult {
		return CommandResult{}
	})
	m.editor = m.editor.SetValue("/upload ")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_AtTrigger(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("text @")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_AtTriggerWithFilter(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("text @go")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_AtTriggerAtStart(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("@mod")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_NoWhitespaceBeforeAt(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("hello@world")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_RefreshEditorCompletion_HidesWhenContextGone(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("/he")
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())

	m.editor = m.editor.SetValue("he")
	m = m.refreshEditorCompletion()
	assert.False(t, m.editor.CompletionActive())
}

func TestModel_SlashCommandsUpdatedMsg_RefreshesCompletion(t *testing.T) {
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("/")
	m = m.refreshEditorCompletion()
	require.True(t, m.editor.CompletionActive())

	// Tab should be intercepted
	handled, _, _ := m.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.True(t, handled)
}

func TestModel_HandleCompletionKey_WhenInactive(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("plain")
	require.False(t, m.editor.CompletionActive())

	// Tab should not be intercepted
	handled, _, _ := m.handleCompletionKey(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.False(t, handled)
}

func TestModel_HandleCompletionKey_RegularKey(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("/he")
	m = m.refreshEditorCompletion()
	require.True(t, m.editor.CompletionActive())

	// Regular key should not be intercepted
	handled, _, _ := m.handleCompletionKey(tea.KeyPressMsg{Text: "a", Code: 'a'})
	assert.False(t, handled)
}

func TestModel_CompletionKeyFlow_TabCycles(t *testing.T) {
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.editor = m.editor.SetValue("line one\nline two @")
	// Position cursor on second line, after @
	m.editor, _ = m.editor.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = m.refreshEditorCompletion()
	assert.True(t, m.editor.CompletionActive())
	assert.Equal(t, components.CompletionFile, m.editor.Completion().Kind())
}

func TestModel_RefreshEditorCompletion_MultilineSlashCommand(t *testing.T) {
	m := newModel(nil, nil, nil)
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
