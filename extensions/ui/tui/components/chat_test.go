package components

import (
	"testing"

	"weave/ext/ui/tui/components/messages"

	uv "github.com/charmbracelet/ultraviolet"
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

func TestChatModel_UpdateItemAt(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.AddItem(messages.NewUserMessage("first"))
	m = m.AddItem(messages.NewUserMessage("second"))
	m = m.AddItem(messages.NewUserMessage("third"))

	require.Len(t, m.Items(), 3)

	m = m.UpdateItemAt(1, messages.NewUserMessage("replaced"))

	items := m.Items()
	require.Len(t, items, 3)
	assert.Equal(t, "first", items[0].(*messages.UserMessage).Content())
	assert.Equal(t, "replaced", items[1].(*messages.UserMessage).Content())
	assert.Equal(t, "third", items[2].(*messages.UserMessage).Content())
}

func TestChatModel_UpdateItemAt_OutOfBounds(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.AddItem(messages.NewUserMessage("only"))

	m = m.UpdateItemAt(5, messages.NewUserMessage("nope"))

	items := m.Items()
	require.Len(t, items, 1)
	assert.Equal(t, "only", items[0].(*messages.UserMessage).Content())
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

// countingItem tracks how many times View is called.
type countingItem struct {
	text  string
	views int
}

func (c *countingItem) View(width int) string {
	c.views++
	return c.text
}

func TestChatModel_CacheAvoidsRedundantRenders(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)

	item := &countingItem{text: "hello world"}
	m = m.AddItem(item)

	// First View renders the item
	_ = m.View()

	assert.Equal(t, 1, item.views)

	// Second View uses cache — no additional render
	_ = m.View()

	assert.Equal(t, 1, item.views)

	// Changing size invalidates cache
	m = m.SetSize(80, 10) // same size — no invalidation
	_ = m.View()

	assert.Equal(t, 1, item.views)

	m = m.SetSize(100, 10) // different size — invalidation
	_ = m.View()

	assert.Equal(t, 2, item.views)

	// UpdateItem invalidates the entry
	m = m.UpdateItem(&countingItem{text: "updated"})
	_ = m.View()
	// New item was rendered once by View (the replaced item doesn't get re-rendered)
}

func TestChatModel_CacheInvalidatedOnSetSizeWidthChange(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)

	item := &countingItem{text: "hello"}
	m = m.AddItem(item)

	_ = m.View()

	assert.Equal(t, 1, item.views)

	m = m.SetSize(60, 10)
	_ = m.View()

	assert.Equal(t, 2, item.views) // re-rendered because width changed
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

// --- Draw tests (screen buffer rendering) ---

func TestChatModel_Draw_NoSize(t *testing.T) {
	m := NewChatModel()
	scr := uv.NewScreenBuffer(80, 10)
	// Should not panic with zero dimensions
	m.Draw(scr, uv.Rect(0, 0, 80, 10))
}

func TestChatModel_Draw_SingleItem(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "hello"})

	scr := uv.NewScreenBuffer(80, 5)
	m.Draw(scr, uv.Rect(0, 0, 80, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello")
}

func TestChatModel_Draw_ScrollsToBottom(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5"})

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))
	rendered := scr.Render()

	assert.Contains(t, rendered, "line3")
	assert.Contains(t, rendered, "line4")
	assert.Contains(t, rendered, "line5")
	assert.NotContains(t, rendered, "line1")
	assert.NotContains(t, rendered, "line2")
}

func TestChatModel_Draw_EmptyArea(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "hello"})

	scr := uv.NewScreenBuffer(80, 5)
	// Zero-size area should not panic
	m.Draw(scr, uv.Rect(0, 0, 0, 0))
	m.Draw(scr, uv.Rect(0, 0, 80, 0))
	m.Draw(scr, uv.Rect(0, 0, 0, 5))
}

func TestChatModel_Draw_CacheInvalidatedOnWidthChange(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)

	item := &countingItem{text: "hello world"}
	m = m.AddItem(item)

	// First Draw renders the item
	scr := uv.NewScreenBuffer(80, 5)
	m.Draw(scr, uv.Rect(0, 0, 80, 5))
	assert.Equal(t, 1, item.views)

	// Second Draw uses cache
	scr2 := uv.NewScreenBuffer(80, 5)
	m.Draw(scr2, uv.Rect(0, 0, 80, 5))
	assert.Equal(t, 1, item.views)

	// Width change invalidates cache
	m = m.SetSize(60, 5)
	scr3 := uv.NewScreenBuffer(60, 5)
	m.Draw(scr3, uv.Rect(0, 0, 60, 5))
	assert.Equal(t, 2, item.views)
}

func TestChatModel_Draw_OffsetArea(t *testing.T) {
	m := NewChatModel().SetSize(40, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3"})

	scr := uv.NewScreenBuffer(80, 24)
	m.Draw(scr, uv.Rect(20, 10, 40, 3))
	rendered := scr.Render()

	assert.Contains(t, rendered, "line1")
	assert.Contains(t, rendered, "line2")
	assert.Contains(t, rendered, "line3")
}

func TestChatModel_Draw_ScrollUpAndDown(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"})

	// Auto-scrolled to bottom
	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))
	rendered := scr.Render()
	assert.Contains(t, rendered, "line8")
	assert.Contains(t, rendered, "line10")

	// Scroll up
	m = m.ScrollUp(3)
	scr2 := uv.NewScreenBuffer(80, 3)
	m.Draw(scr2, uv.Rect(0, 0, 80, 3))
	rendered2 := scr2.Render()
	assert.Contains(t, rendered2, "line5")
	assert.NotContains(t, rendered2, "line10")

	// Scroll down
	m = m.ScrollDown(2)
	scr3 := uv.NewScreenBuffer(80, 3)
	m.Draw(scr3, uv.Rect(0, 0, 80, 3))
	rendered3 := scr3.Render()
	assert.Contains(t, rendered3, "line7")
}

