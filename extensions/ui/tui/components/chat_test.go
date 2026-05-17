package components

import (
	"testing"

	"github.com/weave-agent/weave-tui/components/messages"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
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
	assert.Contains(t, ansi.Strip(view), "hello world")
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

// --- Selection state tests ---

func TestChatModel_StartSelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.StartSelection(5, 10)

	assert.True(t, m.selActive)
	assert.True(t, m.MouseDown())
	assert.Equal(t, 5, m.selStartLine)
	assert.Equal(t, 10, m.selStartCol)
	assert.Equal(t, 5, m.selEndLine)
	assert.Equal(t, 10, m.selEndCol)
}

func TestChatModel_ExtendSelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.StartSelection(5, 10)
	m = m.ExtendSelection(7, 20)

	assert.Equal(t, 5, m.selStartLine)
	assert.Equal(t, 10, m.selStartCol)
	assert.Equal(t, 7, m.selEndLine)
	assert.Equal(t, 20, m.selEndCol)
}

func TestChatModel_ExtendSelection_NoActiveSelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.StartSelection(5, 10)
	m = m.EndSelection()
	m = m.ExtendSelection(7, 20)

	// Should not change since selActive is reset by ClearSelection, not EndSelection.
	// EndSelection keeps selActive=true, so this test needs ClearSelection.
	m = m.ClearSelection()
	m = m.ExtendSelection(7, 20)
	assert.Equal(t, 0, m.selEndLine)
	assert.Equal(t, 0, m.selEndCol)
}

func TestChatModel_EndSelection_Normalizes(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.StartSelection(7, 20)
	m = m.ExtendSelection(5, 10)
	m = m.EndSelection()

	// After end, start should be <= end
	sl, sc, el, ec := m.SelectionBounds()
	assert.Equal(t, 5, sl)
	assert.Equal(t, 10, sc)
	assert.Equal(t, 7, el)
	assert.Equal(t, 20, ec)
	assert.True(t, m.selActive)
}

func TestChatModel_EndSelection_NormalizesSameLine(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.StartSelection(5, 20)
	m = m.ExtendSelection(5, 10)
	m = m.EndSelection()

	sl, sc, el, ec := m.SelectionBounds()
	assert.Equal(t, 5, sl)
	assert.Equal(t, 10, sc)
	assert.Equal(t, 5, el)
	assert.Equal(t, 20, ec)
}

func TestChatModel_ClearSelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.StartSelection(5, 10)
	m = m.ExtendSelection(7, 20)
	m = m.ClearSelection()

	assert.False(t, m.selActive)
	assert.False(t, m.MouseDown())
	assert.False(t, m.HasSelection())
}

func TestChatModel_HasSelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)

	// No selection initially
	assert.False(t, m.HasSelection())

	// Single-point selection is not a selection
	m = m.StartSelection(5, 10)
	m = m.EndSelection()
	assert.False(t, m.HasSelection())

	// Multi-line selection
	m = m.StartSelection(5, 10)
	m = m.ExtendSelection(7, 20)
	m = m.EndSelection()
	assert.True(t, m.HasSelection())

	// Same line, different columns
	m = m.ClearSelection()
	m = m.StartSelection(5, 10)
	m = m.ExtendSelection(5, 15)
	m = m.EndSelection()
	assert.True(t, m.HasSelection())
}

func TestChatModel_MouseDown(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	assert.False(t, m.MouseDown())

	m = m.StartSelection(5, 10)
	assert.True(t, m.MouseDown())

	// EndSelection clears mouseDown (mouse released) but keeps selection visible
	m = m.EndSelection()
	assert.False(t, m.MouseDown())
	assert.True(t, m.selActive)

	// ClearSelection removes everything
	m = m.ClearSelection()
	assert.False(t, m.MouseDown())
}

func TestChatModel_SelectionBounds(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)

	// No selection
	sl, sc, el, ec := m.SelectionBounds()
	assert.Equal(t, 0, sl)
	assert.Equal(t, 0, sc)
	assert.Equal(t, 0, el)
	assert.Equal(t, 0, ec)

	// Forward selection
	m = m.StartSelection(3, 5)
	m = m.ExtendSelection(7, 15)
	sl, sc, el, ec = m.SelectionBounds()
	assert.Equal(t, 3, sl)
	assert.Equal(t, 5, sc)
	assert.Equal(t, 7, el)
	assert.Equal(t, 15, ec)

	// Backward selection (should normalize)
	m = m.StartSelection(7, 15)
	m = m.ExtendSelection(3, 5)
	sl, sc, el, ec = m.SelectionBounds()
	assert.Equal(t, 3, sl)
	assert.Equal(t, 5, sc)
	assert.Equal(t, 7, el)
	assert.Equal(t, 15, ec)
}

