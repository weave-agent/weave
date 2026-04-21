package overlays

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestNewSelectorModel(t *testing.T) {
	items := []SelectorItem{
		{Title: "Item 1", Subtitle: "sub1"},
		{Title: "Item 2", Subtitle: "sub2"},
	}
	m := NewSelectorModel("Choose", items)
	assert.Equal(t, "Choose", m.title)
	assert.Equal(t, 2, len(m.items))
	assert.Equal(t, 0, m.cursor)
	assert.False(t, m.Visible())
}

func TestSelectorShowHide(t *testing.T) {
	m := NewSelectorModel("Test", nil)
	assert.False(t, m.Visible())

	m = m.Show()
	assert.True(t, m.Visible())
	assert.Equal(t, "", m.Filter())
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
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'p'}})
	assert.Equal(t, "ap", m.Filter())
	filtered := m.filteredItems()
	assert.Equal(t, 2, len(filtered))
	assert.Equal(t, "apple", filtered[0].Title)
	assert.Equal(t, "apricot", filtered[1].Title)
}

func TestSelectorFilterBackspace(t *testing.T) {
	items := []SelectorItem{
		{Title: "apple"},
		{Title: "banana"},
	}
	m := NewSelectorModel("Test", items).Show()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'p'}})
	assert.Equal(t, "ap", m.Filter())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
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

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, m.Cursor())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, m.Cursor())

	// down at bottom stays
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, m.Cursor())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 1, m.Cursor())

	// up at top stays
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, m.Cursor())
}

func TestSelectorEnterSelects(t *testing.T) {
	items := []SelectorItem{
		{Title: "First", Subtitle: "desc1"},
		{Title: "Second", Subtitle: "desc2"},
	}
	m := NewSelectorModel("Test", items).Show()

	// select second item
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z', 'z'}})
	assert.Equal(t, 0, len(m.filteredItems()))

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, m.Cursor())

	// typing resets cursor to 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
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
	assert.Equal(t, "", m.View())
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
	assert.Equal(t, "", m.View())
}

func TestSelectorFilterMatchesSubtitle(t *testing.T) {
	items := []SelectorItem{
		{Title: "model-a", Subtitle: "Claude Sonnet"},
		{Title: "model-b", Subtitle: "GPT-4o"},
	}
	m := NewSelectorModel("Model", items).Show()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c', 'l'}})
	filtered := m.filteredItems()
	assert.Equal(t, 1, len(filtered))
	assert.Equal(t, "model-a", filtered[0].Title)
}

func TestSelectorSelectedMsgIndexMatchesOriginal(t *testing.T) {
	items := []SelectorItem{
		{Title: "A", Subtitle: "first"},
		{Title: "B", Subtitle: "second"},
		{Title: "C", Subtitle: "third"},
	}
	m := NewSelectorModel("Test", items).Show()

	// filter to only match B
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	selected := msg.(SelectorSelectedMsg)
	assert.Equal(t, 1, selected.Index) // original index, not filtered index
	assert.Equal(t, "B", selected.Item.Title)
}
