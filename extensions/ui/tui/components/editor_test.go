package components

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEditorModel(t *testing.T) {
	m := NewEditorModel()
	assert.Empty(t, m.Value())
	assert.True(t, m.Focused())
}

func TestEditorFocus(t *testing.T) {
	m := NewEditorModel()
	assert.True(t, m.Focused())
	m = m.Blur()
	assert.False(t, m.Focused())
	m = m.Focus()
	assert.True(t, m.Focused())
}

func TestEditorSetValue(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("hello")
	assert.Equal(t, "hello", m.Value())
}

func TestEditorTyping(t *testing.T) {
	m := NewEditorModel()

	m, _ = m.Update(tea.KeyPressMsg{Text: "h", Code: 'h'})
	m, _ = m.Update(tea.KeyPressMsg{Text: "i", Code: 'i'})
	assert.Equal(t, "hi", m.Value())
}

func TestEditorTypingMultipleRunes(t *testing.T) {
	m := NewEditorModel()
	m, _ = m.Update(tea.KeyPressMsg{Text: "abc", Code: tea.KeyExtended})
	assert.Equal(t, "abc", m.Value())
}

func TestEditorBackspace(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("hello")

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "hell", m.Value())
}

func TestEditorEnterSubmits(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("hello world")

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Empty(t, m.Value())
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Equal(t, "hello world", submit.Text)
}

func TestEditorEnterEmptyEmitsSubmit(t *testing.T) {
	m := NewEditorModel()
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Empty(t, m.Value())
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Empty(t, submit.Text)
}

func TestEditorSubmitAddsToHistory(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("first")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, []string{"first"}, m.History())

	m = m.SetValue("second")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, []string{"second", "first"}, m.History())
}

func TestEditorSubmitNoDuplicateHistory(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("same")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m.SetValue("same")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, []string{"same"}, m.History())
}

func TestEditorSubmitEmptyNotInHistory(t *testing.T) {
	m := NewEditorModel()
	m = m.PushHistory("")
	assert.Empty(t, m.History())
}

func TestEditorHistoryNavigation(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("first")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m.SetValue("second")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// history: ["second", "first"], histIdx=0 (no selection)
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.histIdx)

	// up once → newest = "second"
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "second", m.Value())
	assert.Equal(t, 1, m.histIdx)

	// up again → older = "first"
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "first", m.Value())
	assert.Equal(t, 2, m.histIdx)

	// up at top stays
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "first", m.Value())
	assert.Equal(t, 2, m.histIdx)

	// down → "second"
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, "second", m.Value())
	assert.Equal(t, 1, m.histIdx)

	// down → empty (current)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.histIdx)

	// down at bottom stays
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.histIdx)
}

func TestEditorHistoryEmpty(t *testing.T) {
	m := NewEditorModel()
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Empty(t, m.Value())
}

func TestEditorUnfocusedIgnoresInput(t *testing.T) {
	m := NewEditorModel().Blur()
	assert.False(t, m.Focused())

	m, cmd := m.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	assert.Empty(t, m.Value())
	assert.Nil(t, cmd)
}

func TestEditorSetSize(t *testing.T) {
	m := NewEditorModel()
	m = m.SetSize(100, 5)
	// textarea subtracts border/padding from width
	assert.GreaterOrEqual(t, m.Width(), 90)
}

func TestEditorViewRenders(t *testing.T) {
	m := NewEditorModel().SetSize(40, 3)
	m = m.SetValue("hi")
	view := m.View()
	assert.Contains(t, view, "hi")
}

func TestEditorViewEmptyWidth(t *testing.T) {
	m := NewEditorModel()
	view := m.View()
	// textarea always renders its border
	assert.NotEmpty(t, view)
}

func TestEditorDefaultBorderColor(t *testing.T) {
	m := NewEditorModel()
	assert.Equal(t, "63", m.BorderColor)
}

func TestEditorSetBorderColor(t *testing.T) {
	m := NewEditorModel().SetBorderColor("177")
	assert.Equal(t, "177", m.BorderColor)
}

// --- Draw tests (screen buffer rendering) ---

func TestEditorDraw_NoSize(t *testing.T) {
	m := NewEditorModel()
	scr := uv.NewScreenBuffer(80, 24)
	// Should not panic with zero dimensions
	m.Draw(scr, uv.Rect(0, 0, 0, 0))
	m.Draw(scr, uv.Rect(0, 0, 0, 10))
	m.Draw(scr, uv.Rect(0, 0, 80, 0))
}

func TestEditorDraw_RendersText(t *testing.T) {
	m := NewEditorModel().SetSize(40, 3)
	m = m.SetValue("hello world")

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello world")
}

func TestEditorDraw_BorderColorApplied(t *testing.T) {
	m := NewEditorModel().SetSize(40, 3).SetBorderColor("99")
	m = m.SetValue("test")

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "test")
	assert.Equal(t, "99", m.BorderColor)
}

func TestEditorDraw_AfterTyping(t *testing.T) {
	m := NewEditorModel().SetSize(40, 3)
	m, _ = m.Update(tea.KeyPressMsg{Text: "abc", Code: tea.KeyExtended})

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "abc")
}

func TestEditorDraw_AfterBackspace(t *testing.T) {
	m := NewEditorModel().SetSize(40, 3)
	m = m.SetValue("hello")

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hell")
	assert.NotContains(t, rendered, "hello")
}

func TestEditorDraw_SubmitClearsContent(t *testing.T) {
	m := NewEditorModel().SetSize(40, 3)
	m = m.SetValue("submit me")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.NotContains(t, rendered, "submit me")
}

func TestEditorDraw_OffsetArea(t *testing.T) {
	m := NewEditorModel().SetSize(30, 3)
	m = m.SetValue("test content")

	scr := uv.NewScreenBuffer(80, 24)
	m.Draw(scr, uv.Rect(20, 10, 30, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "test content")
}

func TestEditorDraw_MultilineContent(t *testing.T) {
	m := NewEditorModel().SetSize(40, 5)
	m = m.SetValue("line one\nline two\nline three")

	scr := uv.NewScreenBuffer(40, 8)
	m.Draw(scr, uv.Rect(0, 0, 40, 8))
	rendered := scr.Render()

	assert.Contains(t, rendered, "line one")
	assert.Contains(t, rendered, "line two")
	assert.Contains(t, rendered, "line three")
}

func TestEditorDraw_AfterHistoryNavigation(t *testing.T) {
	m := NewEditorModel().SetSize(40, 3)
	m = m.SetValue("first")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m.SetValue("second")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Navigate to "second" (most recent)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "second")
}
