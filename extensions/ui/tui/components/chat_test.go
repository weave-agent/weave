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
	assert.Equal(t, "", m.View())
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
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
