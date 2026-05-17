package components

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"

	"github.com/weave-agent/weave-tui/palette"
)

func TestNewCompletionModel(t *testing.T) {
	m := NewCompletionModel()
	assert.False(t, m.Visible())
	assert.Equal(t, CompletionNone, m.Kind())
	assert.Equal(t, 0, m.FilteredCount())
	assert.Equal(t, 0, m.Cursor())
}

func TestCompletionShow(t *testing.T) {
	m := NewCompletionModel()
	items := []CompletionItem{
		{Label: "/help", Description: "Show help", Value: "/help "},
		{Label: "/quit", Description: "Exit", Value: "/quit "},
	}

	m = m.Show(CompletionSlash, items)
	assert.True(t, m.Visible())
	assert.Equal(t, CompletionSlash, m.Kind())
	assert.Equal(t, 2, m.FilteredCount())
	assert.Equal(t, 0, m.Cursor())

	// Selected item is the first one
	item, ok := m.SelectedItem()
	assert.True(t, ok)
	assert.Equal(t, "/help", item.Label)
}

func TestCompletionHide(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "/help", Value: "/help "},
	})
	assert.True(t, m.Visible())

	m = m.Hide()
	assert.False(t, m.Visible())
}

func TestCompletionShowEmptyItems(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{})
	assert.True(t, m.Visible())
	assert.Equal(t, 0, m.FilteredCount())

	_, ok := m.SelectedItem()
	assert.False(t, ok)
}

func TestCompletionSetFilter(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "help", Value: "/help "},
		{Label: "quit", Value: "/quit "},
		{Label: "clear", Value: "/clear "},
	})

	// Filter matching prefix
	m = m.SetFilter("he")
	assert.Equal(t, 1, m.FilteredCount())
	assert.Equal(t, "help", m.filtered[0].Label)

	// Filter reset
	m = m.SetFilter("")
	assert.Equal(t, 3, m.FilteredCount())

	// Case-insensitive filter
	m = m.SetFilter("HE")
	assert.Equal(t, 1, m.FilteredCount())
	assert.Equal(t, "help", m.filtered[0].Label)

	// No match
	m = m.SetFilter("xyz")
	assert.Equal(t, 0, m.FilteredCount())
}

func TestCompletionSetFilterResetsCursor(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "help", Value: "/help "},
		{Label: "quit", Value: "/quit "},
		{Label: "clear", Value: "/clear "},
	})
	m = m.CursorDown().CursorDown() // cursor at 2
	assert.Equal(t, 2, m.Cursor())

	m = m.SetFilter("he")
	assert.Equal(t, 0, m.Cursor())
}

func TestCompletionCursorDown(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "a", Value: "a"},
		{Label: "b", Value: "b"},
		{Label: "c", Value: "c"},
	})

	assert.Equal(t, 0, m.Cursor())
	m = m.CursorDown()
	assert.Equal(t, 1, m.Cursor())
	m = m.CursorDown()
	assert.Equal(t, 2, m.Cursor())
	// Wrap around
	m = m.CursorDown()
	assert.Equal(t, 0, m.Cursor())
}

func TestCompletionCursorUp(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "a", Value: "a"},
		{Label: "b", Value: "b"},
		{Label: "c", Value: "c"},
	})

	// Wrap around to end
	m = m.CursorUp()
	assert.Equal(t, 2, m.Cursor())
	m = m.CursorUp()
	assert.Equal(t, 1, m.Cursor())
	m = m.CursorUp()
	assert.Equal(t, 0, m.Cursor())
}

func TestCompletionCursorEmptyItems(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{})

	// Should not panic
	m = m.CursorDown()
	m = m.CursorUp()
	assert.Equal(t, 0, m.Cursor())
}

func TestCompletionCursorAfterFilter(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "a", Value: "a"},
		{Label: "b", Value: "b"},
		{Label: "c", Value: "c"},
	})

	m = m.CursorDown().CursorDown() // at c
	assert.Equal(t, 2, m.Cursor())

	m = m.SetFilter("b")
	assert.Equal(t, 1, m.FilteredCount())
	assert.Equal(t, 0, m.Cursor())

	m = m.CursorDown() // wrap to 0 since only 1 item
	assert.Equal(t, 0, m.Cursor())
}