func TestChatModel_SelectionForLine(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)

	// No selection
	assert.Nil(t, m.selectionForLine(0))

	// Single-line selection
	m = m.StartSelection(2, 5)
	m = m.ExtendSelection(2, 15)
	m = m.EndSelection()

	span := m.selectionForLine(2)
	require.NotNil(t, span)
	assert.Equal(t, 5, span.startCol)
	assert.Equal(t, 15, span.endCol)

	// Line before selection
	assert.Nil(t, m.selectionForLine(1))
	// Line after selection
	assert.Nil(t, m.selectionForLine(3))

	// Multi-line selection
	m = m.ClearSelection()
	m = m.StartSelection(2, 5)
	m = m.ExtendSelection(4, 10)
	m = m.EndSelection()

	// Start line
	span = m.selectionForLine(2)
	require.NotNil(t, span)
	assert.Equal(t, 5, span.startCol)
	assert.Equal(t, 80, span.endCol) // to end of line

	// Middle line
	span = m.selectionForLine(3)
	require.NotNil(t, span)
	assert.Equal(t, 0, span.startCol)
	assert.Equal(t, 80, span.endCol)

	// End line
	span = m.selectionForLine(4)
	require.NotNil(t, span)
	assert.Equal(t, 0, span.startCol)
	assert.Equal(t, 10, span.endCol)

	// Empty single-point selection returns nil
	m = m.ClearSelection()
	m = m.StartSelection(2, 5)
	m = m.EndSelection()
	assert.Nil(t, m.selectionForLine(2))
}

func TestChatModel_Draw_SmallViewport(t *testing.T) {
	m := NewChatModel().SetSize(40, 2)
	m = m.AddItem(stubItem{text: "short"})
	m = m.AddItem(stubItem{text: "tiny"})

	scr := uv.NewScreenBuffer(40, 2)
	m.Draw(scr, uv.Rect(0, 0, 40, 2))
	rendered := scr.Render()

	// With blank separator between items, viewport shows separator + "tiny"
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

	require.True(t, m.AtBottom())
	assert.False(t, m.NewContent())
	assert.False(t, m.TurnEndPending())

	// When at bottom, adding new content should not trigger indicator
	m = m.AddItem(stubItem{text: "line3"})
	assert.True(t, m.AtBottom())
	assert.False(t, m.NewContent())
}

// --- Task 2: Chat spacing and scroll indicator tests ---

func TestChatModel_BlankLineBetweenItems(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.AddItem(stubItem{text: "first"})
	m = m.AddItem(stubItem{text: "second"})

	view := m.View()
	lines := splitLines(view)

	// Should have "first", dot divider, "second", and padding
	require.Len(t, lines, 10)
	assert.Equal(t, "first", lines[0])
	assert.Contains(t, lines[1], "·")
	assert.Equal(t, "second", lines[2])
}

func TestChatModel_BlankLineNotAfterLastItem(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "only"})

	view := m.View()
	lines := splitLines(view)

	// Single item should not have trailing blank line
	assert.Equal(t, "only", lines[0])
	assert.Empty(t, lines[1])
	assert.Empty(t, lines[2])
}

func TestChatModel_Draw_BlankLineBetweenItems(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.AddItem(stubItem{text: "first"})
	m = m.AddItem(stubItem{text: "second"})

	scr := uv.NewScreenBuffer(80, 10)
	m.Draw(scr, uv.Rect(0, 0, 80, 10))
	rendered := scr.Render()

	assert.Contains(t, rendered, "first")
	assert.Contains(t, rendered, "second")
}

func TestChatModel_ScrollIndicator_IsStyledPill(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8"})
	m = m.ScrollUp(5)
	m = m.AddItem(stubItem{text: "line9"})

	require.True(t, m.NewContent())

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))
	rendered := scr.Render()

	assert.Contains(t, rendered, "new content")
	// The indicator should have background styling (background tint color code 234)
	assert.Contains(t, rendered, "234")
}

