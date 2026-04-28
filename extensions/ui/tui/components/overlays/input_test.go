package overlays

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInputModel(t *testing.T) {
	m := NewInputModel("Enter name:")
	assert.Equal(t, "Enter name:", m.prompt)
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.Cursor())
	assert.False(t, m.Visible())
}

func TestInputShowHide(t *testing.T) {
	m := NewInputModel("Test")
	assert.False(t, m.Visible())

	m = m.Show()
	assert.True(t, m.Visible())
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.Cursor())

	m = m.Hide()
	assert.False(t, m.Visible())
}

func TestInputSetSize(t *testing.T) {
	m := NewInputModel("Test")
	m = m.SetSize(80, 24)
	assert.Equal(t, 80, m.Width())
	assert.Equal(t, 24, m.Height())
}

func TestInputTyping(t *testing.T) {
	m := NewInputModel("Name:").Show()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'i'}})
	assert.Equal(t, "hi", m.Value())
	assert.Equal(t, 2, m.Cursor())
}

func TestInputBackspace(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'b', 'c'}})
	assert.Equal(t, "abc", m.Value())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "ab", m.Value())
	assert.Equal(t, 2, m.Cursor())
}

func TestInputBackspaceAtStart(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.Cursor())
}

func TestInputDeleteForward(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'b', 'c'}})
	m.cursor = 1

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	assert.Equal(t, "ac", m.Value())
	assert.Equal(t, 1, m.Cursor())
}

func TestInputCursorMovement(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'b', 'c'}})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 2, m.Cursor())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 1, m.Cursor())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, 2, m.Cursor())

	// left at start
	m.cursor = 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 0, m.Cursor())

	// right at end
	m.cursor = 3
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, 3, m.Cursor())
}

func TestInputEnterSubmits(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t', 'e', 's', 't'}})

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	assert.False(t, m.Visible())

	msg := cmd()
	result, ok := msg.(InputResultMsg)
	require.True(t, ok)
	assert.Equal(t, "test", result.Value)
	assert.True(t, result.Ok)
}

func TestInputEscapeCancels(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)
	assert.False(t, m.Visible())

	msg := cmd()
	result, ok := msg.(InputResultMsg)
	require.True(t, ok)
	assert.False(t, result.Ok)
	assert.Empty(t, result.Value)
}

func TestInputInsertMidText(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'c'}})
	m.cursor = 1

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	assert.Equal(t, "abc", m.Value())
	assert.Equal(t, 2, m.Cursor())
}

func TestInputViewInvisible(t *testing.T) {
	m := NewInputModel("Test")
	assert.Empty(t, m.View())
}

func TestInputViewVisible(t *testing.T) {
	m := NewInputModel("Enter name:").Show().SetSize(60, 20)
	view := m.View()
	assert.Contains(t, view, "Enter name:")
	assert.Contains(t, view, "confirm")
}

func TestInputViewZeroWidth(t *testing.T) {
	m := NewInputModel("Test").Show()
	assert.Empty(t, m.View())
}

func TestInputViewShowsCursor(t *testing.T) {
	m := NewInputModel("Name:").Show().SetSize(60, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'b'}})
	view := m.View()
	assert.Contains(t, view, "ab")
}

func TestInputDraw_Visible(t *testing.T) {
	m := NewInputModel("Enter name:").Show().SetSize(60, 20)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "Enter name:")
	assert.Contains(t, output, "confirm")
}

func TestInputDraw_WithText(t *testing.T) {
	m := NewInputModel("Name:").Show().SetSize(60, 20)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'e', 'l', 'l', 'o'}})
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "hello")
}

func TestInputDraw_Invisible(t *testing.T) {
	m := NewInputModel("Test")
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	// Draw is a no-op when invisible — screen buffer stays blank
}

func TestInputDraw_ZeroArea(t *testing.T) {
	m := NewInputModel("Test").Show().SetSize(60, 20)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, uv.Rect(0, 0, 0, 0))
}
