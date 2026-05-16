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

	m, _ = m.Update(tea.KeyPressMsg{Text: "hi", Code: tea.KeyExtended})
	assert.Equal(t, "hi", m.Value())
	assert.Equal(t, 2, m.Cursor())
}

func TestInputBackspace(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyPressMsg{Text: "abc", Code: tea.KeyExtended})
	assert.Equal(t, "abc", m.Value())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "ab", m.Value())
	assert.Equal(t, 2, m.Cursor())
}

func TestInputBackspaceAtStart(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.Cursor())
}

func TestInputDeleteForward(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyPressMsg{Text: "abc", Code: tea.KeyExtended})
	m.cursor = 1

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDelete})
	assert.Equal(t, "ac", m.Value())
	assert.Equal(t, 1, m.Cursor())
}

func TestInputCursorMovement(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyPressMsg{Text: "abc", Code: tea.KeyExtended})

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	assert.Equal(t, 2, m.Cursor())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	assert.Equal(t, 1, m.Cursor())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	assert.Equal(t, 2, m.Cursor())

	// left at start
	m.cursor = 0
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	assert.Equal(t, 0, m.Cursor())

	// right at end
	m.cursor = 3
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	assert.Equal(t, 3, m.Cursor())
}

func TestInputEnterSubmits(t *testing.T) {
	m := NewInputModel("Name:").Show()
	m, _ = m.Update(tea.KeyPressMsg{Text: "test", Code: tea.KeyExtended})

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	m, _ = m.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
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
	m, _ = m.Update(tea.KeyPressMsg{Text: "ac", Code: tea.KeyExtended})
	m.cursor = 1

	m, _ = m.Update(tea.KeyPressMsg{Text: "b", Code: 'b'})
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
	m, _ = m.Update(tea.KeyPressMsg{Text: "ab", Code: tea.KeyExtended})
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
	m, _ = m.Update(tea.KeyPressMsg{Text: "hello", Code: tea.KeyExtended})
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

func TestInputView_StyledWithRoundedBorder(t *testing.T) {
	m := NewInputModel("Enter name:").Show().SetSize(60, 20)
	view := m.View()
	assert.Contains(t, view, "Enter name:")
	assert.Contains(t, view, "confirm")
	// Rounded border should be present
	assert.Contains(t, view, "╭")
}

func TestInputSetMask(t *testing.T) {
	m := NewInputModel("Password:").SetMask('*')
	assert.Equal(t, rune('*'), m.mask)
}

func TestInputTypingWithMask(t *testing.T) {
	m := NewInputModel("Password:").Show().SetMask('*')
	m, _ = m.Update(tea.KeyPressMsg{Text: "secret", Code: tea.KeyExtended})
	assert.Equal(t, "secret", m.Value())
	assert.Equal(t, 6, m.Cursor())
}

func TestInputViewMasked(t *testing.T) {
	m := NewInputModel("Password:").Show().SetSize(60, 20).SetMask('*')
	m, _ = m.Update(tea.KeyPressMsg{Text: "secret", Code: tea.KeyExtended})
	view := m.View()
	assert.Contains(t, view, "Password:")
	assert.Contains(t, view, "******")
	assert.NotContains(t, view, "secret")
}

func TestInputViewMaskedCursor(t *testing.T) {
	m := NewInputModel("Password:").Show().SetSize(60, 20).SetMask('*')
	m, _ = m.Update(tea.KeyPressMsg{Text: "ab", Code: tea.KeyExtended})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	view := m.View()
	assert.Contains(t, view, "*▎*")
	assert.NotContains(t, view, "ab")
}

func TestInputDraw_Masked(t *testing.T) {
	m := NewInputModel("Password:").Show().SetSize(60, 20).SetMask('*')
	m, _ = m.Update(tea.KeyPressMsg{Text: "hello", Code: tea.KeyExtended})
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "*****")
	assert.NotContains(t, output, "hello")
}

func TestInputMaskZeroMeansNoMasking(t *testing.T) {
	m := NewInputModel("Name:").Show().SetSize(60, 20)
	m, _ = m.Update(tea.KeyPressMsg{Text: "hello", Code: tea.KeyExtended})
	view := m.View()
	assert.Contains(t, view, "hello")
	assert.NotContains(t, view, "*****")
}