func TestChatModel_TurnEndIndicator_IsStyledPill(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8"})
	m = m.ScrollUp(5)
	m = m.SetTurnEndPending(true)

	require.True(t, m.TurnEndPending())

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))
	rendered := scr.Render()

	assert.Contains(t, rendered, "scroll to bottom")
	// The indicator should have background styling
	assert.Contains(t, rendered, "234")
}

// --- Selection highlight rendering tests ---

func TestChatModel_Draw_SelectionWithinVisibleArea(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "hello world line1\nhello world line2\nhello world line3"})

	// Select from line 1, col 6 to line 2, col 11
	m = m.StartSelection(1, 6)
	m = m.ExtendSelection(2, 11)
	m = m.EndSelection()

	scr := uv.NewScreenBuffer(80, 5)
	m.Draw(scr, uv.Rect(0, 0, 80, 5))

	// Check that cells in the selection have AttrReverse set
	// Line 1 (screen row 1): col 6 to end of line (80)
	for x := 6; x < 80; x++ {
		cell := scr.CellAt(x, 1)
		require.NotNil(t, cell)
		assert.NotEqual(t, 0, cell.Style.Attrs&uv.AttrReverse, "cell at (%d,1) should have AttrReverse", x)
	}

	// Line 2 (screen row 2): col 0 to 11
	for x := range 11 {
		cell := scr.CellAt(x, 2)
		require.NotNil(t, cell)
		assert.NotEqual(t, 0, cell.Style.Attrs&uv.AttrReverse, "cell at (%d,2) should have AttrReverse", x)
	}

	// Cells outside selection should NOT have AttrReverse
	// Line 0 (screen row 0): no selection
	cell := scr.CellAt(0, 0)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse, "cell at (0,0) should NOT have AttrReverse")

	// Line 1 before col 6
	cell = scr.CellAt(0, 1)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse, "cell at (0,1) should NOT have AttrReverse")

	// Line 2 after col 11
	cell = scr.CellAt(15, 2)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse, "cell at (15,2) should NOT have AttrReverse")
}

func TestChatModel_Draw_SelectionSingleLine(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	// Select from col 6 to col 11 on line 0
	m = m.StartSelection(0, 6)
	m = m.ExtendSelection(0, 11)
	m = m.EndSelection()

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))

	// Selected cols 6-10 should have AttrReverse
	for x := 6; x < 11; x++ {
		cell := scr.CellAt(x, 0)
		require.NotNil(t, cell)
		assert.NotEqual(t, 0, cell.Style.Attrs&uv.AttrReverse, "cell at (%d,0) should have AttrReverse", x)
	}

	// Before selection
	cell := scr.CellAt(5, 0)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse)

	// After selection
	cell = scr.CellAt(11, 0)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse)
}

func TestChatModel_Draw_SelectionPartiallyVisible(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5"})

	// Select lines 1-3 (lines 2-4 in 0-indexed: "line2", "line3", "line4")
	m = m.StartSelection(1, 5)
	m = m.ExtendSelection(3, 3)
	m = m.EndSelection()

	// Scroll down so only part of the selection is visible (lines 2-4)
	m = m.ScrollDown(2)

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))

	// Screen row 0 corresponds to global line 2: full line selection
	for x := range 80 {
		cell := scr.CellAt(x, 0)
		require.NotNil(t, cell)
		assert.NotEqual(t, 0, cell.Style.Attrs&uv.AttrReverse, "cell at (%d,0) should have AttrReverse", x)
	}

	// Screen row 1 corresponds to global line 3: partial (cols 0-3)
	for x := range 3 {
		cell := scr.CellAt(x, 1)
		require.NotNil(t, cell)
		assert.NotEqual(t, 0, cell.Style.Attrs&uv.AttrReverse, "cell at (%d,1) should have AttrReverse", x)
	}

	// After selection on row 1
	cell := scr.CellAt(5, 1)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse)
}

func TestChatModel_Draw_EmptySelection_NoHighlight(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	// Single-point selection (not a real selection)
	m = m.StartSelection(0, 5)
	m = m.EndSelection()

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))

	// No cells should have AttrReverse
	for x := range 80 {
		cell := scr.CellAt(x, 0)
		require.NotNil(t, cell)
		assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse, "cell at (%d,0) should NOT have AttrReverse", x)
	}
}