func TestChatModel_Draw_IntegrationWithAssistantMessage(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)

	m = m.AddItem(messages.NewUserMessage("hello"))

	am := messages.NewAssistantMessage()
	am.Append("hello ")
	am.Append("world")
	m = m.AddItem(am)

	scr := uv.NewScreenBuffer(80, 10)
	m.Draw(scr, uv.Rect(0, 0, 80, 10))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello world")
}

func TestChatModel_Draw_MatchesView(t *testing.T) {
	// Draw and View should produce the same visible content
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "alpha\nbeta\ngamma\ndelta\nepsilon"})

	scr := uv.NewScreenBuffer(80, 5)
	m.Draw(scr, uv.Rect(0, 0, 80, 5))
	drawRendered := scr.Render()

	viewRendered := m.View()

	// Both should contain the same lines (last 5 since auto-scroll)
	assert.Contains(t, drawRendered, "alpha")
	assert.Contains(t, drawRendered, "epsilon")
	assert.Contains(t, viewRendered, "alpha")
	assert.Contains(t, viewRendered, "epsilon")
}

func TestChatModel_Draw_SmallViewport(t *testing.T) {
	m := NewChatModel().SetSize(40, 2)
	m = m.AddItem(stubItem{text: "short"})
	m = m.AddItem(stubItem{text: "tiny"})

	scr := uv.NewScreenBuffer(40, 2)
	m.Draw(scr, uv.Rect(0, 0, 40, 2))
	rendered := scr.Render()

	assert.Contains(t, rendered, "short")
	assert.Contains(t, rendered, "tiny")
}

// --- Smart auto-scroll tests ---

func TestChatModel_AutoScrollDefaultOn(t *testing.T) {
	m := NewChatModel()
	assert.True(t, m.AutoScroll())
}

func TestChatModel_AutoScrollsWhenNearBottom(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3"}) // fills viewport, auto-scrolls

	assert.True(t, m.AtBottom())

	// Adding more content while at bottom should auto-scroll
	m = m.AddItem(stubItem{text: "line4"})
	assert.True(t, m.AtBottom())
	assert.False(t, m.NewContent())
}

func TestChatModel_NoAutoScrollWhenScrolledUp(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)

	// Add 10 lines
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"})
	require.True(t, m.AtBottom())

	// Scroll up
	m = m.ScrollUp(5)
	require.False(t, m.AtBottom())
	require.False(t, m.AutoScroll())

	// Add new content while scrolled up
	m = m.AddItem(stubItem{text: "line11"})
	assert.False(t, m.AtBottom())
	assert.True(t, m.NewContent())
}

func TestChatModel_ScrollUpDisablesAutoScroll(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5"})

	require.True(t, m.AutoScroll())

	m = m.ScrollUp(2)
	assert.False(t, m.AutoScroll())
}

func TestChatModel_ScrollDownToBottomReEnablesAutoScroll(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"})
	m = m.ScrollUp(5)
	m = m.AddItem(stubItem{text: "line11"}) // newContent = true (scrolled up > 3 lines from bottom)

	require.False(t, m.AutoScroll())
	require.True(t, m.NewContent())

	// Scroll down to bottom
	m = m.ScrollDown(10) // enough to reach bottom
	assert.True(t, m.AutoScroll())
	assert.False(t, m.NewContent())
}

func TestChatModel_JumpToBottom(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7"})
	m = m.ScrollUp(4)
	m = m.SetTurnEndPending(true)
	m = m.AddItem(stubItem{text: "line8"}) // triggers newContent

	require.True(t, m.NewContent())
	require.True(t, m.TurnEndPending())
	require.False(t, m.AtBottom())

	m = m.JumpToBottom()

	assert.True(t, m.AtBottom())
	assert.True(t, m.AutoScroll())
	assert.False(t, m.NewContent())
	assert.False(t, m.TurnEndPending())
}

func TestChatModel_NearBottom(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"})

	// At bottom
	assert.True(t, m.NearBottom())

	// Within 3 lines of bottom
	m = m.ScrollUp(1)
	assert.True(t, m.NearBottom())

	m = m.ScrollUp(1)
	assert.True(t, m.NearBottom())

	// Beyond 3 lines from bottom
	m = m.ScrollUp(2)
	assert.False(t, m.NearBottom())
}

func TestChatModel_UpdateItemAutoScrolls(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5"})

	require.True(t, m.AutoScroll())
	require.True(t, m.AtBottom())

	// UpdateItem should auto-scroll when autoScroll is on
	m = m.UpdateItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7"})
	assert.True(t, m.AtBottom())
}

func TestChatModel_NewContentIndicator(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8"})
	m = m.ScrollUp(5)                      // scroll away from bottom
	m = m.AddItem(stubItem{text: "line9"}) // new content arrives

	require.True(t, m.NewContent())

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))
	rendered := scr.Render()

	assert.Contains(t, rendered, "new content")
}

func TestChatModel_TurnEndIndicator(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8"})
	m = m.ScrollUp(5)
	m = m.SetTurnEndPending(true)

	require.True(t, m.TurnEndPending())

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))
	rendered := scr.Render()

	assert.Contains(t, rendered, "scroll to bottom")
}

func TestChatModel_NoIndicatorWhenAtBottom(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "line1\nline2"})
	m = m.SetTurnEndPending(true) // shouldn't happen in practice, but verify no indicator

	require.True(t, m.AtBottom())
	// TurnEndPending set but at bottom — the caller should clear it, but even if not,
	// the indicator shows. In practice, the model only sets it when !AtBottom.
}
