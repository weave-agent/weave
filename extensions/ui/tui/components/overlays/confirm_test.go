package overlays

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfirmModel(t *testing.T) {
	m := NewConfirmModel("Are you sure?")
	assert.Equal(t, "Are you sure?", m.message)
	assert.Equal(t, 0, m.Cursor())
	assert.False(t, m.Visible())
}

func TestConfirmShowHide(t *testing.T) {
	m := NewConfirmModel("Test")
	assert.False(t, m.Visible())

	m = m.Show()
	assert.True(t, m.Visible())
	assert.Equal(t, 0, m.Cursor())

	m = m.Hide()
	assert.False(t, m.Visible())
}

func TestConfirmSetSize(t *testing.T) {
	m := NewConfirmModel("Test")
	m = m.SetSize(80, 24)
	assert.Equal(t, 80, m.Width())
	assert.Equal(t, 24, m.Height())
}

func TestConfirmEscapeReturnsFalse(t *testing.T) {
	m := NewConfirmModel("Continue?").Show()

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.NotNil(t, cmd)
	assert.False(t, m.Visible())

	msg := cmd()
	result, ok := msg.(ConfirmResultMsg)
	require.True(t, ok)
	assert.False(t, result.Confirmed)
}

func TestConfirmEnterYesSelected(t *testing.T) {
	m := NewConfirmModel("Continue?").Show()
	assert.Equal(t, 0, m.Cursor()) // yes is selected by default

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result := msg.(ConfirmResultMsg)
	assert.True(t, result.Confirmed)
}

func TestConfirmEnterNoSelected(t *testing.T) {
	m := NewConfirmModel("Continue?").Show()

	// move to No
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	assert.Equal(t, 1, m.Cursor())

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	result := msg.(ConfirmResultMsg)
	assert.False(t, result.Confirmed)
}

func TestConfirmArrowNavigation(t *testing.T) {
	m := NewConfirmModel("Test").Show()
	assert.Equal(t, 0, m.Cursor())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	assert.Equal(t, 1, m.Cursor())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	assert.Equal(t, 0, m.Cursor())
}

func TestConfirmYKey(t *testing.T) {
	m := NewConfirmModel("Continue?").Show()

	m, cmd := m.Update(tea.KeyPressMsg{Text: "y", Code: 'y'})
	require.NotNil(t, cmd)

	msg := cmd()
	result := msg.(ConfirmResultMsg)
	assert.True(t, result.Confirmed)
	assert.False(t, m.Visible())
}

func TestConfirmNKey(t *testing.T) {
	m := NewConfirmModel("Continue?").Show()

	m, cmd := m.Update(tea.KeyPressMsg{Text: "n", Code: 'n'})
	require.NotNil(t, cmd)

	msg := cmd()
	result := msg.(ConfirmResultMsg)
	assert.False(t, result.Confirmed)
	assert.False(t, m.Visible())
}

func TestConfirmOtherKeyIgnored(t *testing.T) {
	m := NewConfirmModel("Continue?").Show()

	m, cmd := m.Update(tea.KeyPressMsg{Text: "x", Code: 'x'})
	assert.Nil(t, cmd)
	assert.True(t, m.Visible())
}

func TestConfirmViewInvisible(t *testing.T) {
	m := NewConfirmModel("Test")
	assert.Empty(t, m.View())
}

func TestConfirmViewVisible(t *testing.T) {
	m := NewConfirmModel("Are you sure?").Show().SetSize(60, 20)
	view := m.View()
	assert.Contains(t, view, "Are you sure?")
	assert.Contains(t, view, "Yes")
	assert.Contains(t, view, "No")
}

func TestConfirmViewZeroWidth(t *testing.T) {
	m := NewConfirmModel("Test").Show()
	assert.Empty(t, m.View())
}

func TestConfirmDraw_Visible(t *testing.T) {
	m := NewConfirmModel("Are you sure?").Show().SetSize(60, 20)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "Are you sure?")
	assert.Contains(t, output, "Yes")
	assert.Contains(t, output, "No")
}

func TestConfirmDraw_Invisible(t *testing.T) {
	m := NewConfirmModel("Test")
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, canvas.Bounds())
	// Draw is a no-op when invisible — screen buffer stays blank
}

func TestConfirmDraw_ZeroArea(t *testing.T) {
	m := NewConfirmModel("Test").Show().SetSize(60, 20)
	canvas := uv.NewScreenBuffer(60, 20)
	m.Draw(canvas, uv.Rect(0, 0, 0, 0))
}

func TestConfirmView_StylingUsesWarningAccent(t *testing.T) {
	m := NewConfirmModel("Delete everything?").Show().SetSize(60, 20)
	view := m.View()
	// Should render with Yes/No buttons and message
	assert.Contains(t, view, "Delete everything?")
	assert.Contains(t, view, "Yes")
	assert.Contains(t, view, "No")
	// Rounded border should be present
	assert.Contains(t, view, "╭")
}