func TestChatModel_Draw_SelectionClippedToAreaBounds(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	// Select entire line (col 0 to 80)
	m = m.StartSelection(0, 0)
	m = m.ExtendSelection(0, 80)
	m = m.EndSelection()

	// Draw into a smaller area (width 20)
	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 20, 3))

	// Only cols 0-19 should have AttrReverse (clipped to area width)
	for x := range 20 {
		cell := scr.CellAt(x, 0)
		require.NotNil(t, cell)
		assert.NotEqual(t, 0, cell.Style.Attrs&uv.AttrReverse, "cell at (%d,0) should have AttrReverse", x)
	}

	// Cols beyond area width should NOT have AttrReverse
	cell := scr.CellAt(25, 0)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse)
}

func TestChatModel_Draw_SelectionWithOffsetArea(t *testing.T) {
	m := NewChatModel().SetSize(40, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	// Select cols 6-11 on line 0
	m = m.StartSelection(0, 6)
	m = m.ExtendSelection(0, 11)
	m = m.EndSelection()

	// Draw into an offset area
	scr := uv.NewScreenBuffer(80, 24)
	m.Draw(scr, uv.Rect(10, 5, 40, 3))

	// Selection should be at offset screen coordinates
	// Screen row 5, cols 16-21 (10 + 6 to 10 + 11)
	for x := 16; x < 21; x++ {
		cell := scr.CellAt(x, 5)
		require.NotNil(t, cell)
		assert.NotEqual(t, 0, cell.Style.Attrs&uv.AttrReverse, "cell at (%d,5) should have AttrReverse", x)
	}

	// Before selection
	cell := scr.CellAt(15, 5)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse)

	// After selection
	cell = scr.CellAt(21, 5)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse)
}

func TestChatModel_Draw_SelectionExtendsBeyondVisibleArea(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "line1\nline2\nline3\nline4\nline5"})

	// Select from line 0 to line 4 (entire content)
	m = m.StartSelection(0, 0)
	m = m.ExtendSelection(4, 80)
	m = m.EndSelection()

	// Scroll to show only lines 2-4
	m = m.ScrollDown(2)

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))

	// All visible rows should be highlighted
	for row := range 3 {
		for x := range 80 {
			cell := scr.CellAt(x, row)
			require.NotNil(t, cell)
			assert.NotEqual(t, 0, cell.Style.Attrs&uv.AttrReverse,
				"cell at (%d,%d) should have AttrReverse", x, row)
		}
	}
}

func TestChatModel_Draw_NoSelectionWhenInactive(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))

	// No cells should have AttrReverse without active selection
	for x := range 80 {
		cell := scr.CellAt(x, 0)
		require.NotNil(t, cell)
		assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse)
	}
}

func TestChatModel_Draw_BackwardSelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	// Backward selection: start at col 11, end at col 6
	m = m.StartSelection(0, 11)
	m = m.ExtendSelection(0, 6)
	m = m.EndSelection()

	scr := uv.NewScreenBuffer(80, 3)
	m.Draw(scr, uv.Rect(0, 0, 80, 3))

	// After EndSelection normalizes, cols 6-10 should be highlighted
	for x := 6; x < 11; x++ {
		cell := scr.CellAt(x, 0)
		require.NotNil(t, cell)
		assert.NotEqual(t, 0, cell.Style.Attrs&uv.AttrReverse, "cell at (%d,0) should have AttrReverse", x)
	}

	// Before selection
	cell := scr.CellAt(5, 0)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse)

	// After selection
	cell = scr.CellAt(11, 0)
	require.NotNil(t, cell)
	assert.Equal(t, uint8(0), cell.Style.Attrs&uv.AttrReverse)
}

// --- Task 3: ExtractSelection tests ---

func TestChatModel_ExtractSelection_SingleLine(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	m = m.StartSelection(0, 6)
	m = m.ExtendSelection(0, 11)
	m = m.EndSelection()

	result := m.ExtractSelection()
	assert.Equal(t, "world", result)
}

func TestChatModel_ExtractSelection_MultiLine(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "hello world line1\nhello world line2\nhello world line3"})

	// Select from line 1, col 6 to line 2, col 11
	m = m.StartSelection(1, 6)
	m = m.ExtendSelection(2, 11)
	m = m.EndSelection()

	result := m.ExtractSelection()
	assert.Equal(t, "world line2\nhello world", result)
}

