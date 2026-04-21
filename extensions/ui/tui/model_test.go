package tui

import (
	"testing"

	"weave/ext/ui/tui/components"
	"weave/ext/ui/tui/components/messages"
	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_HandlesMessageStart(t *testing.T) {
	m := newModel(nil, nil)
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
	assert.Equal(t, "", am.Content())
}

func TestModel_HandlesMessageUpdate(t *testing.T) {
	m := newModel(nil, nil)
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
	m := newModel(nil, nil)
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
	m := newModel(nil, nil)
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
	m := newModel(nil, nil)
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
	m := newModel(nil, nil)
	m.width = 80
	m.height = 10
	m.chat = m.chat.SetSize(80, 10)

	// Update without MessageStart should be ignored
	model, _ := m.Update(MessageUpdateMsg{Content: "orphan"})
	m = model.(Model)

	assert.Empty(t, m.chat.Items())
}

func TestModel_Shutdown(t *testing.T) {
	m := newModel(nil, nil)
	_, cmd := m.Update(ShutdownMsg{})
	require.NotNil(t, cmd)
	// tea.Quit is a func, so we verify it produces a tea.QuitMsg
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "expected tea.QuitMsg from shutdown command")
}

func TestModel_WindowResize(t *testing.T) {
	m := newModel(nil, nil)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = model.(Model)
	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)
	assert.Equal(t, 120, m.chat.Width())
	assert.Equal(t, 40, m.chat.Height())
}

func TestModel_MultipleTurns(t *testing.T) {
	m := newModel(nil, nil)
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
	var _ components.ChatItem = messages.NewAssistantMessage()
	var _ components.ChatItem = messages.NewUserMessage("test")
	var _ components.ChatItem = messages.NewToolPanel("tc1", "bash", "")
}

func TestToolPanelItemIdentity(t *testing.T) {
	// Verify ToolPanel satisfies ChatItemIdentity
	var _ components.ChatItemIdentity = messages.NewToolPanel("tc1", "bash", "")
}

func TestModel_MessageEndCreatesToolPanels(t *testing.T) {
	m := newModel(nil, nil)
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
	m := newModel(nil, nil)
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
	m := newModel(nil, nil)
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
	m := newModel(nil, nil)
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
	m := newModel(nil, nil)
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