func TestCompletionSelectedItem(t *testing.T) {
	m := NewCompletionModel()

	// Not visible
	_, ok := m.SelectedItem()
	assert.False(t, ok)

	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "/help", Description: "Show help", Value: "/help "},
		{Label: "/quit", Value: "/quit "},
	})

	item, ok := m.SelectedItem()
	assert.True(t, ok)
	assert.Equal(t, "/help", item.Label)
	assert.Equal(t, "Show help", item.Description)
	assert.Equal(t, "/help ", item.Value)

	m = m.CursorDown()
	item, ok = m.SelectedItem()
	assert.True(t, ok)
	assert.Equal(t, "/quit", item.Label)

	// After hide
	m = m.Hide()
	_, ok = m.SelectedItem()
	assert.False(t, ok)
}

func TestCompletionSelectedItemEmpty(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{})

	_, ok := m.SelectedItem()
	assert.False(t, ok)
}

func TestCompletionView(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "/help", Description: "Show help", Value: "/help "},
		{Label: "/quit", Value: "/quit "},
	})

	view := m.View()
	assert.NotEmpty(t, view)
	// Border characters should be present
	assert.Contains(t, view, "┌")
	assert.Contains(t, view, "┘")
	// Labels should be present
	assert.Contains(t, view, "/help")
	assert.Contains(t, view, "/quit")
}

func TestCompletionViewNotVisible(t *testing.T) {
	m := NewCompletionModel()
	view := m.View()
	assert.Empty(t, view)
}

func TestCompletionViewEmptyItems(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{})
	view := m.View()
	assert.Empty(t, view)
}

func TestCompletionViewAfterFilterEmpty(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "/help", Value: "/help "},
	})
	m = m.SetFilter("xyz")
	view := m.View()
	assert.Empty(t, view)
}

func TestCompletionViewMaxVisible(t *testing.T) {
	m := NewCompletionModel()
	// Set a smaller maxVisible to test limiting
	m.maxVisible = 3

	items := []CompletionItem{
		{Label: "a", Value: "a"},
		{Label: "b", Value: "b"},
		{Label: "c", Value: "c"},
		{Label: "d", Value: "d"},
		{Label: "e", Value: "e"},
	}
	m = m.Show(CompletionSlash, items)

	view := m.View()
	assert.NotEmpty(t, view)
	// Only 3 items visible, but all labels in first 3 should appear
	assert.Contains(t, view, "a")
	assert.Contains(t, view, "b")
	assert.Contains(t, view, "c")
}

func TestCompletionDraw(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "/help", Value: "/help "},
		{Label: "/quit", Value: "/quit "},
	})

	scr := uv.NewScreenBuffer(60, 20)
	m.Draw(scr, uv.Rect(5, 5, 50, 10))
	rendered := scr.Render()

	assert.Contains(t, rendered, "/help")
	assert.Contains(t, rendered, "/quit")
}

func TestCompletionDrawNotVisible(t *testing.T) {
	m := NewCompletionModel()
	scr := uv.NewScreenBuffer(60, 20)
	m.Draw(scr, uv.Rect(5, 5, 50, 10))
	// Should not panic and not render anything
}

func TestCompletionDrawEmptyItems(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{})
	scr := uv.NewScreenBuffer(60, 20)
	m.Draw(scr, uv.Rect(5, 5, 50, 10))
	// Should not panic
}

func TestCompletionDrawZeroArea(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "/help", Value: "/help "},
	})
	scr := uv.NewScreenBuffer(60, 20)
	m.Draw(scr, uv.Rect(0, 0, 0, 0))
	m.Draw(scr, uv.Rect(0, 0, 10, 0))
	// Should not panic
}

func TestCompletionFileKind(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionFile, []CompletionItem{
		{Label: "main.go", Value: "main.go"},
		{Label: "README.md", Value: "README.md"},
	})

	assert.True(t, m.Visible())
	assert.Equal(t, CompletionFile, m.Kind())
	assert.Equal(t, 2, m.FilteredCount())
}

func TestCompletionShowReplacesItems(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "/help", Value: "/help "},
	})
	m = m.CursorDown() // no-op with 1 item
	m = m.Show(CompletionFile, []CompletionItem{
		{Label: "a.go", Value: "a.go"},
		{Label: "b.go", Value: "b.go"},
	})

	assert.Equal(t, CompletionFile, m.Kind())
	assert.Equal(t, 2, m.FilteredCount())
	assert.Equal(t, 0, m.Cursor())
}