func TestChatModel_ExtractSelection_FullLine(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	m = m.StartSelection(0, 0)
	m = m.ExtendSelection(0, 80)
	m = m.EndSelection()

	result := m.ExtractSelection()
	assert.Equal(t, "hello world", result)
}

func TestChatModel_ExtractSelection_EmptySelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	// No selection
	assert.Empty(t, m.ExtractSelection())

	// Single-point selection
	m = m.StartSelection(0, 5)
	m = m.EndSelection()
	assert.Empty(t, m.ExtractSelection())
}

func TestChatModel_ExtractSelection_BackwardSelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	// Backward selection (should normalize after EndSelection)
	m = m.StartSelection(0, 11)
	m = m.ExtendSelection(0, 6)
	m = m.EndSelection()

	result := m.ExtractSelection()
	assert.Equal(t, "world", result)
}

func TestChatModel_ExtractSelection_StripsTrailingWhitespace(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world  "})

	m = m.StartSelection(0, 0)
	m = m.ExtendSelection(0, 80)
	m = m.EndSelection()

	result := m.ExtractSelection()
	assert.Equal(t, "hello world", result)
}

func TestChatModel_ExtractSelection_MultiLineTrailingWhitespace(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	m = m.AddItem(stubItem{text: "line1   \nline2   \nline3"})

	m = m.StartSelection(0, 0)
	m = m.ExtendSelection(2, 80)
	m = m.EndSelection()

	result := m.ExtractSelection()
	assert.Equal(t, "line1\nline2\nline3", result)
}

func TestChatModel_ExtractSelection_WithANSI(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	// Styled text using lipgloss
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	styledText := style.Render("hello world")
	m = m.AddItem(stubItem{text: styledText})

	m = m.StartSelection(0, 6)
	m = m.ExtendSelection(0, 11)
	m = m.EndSelection()

	result := m.ExtractSelection()
	// Should extract plain text without ANSI sequences
	assert.Equal(t, "world", result)
}

func TestChatModel_ExtractSelection_MultiLineWithANSI(t *testing.T) {
	m := NewChatModel().SetSize(80, 5)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	line1 := style.Render("red text here")
	line2 := "plain text here"
	m = m.AddItem(stubItem{text: line1 + "\n" + line2})

	m = m.StartSelection(0, 0)
	m = m.ExtendSelection(1, 80)
	m = m.EndSelection()

	result := m.ExtractSelection()
	assert.Equal(t, "red text here\nplain text here", result)
}

func TestChatModel_ExtractSelection_WithWideCharacters(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	// Emoji and CJK characters are wide (2 columns each)
	m = m.AddItem(stubItem{text: "hello 世界 world"}) // hello 世界 world

	m = m.StartSelection(0, 6)
	m = m.ExtendSelection(0, 10)
	m = m.EndSelection()

	result := m.ExtractSelection()
	// Should extract the wide characters correctly
	assert.Equal(t, "世界", result)
}

func TestChatModel_ExtractSelection_AcrossMultipleItems(t *testing.T) {
	m := NewChatModel().SetSize(80, 10)
	m = m.AddItem(stubItem{text: "first item line1\nfirst item line2"})
	m = m.AddItem(stubItem{text: "second item line1\nsecond item line2"})

	// Select from line 1 of first item through line 1 of second item
	// Global lines: 0=first line1, 1=first line2, 2=dot divider sep, 3=second line1, 4=second line2
	m = m.StartSelection(1, 6)
	m = m.ExtendSelection(3, 7)
	m = m.EndSelection()

	result := m.ExtractSelection()
	assert.Equal(t, "item line2\n·\nsecond", result)
}

func TestChatModel_ExtractSelection_PartiallyBeyondContent(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hi"})

	m = m.StartSelection(0, 0)
	m = m.ExtendSelection(0, 100)
	m = m.EndSelection()

	result := m.ExtractSelection()
	assert.Equal(t, "hi", result)
}

func TestChatModel_ExtractSelection_NoActiveSelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	assert.Empty(t, m.ExtractSelection())
}

func TestChatModel_ExtractSelection_ClearedSelection(t *testing.T) {
	m := NewChatModel().SetSize(80, 3)
	m = m.AddItem(stubItem{text: "hello world"})

	m = m.StartSelection(0, 6)
	m = m.ExtendSelection(0, 11)
	m = m.EndSelection()
	require.Equal(t, "world", m.ExtractSelection())

	m = m.ClearSelection()
	assert.Empty(t, m.ExtractSelection())
}
