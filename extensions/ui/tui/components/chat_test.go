package components

import (
	"testing"

	"weave/ext/ui/tui/components/messages"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubItem is a simple ChatItem for testing.
type stubItem struct {
	text string
}

func (s stubItem) View(width int) string { return s.text }

func TestChatModel_AddItem(t *testing.T) {
	m := NewChatModel()
	m = m.SetSize(80, 10)

	m = m.AddItem(stubItem{text: "line1"})
	m = m.AddItem(stubItem{text: "line2"})

	require.Len(t, m.Items(), 2)
}

func TestChatModel_UpdateItem(t *testing.T) {
	m := NewChatModel()
	m = m.SetSize(80, 10)

	m = m.AddItem(stubItem{text: "original"})
	m = m.UpdateItem(stubItem{text: "updated"})

	require.Len(t, m.Items(), 1)
	assert.Equal(t, "updated", m.Items()[0].View(80))
}

func TestChatModel_UpdateItem_EmptyList(t *testing.T) {
	m := NewChatModel()
	m = m.UpdateItem(stubItem{text: "appended"})

	require.Len(t, m.Items(), 1)
	assert.Equal(t, "appended", m.Items()[0].View(80))
}

func TestChatModel_View_NoSize(t *testing.T) {
	m := NewChatModel()
	assert.Empty(t, m.View())
}

func TestChatModel_View_SingleItem(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "hello"})

	view := m.View()
	assert.Contains(t, view, "hello")
}

func TestChatModel_View_ScrollsToBottom(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)

	// Add more lines than the viewport
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5"})

	view := m.View()
	// Should show the last 3 lines
	assert.Contains(t, view, "line3")
	assert.Contains(t, view, "line4")
	assert.Contains(t, view, "line5")
	assert.NotContains(t, view, "line1")
	assert.NotContains(t, view, "line2")
}

func TestChatModel_View_PadsToHeight(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "only one line"})

	view := m.View()
	lines := splitLines(view)
	assert.Len(t, lines, 5)
}

func TestChatModel_SetSize(t *testing.T) {
	m := NewChatModel()
	m = m.SetSize(100, 30)
	assert.Equal(t, 100, m.Width())
	assert.Equal(t, 30, m.Height())
}

func TestChatModel_IntegrationWithAssistantMessage(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)

	// Add user message
	m = m.AddItem(messages.NewUserMessage("hello"))

	// Add streaming assistant message
	am := messages.NewAssistantMessage()
	am.Append("hello ")
	am.Append("world")
	m = m.AddItem(am)

	items := m.Items()
	require.Len(t, items, 2)

	view := m.View()
	assert.Contains(t, view, "hello world")
}

func TestChatModel_UpdateItemByID(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)

	// Add user, then tool panel, then another user
	m = m.AddItem(messages.NewUserMessage("first"))
	panel := messages.NewToolPanel("tc1", "bash", "ls")
	panel.SetResult("file.txt", false)
	m = m.AddItem(panel)
	m = m.AddItem(messages.NewUserMessage("second"))

	require.Len(t, m.Items(), 3)

	// Update the tool panel by ID
	updated := messages.NewToolPanel("tc1", "bash", "ls")
	updated.SetResult("new output", false)
	m = m.UpdateItemByID(updated)

	require.Len(t, m.Items(), 3) // still 3 items, not 4

	// Verify the tool panel was updated in place
	tp, ok := m.Items()[1].(*messages.ToolPanel)
	require.True(t, ok)
	assert.Contains(t, tp.View(80), "new output")
}

func TestChatModel_UpdateItemByID_NotFound_Appends(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.AddItem(messages.NewUserMessage("first"))

	panel := messages.NewToolPanel("tc-missing", "bash", "ls")
	m = m.UpdateItemByID(panel)

	require.Len(t, m.Items(), 2) // appended because not found
}

func TestChatModel_IntegrationWithToolPanel(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)

	// Simulate a conversation with tool use
	am := messages.NewAssistantMessage()
	am.Finalize("I'll list the files")
	m = m.AddItem(am)

	panel := messages.NewToolPanel("tc1", "bash", "ls -la")
	panel.SetResult("file1.txt\nfile2.txt", false)
	m = m.AddItem(panel)

	am2 := messages.NewAssistantMessage()
	am2.Finalize("Here are the files")
	m = m.AddItem(am2)

	items := m.Items()
	require.Len(t, items, 3)

	view := m.View()
	assert.Contains(t, view, "file1.txt")
	assert.Contains(t, view, "file2.txt")
}

func TestChatModel_ScrollOffset(t *testing.T) {
	m := NewChatModel()
	assert.Equal(t, 0, m.ScrollOffset())
}

func TestFormatUserMessage(t *testing.T) {
	assert.Equal(t, "> fix the bug", FormatUserMessage("fix the bug"))
}

// splitLines splits a string by newlines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}

	var result []string

	start := 0

	for i := range len(s) {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}

	result = append(result, s[start:])

	return result
}
