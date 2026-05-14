package overlays

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuzzyMatch(t *testing.T) {
	assert.True(t, fuzzyMatch("hello", "hlo"))
	assert.True(t, fuzzyMatch("hello", "he"))
	assert.True(t, fuzzyMatch("Hello World", "hw"))
	assert.True(t, fuzzyMatch("Hello World", "hw"))
	assert.False(t, fuzzyMatch("hello", "hx"))
	assert.True(t, fuzzyMatch("", ""))
	assert.True(t, fuzzyMatch("test", ""))
	assert.False(t, fuzzyMatch("", "x"))
}

func TestFuzzyMatchUnicode(t *testing.T) {
	assert.True(t, fuzzyMatch("café", "cf"))
	assert.True(t, fuzzyMatch("naïve", "nv"))
	assert.True(t, fuzzyMatch("Tokyo東京", "to"))
	assert.False(t, fuzzyMatch("café", "xyz"))
}

func TestNewSelectorModel(t *testing.T) {
	items := []SelectorItem{
		{Title: "Item 1", Subtitle: "sub1"},
		{Title: "Item 2", Subtitle: "sub2"},
	}
	m := NewSelectorModel("Choose", items)
	assert.Equal(t, "Choose", m.title)
	assert.Len(t, m.items, 2)
	assert.Equal(t, 0, m.cursor)
	assert.False(t, m.Visible())
}

func TestSelectorShowHide(t *testing.T) {
	m := NewSelectorModel("Test", nil)
	assert.False(t, m.Visible())

	m = m.Show()
	assert.True(t, m.Visible())
	assert.Empty(t, m.Filter())
	assert.Equal(t, 0, m.Cursor())

	m = m.Hide()
	assert.False(t, m.Visible())
}

func TestSelectorFilterOnTyping(t *testing.T) {
	items := []SelectorItem{
		{Title: "apple"},
		{Title: "banana"},
		{Title: "apricot"},
	}
	m := NewSelectorModel("Fruit", items).Show()

	// type "ap" should match apple and apricot
	m, _ = m.Update(tea.KeyPressMsg{Text: "ap", Code: tea.KeyExtended})
	assert.Equal(t, "ap", m.Filter())
	filtered := m.filteredItems()
	assert.Len(t, filtered, 2)
	assert.Equal(t, "apple", filtered[0].Item.Title)
	assert.Equal(t, "apricot", filtered[1].Item.Title)
}

func TestSelectorFilterBackspace(t *testing.T) {
	items := []SelectorItem{
		{Title: "apple"},
		{Title: "banana"},
	}
	m := NewSelectorModel("Test", items).Show()

	m, _ = m.Update(tea.KeyPressMsg{Text: "ap", Code: tea.KeyExtended})
	assert.Equal(t, "ap", m.Filter())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "a", m.Filter())
}

func TestSelectorNavigation(t *testing.T) {
	items := []SelectorItem{
		{Title: "A"},
		{Title: "B"},
		{Title: "C"},
	}
	m := NewSelectorModel("Test", items).Show()
	assert.Equal(t, 0, m.Cursor())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, m.Cursor())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, m.Cursor())

	// down at bottom stays
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, m.Cursor())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 1, m.Cursor())

	// up at top stays
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, m.Cursor())
}

func TestSelectorEnterSelects(t *testing.T) {
	items := []SelectorItem{
		{Title: "First", Subtitle: "desc1"},
		{Title: "Second", Subtitle: "desc2"},
	}
	m := NewSelectorModel("Test", items).Show()

	// select second item
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	selected, ok := msg.(SelectorSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, 1, selected.Index)
	assert.Equal(t, "Second", selected.Item.Title)
	assert.False(t, m.Visible())
}

func TestSelectorEscapeCancels(t *testing.T) {
	items := []SelectorItem{{Title: "A"}}
	m := NewSelectorModel("Test", items).Show()

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.NotNil(t, cmd)
	assert.False(t, m.Visible())

	msg := cmd()
	_, ok := msg.(SelectorCancelledMsg)
	assert.True(t, ok)
}

func TestSelectorEnterWithNoMatchesDoesNothing(t *testing.T) {
	items := []SelectorItem{{Title: "apple"}}
	m := NewSelectorModel("Test", items).Show()

	// type filter that matches nothing
	m, _ = m.Update(tea.KeyPressMsg{Text: "zz", Code: tea.KeyExtended})
	assert.Empty(t, m.filteredItems())

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
	assert.True(t, m.Visible())
}

