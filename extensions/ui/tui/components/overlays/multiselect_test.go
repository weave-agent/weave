package overlays

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMultiSelectModel(t *testing.T) {
	items := []string{"apple", "banana", "cherry"}
	defaults := []bool{true, false, true}
	m := NewMultiSelectModel("Choose", items, defaults)

	assert.Equal(t, "Choose", m.title)
	assert.Len(t, m.items, 3)
	assert.Equal(t, 0, m.cursor)
	assert.False(t, m.Visible())
	assert.True(t, m.IsSelected(0))
	assert.False(t, m.IsSelected(1))
	assert.True(t, m.IsSelected(2))
}

func TestMultiSelectDefaultsLengthMismatch(t *testing.T) {
	items := []string{"a", "b"}
	defaults := []bool{true, false, true} // longer than items
	m := NewMultiSelectModel("Test", items, defaults)

	assert.True(t, m.IsSelected(0))
	assert.False(t, m.IsSelected(1))
	// index 2 should not be selected because it's out of range
	assert.False(t, m.IsSelected(2))
}

func TestMultiSelectShowHide(t *testing.T) {
	m := NewMultiSelectModel("Test", []string{"a"}, nil)
	assert.False(t, m.Visible())

	m = m.Show()
	assert.True(t, m.Visible())

	m = m.Hide()
	assert.False(t, m.Visible())
}

func TestMultiSelectSetSize(t *testing.T) {
	m := NewMultiSelectModel("Test", nil, nil)
	m = m.SetSize(80, 24)
	assert.Equal(t, 80, m.Width())
	assert.Equal(t, 24, m.Height())
}

func TestMultiSelectNavigation(t *testing.T) {
	items := []string{"A", "B", "C"}
	m := NewMultiSelectModel("Test", items, nil).Show()
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

func TestMultiSelectToggleWithEnter(t *testing.T) {
	items := []string{"A", "B", "C"}
	m := NewMultiSelectModel("Test", items, nil).Show()
	assert.False(t, m.IsSelected(0))

	// Enter toggles selection
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, m.IsSelected(0))

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, m.IsSelected(0))
}

func TestMultiSelectToggleWithSpace(t *testing.T) {
	items := []string{"A", "B", "C"}
	m := NewMultiSelectModel("Test", items, nil).Show()

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.True(t, m.IsSelected(0))

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	assert.False(t, m.IsSelected(0))
}

func TestMultiSelectCtrlEnterConfirms(t *testing.T) {
	items := []string{"A", "B", "C"}
	m := NewMultiSelectModel("Test", items, nil).Show()

	// Select first and third items
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	require.NotNil(t, cmd)
	assert.False(t, m.Visible())

	msg := cmd()
	result, ok := msg.(MultiSelectResultMsg)
	require.True(t, ok)
	assert.True(t, result.Ok)
	assert.Equal(t, []int{0, 2}, result.Selected)
}

func TestMultiSelectEscapeCancels(t *testing.T) {
	items := []string{"A", "B"}
	m := NewMultiSelectModel("Test", items, nil).Show()

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.NotNil(t, cmd)
	assert.False(t, m.Visible())

	msg := cmd()
	result, ok := msg.(MultiSelectResultMsg)
	require.True(t, ok)
	assert.False(t, result.Ok)
	assert.Nil(t, result.Selected)
}

func TestMultiSelectConfirmWithNoSelection(t *testing.T) {
	items := []string{"A", "B"}
	m := NewMultiSelectModel("Test", items, nil).Show()

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(MultiSelectResultMsg)
	require.True(t, ok)
	assert.True(t, result.Ok)
	assert.Empty(t, result.Selected)
}

func TestMultiSelectSelectedReturnsSorted(t *testing.T) {
	items := []string{"A", "B", "C", "D"}
	m := NewMultiSelectModel("Test", items, nil).Show()

	// Select items out of order
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // select C (index 2)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // select A (index 0)

	assert.Equal(t, []int{0, 2}, m.Selected())
}

func TestMultiSelectViewInvisible(t *testing.T) {
	m := NewMultiSelectModel("Test", nil, nil)
	assert.Empty(t, m.View())
}

func TestMultiSelectViewVisible(t *testing.T) {
	items := []string{"Item 1", "Item 2"}
	m := NewMultiSelectModel("Choose", items, nil).Show().SetSize(60, 20)
	view := m.View()
	assert.Contains(t, view, "Choose")
	assert.Contains(t, view, "Item 1")
	assert.Contains(t, view, "Item 2")
}

func TestMultiSelectViewZeroWidth(t *testing.T) {
	m := NewMultiSelectModel("Test", []string{"A"}, nil).Show()
	assert.Empty(t, m.View())
}

func TestMultiSelectViewShowsCheckboxes(t *testing.T) {
	items := []string{"A", "B"}
	m := NewMultiSelectModel("Test", items, []bool{true, false}).Show().SetSize(60, 20)
	view := m.View()
	assert.Contains(t, view, "☑")
	assert.Contains(t, view, "☐")
}

func TestMultiSelectDraw_Visible(t *testing.T) {
	items := []string{"Item 1", "Item 2"}
	m := NewMultiSelectModel("Choose", items, nil).Show().SetSize(60, 20)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "Choose")
	assert.Contains(t, output, "Item 1")
}

func TestMultiSelectDraw_Invisible(t *testing.T) {
	m := NewMultiSelectModel("Test", nil, nil)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	// Draw is a no-op when invisible
}

func TestMultiSelectDraw_ZeroArea(t *testing.T) {
	m := NewMultiSelectModel("Test", []string{"A"}, nil).Show().SetSize(60, 20)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, uv.Rect(0, 0, 0, 0))
}

func TestMultiSelectView_StyledWithPrimaryAccent(t *testing.T) {
	items := []string{"Item 1", "Item 2"}
	m := NewMultiSelectModel("Choose", items, nil).Show().SetSize(60, 20)
	view := m.View()
	assert.Contains(t, view, "Choose")
	assert.Contains(t, view, "Item 1")
	// Rounded border should be present
	assert.Contains(t, view, "╭")
}

func TestMultiSelectEmptyItems(t *testing.T) {
	m := NewMultiSelectModel("Test", nil, nil).Show().SetSize(60, 20)
	view := m.View()
	assert.Contains(t, view, "No items")
}

func TestMultiSelectNoOpOnOtherKeys(t *testing.T) {
	items := []string{"A", "B"}
	m := NewMultiSelectModel("Test", items, nil).Show()
	assert.True(t, m.Visible())

	m, cmd := m.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	assert.Nil(t, cmd)
	assert.True(t, m.Visible())
}