func TestCompletionView_BorderStyle(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "/help", Description: "Show help", Value: "/help "},
	})

	view := m.View()
	assert.NotEmpty(t, view)
	// Border characters should be present
	assert.Contains(t, view, "┌")
	assert.Contains(t, view, "┘")
}

func TestCompletionView_SelectedItemContrast(t *testing.T) {
	m := NewCompletionModel()
	m = m.Show(CompletionSlash, []CompletionItem{
		{Label: "/help", Description: "Show help", Value: "/help "},
		{Label: "/quit", Value: "/quit "},
	})

	view := m.View()
	assert.Contains(t, view, "/help")
	// First item is selected by default — should be bold
	assert.Contains(t, view, "/help")
}

func TestCompletionRenderLineWithDescription(t *testing.T) {
	m := NewCompletionModel()
	item := CompletionItem{Label: "/help", Description: "Show help text", Value: "/help "}

	line := m.renderLine(item, false, 40, palette.DefaultTheme())
	assert.NotEmpty(t, line)
	// Should contain the label
	assert.Contains(t, line, "/help")
}

func TestCompletionRenderLineSelected(t *testing.T) {
	m := NewCompletionModel()
	item := CompletionItem{Label: "/help", Value: "/help "}

	line := m.renderLine(item, true, 40, palette.DefaultTheme())
	assert.NotEmpty(t, line)
	assert.Contains(t, line, "/help")
}

func TestCompletionRenderLineTruncation(t *testing.T) {
	m := NewCompletionModel()
	item := CompletionItem{Label: "/very-long-command-name", Description: "a very long description here", Value: "/very-long-command-name "}

	line := m.renderLine(item, false, 20, palette.DefaultTheme())
	assert.NotEmpty(t, line)
	// Should contain label start
	assert.Contains(t, line, "/very")
}

func TestCompletionCursorDownScrollsViewport(t *testing.T) {
	m := NewCompletionModel()
	m.maxVisible = 3

	items := make([]CompletionItem, 10)
	for i := range 10 {
		items[i] = CompletionItem{Label: string(rune('a' + i)), Value: string(rune('a' + i))}
	}

	m = m.Show(CompletionSlash, items)

	// Cursor down past visible window
	for range 4 {
		m = m.CursorDown()
	}

	assert.Equal(t, 4, m.cursor)
	assert.Equal(t, 2, m.scrollOffset) // items 2,3,4 visible

	// View should only contain visible items
	view := m.View()
	assert.Contains(t, view, "e")    // item 4 is selected
	assert.NotContains(t, view, "a") // item 0 is scrolled out
}

func TestCompletionCursorUpScrollsViewport(t *testing.T) {
	m := NewCompletionModel()
	m.maxVisible = 3

	items := make([]CompletionItem, 10)
	for i := range 10 {
		items[i] = CompletionItem{Label: string(rune('a' + i)), Value: string(rune('a' + i))}
	}

	m = m.Show(CompletionSlash, items)

	// Scroll down to bottom
	for range 9 {
		m = m.CursorDown()
	}

	assert.Equal(t, 9, m.cursor)
	assert.Equal(t, 7, m.scrollOffset)

	// Scroll back up
	for range 3 {
		m = m.CursorUp()
	}

	assert.Equal(t, 6, m.cursor)
	assert.Equal(t, 6, m.scrollOffset) // cursor at bottom of visible window

	view := m.View()
	assert.Contains(t, view, "g")
	assert.Contains(t, view, "h")
	assert.Contains(t, view, "i")
}

func TestCompletionWrapResetsScroll(t *testing.T) {
	m := NewCompletionModel()
	m.maxVisible = 3

	items := make([]CompletionItem, 5)
	for i := range 5 {
		items[i] = CompletionItem{Label: string(rune('a' + i)), Value: string(rune('a' + i))}
	}

	m = m.Show(CompletionSlash, items)

	// Navigate past end to wrap to start
	for range 5 {
		m = m.CursorDown()
	}

	assert.Equal(t, 0, m.cursor)
	assert.Equal(t, 0, m.scrollOffset)
}