func TestSelectorFilterResetsCursor(t *testing.T) {
	items := []SelectorItem{
		{Title: "A"},
		{Title: "B"},
		{Title: "C"},
	}
	m := NewSelectorModel("Test", items).Show()

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 2, m.Cursor())

	// typing resets cursor to 0
	m, _ = m.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	assert.Equal(t, 0, m.Cursor())
}

func TestSelectorSetSize(t *testing.T) {
	m := NewSelectorModel("Test", nil)
	m = m.SetSize(80, 24)
	assert.Equal(t, 80, m.Width())
	assert.Equal(t, 24, m.Height())
}

func TestSelectorViewInvisible(t *testing.T) {
	m := NewSelectorModel("Test", nil)
	assert.Empty(t, m.View())
}

func TestSelectorViewVisible(t *testing.T) {
	items := []SelectorItem{
		{Title: "Item 1", Subtitle: "sub1"},
		{Title: "Item 2", Subtitle: "sub2"},
	}
	m := NewSelectorModel("Choose", items).Show().SetSize(60, 20)
	view := m.View()
	assert.Contains(t, view, "Choose")
	assert.Contains(t, view, "Item 1")
	assert.Contains(t, view, "Item 2")
}

func TestSelectorViewZeroWidth(t *testing.T) {
	m := NewSelectorModel("Test", []SelectorItem{{Title: "A"}}).Show()
	assert.Empty(t, m.View())
}

func TestSelectorFilterMatchesSubtitle(t *testing.T) {
	items := []SelectorItem{
		{Title: "model-a", Subtitle: "Claude Sonnet"},
		{Title: "model-b", Subtitle: "GPT-4o"},
	}
	m := NewSelectorModel("Model", items).Show()

	m, _ = m.Update(tea.KeyPressMsg{Text: "cl", Code: tea.KeyExtended})
	filtered := m.filteredItems()
	assert.Len(t, filtered, 1)
	assert.Equal(t, "model-a", filtered[0].Item.Title)
}

func TestSelectorSelectedMsgIndexMatchesOriginal(t *testing.T) {
	items := []SelectorItem{
		{Title: "A", Subtitle: "first"},
		{Title: "B", Subtitle: "second"},
		{Title: "C", Subtitle: "third"},
	}
	m := NewSelectorModel("Test", items).Show()

	// filter to only match B
	m, _ = m.Update(tea.KeyPressMsg{Text: "b", Code: 'b'})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	selected := msg.(SelectorSelectedMsg)
	assert.Equal(t, 1, selected.Index) // original index, not filtered index
	assert.Equal(t, "B", selected.Item.Title)
}

func TestSelectorDuplicateItemsReturnsCorrectIndex(t *testing.T) {
	items := []SelectorItem{
		{Title: "same", Subtitle: "first"},
		{Title: "same", Subtitle: "second"},
		{Title: "same", Subtitle: "third"},
	}
	m := NewSelectorModel("Test", items).Show()

	// Go to second item (index 1 in filtered = "same/second")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	selected := msg.(SelectorSelectedMsg)
	assert.Equal(t, 1, selected.Index, "should return original index 1, not 0")
	assert.Equal(t, "second", selected.Item.Subtitle)
}

func TestSelectorDraw_Visible(t *testing.T) {
	items := []SelectorItem{
		{Title: "Item 1", Subtitle: "sub1"},
		{Title: "Item 2", Subtitle: "sub2"},
	}
	m := NewSelectorModel("Choose", items).Show().SetSize(60, 20)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "Choose")
	assert.Contains(t, output, "Item 1")
}

func TestSelectorDraw_Invisible(t *testing.T) {
	m := NewSelectorModel("Test", nil)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	// Draw is a no-op when invisible — screen buffer stays blank
}

func TestSelectorDraw_ZeroArea(t *testing.T) {
	items := []SelectorItem{{Title: "A"}}
	m := NewSelectorModel("Test", items).Show().SetSize(60, 20)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, uv.Rect(0, 0, 0, 0))
}

func TestSelectorView_StyledWithPrimaryAccent(t *testing.T) {
	items := []SelectorItem{
		{Title: "Item 1", Subtitle: "sub1"},
		{Title: "Item 2", Subtitle: "sub2"},
	}
	m := NewSelectorModel("Choose", items).Show().SetSize(60, 20)
	view := m.View()
	assert.Contains(t, view, "Choose")
	assert.Contains(t, view, "Item 1")
	// Rounded border should be present
	assert.Contains(t, view, "╭")
}

func TestSelectorView_SelectedItemContrast(t *testing.T) {
	items := []SelectorItem{
		{Title: "First"},
		{Title: "Second"},
	}
	m := NewSelectorModel("Test", items).Show().SetSize(60, 20)
	// First item is selected by default
	view := m.View()
	assert.Contains(t, view, "> First")
}
