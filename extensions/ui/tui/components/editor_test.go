package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEditorModel(t *testing.T) {
	m := NewEditorModel()
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.cursor)
	assert.True(t, m.dirty)
	assert.False(t, m.Focused())
}

func TestEditorFocus(t *testing.T) {
	m := NewEditorModel()
	m = m.Focus()
	assert.True(t, m.Focused())
	m = m.Blur()
	assert.False(t, m.Focused())
}

func TestEditorSetValue(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("hello")
	assert.Equal(t, "hello", m.Value())
	assert.Equal(t, 5, m.cursor)
}

func TestEditorTyping(t *testing.T) {
	m := NewEditorModel().Focus()

	// type "hi"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	assert.Equal(t, "hi", m.Value())
	assert.Equal(t, 2, m.cursor)
}

func TestEditorTypingMultipleRunes(t *testing.T) {
	m := NewEditorModel().Focus()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'b', 'c'}})
	assert.Equal(t, "abc", m.Value())
	assert.Equal(t, 3, m.cursor)
}

func TestEditorBackspace(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetValue("hello")
	m.cursor = 5

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "hell", m.Value())
	assert.Equal(t, 4, m.cursor)

	// backspace at start does nothing
	m.cursor = 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "hell", m.Value())
	assert.Equal(t, 0, m.cursor)
}

func TestEditorDeleteForward(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetValue("hello")
	m.cursor = 2

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	assert.Equal(t, "helo", m.Value())
	assert.Equal(t, 2, m.cursor)

	// delete at end does nothing
	m.cursor = 4
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	assert.Equal(t, "helo", m.Value())
}

func TestEditorCursorMovement(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetValue("hello")

	// move left
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 4, m.cursor)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 3, m.cursor)

	// move right
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, 4, m.cursor)

	// home
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	assert.Equal(t, 0, m.cursor)

	// end
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	assert.Equal(t, 5, m.cursor)

	// left at start stays at 0
	m.cursor = 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 0, m.cursor)

	// right at end stays at end
	m.cursor = 5
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, 5, m.cursor)
}

func TestEditorEnterSubmits(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetValue("hello world")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.cursor)
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Equal(t, "hello world", submit.Text)
}

func TestEditorEnterEmptyDoesNothing(t *testing.T) {
	m := NewEditorModel().Focus()
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Empty(t, m.Value())
	assert.Nil(t, cmd)
}

func TestEditorAltEnterInsertsNewline(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetValue("hello")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	assert.Equal(t, "hello\n", m.Value())
}

func TestEditorSubmitAddsToHistory(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetValue("first")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, []string{"first"}, m.History())

	m = m.SetValue("second")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, []string{"second", "first"}, m.History())
}

func TestEditorSubmitNoDuplicateHistory(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetValue("same")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetValue("same")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, []string{"same"}, m.History())
}

func TestEditorSubmitEmptyNotInHistory(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.PushHistory("")
	assert.Empty(t, m.History())
}

func TestEditorHistoryNavigation(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetValue("first")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetValue("second")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// history: ["second", "first"], histIdx=0 (no selection)
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.histIdx)

	// up once → newest = "second"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, "second", m.Value())
	assert.Equal(t, 1, m.histIdx)

	// up again → older = "first"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, "first", m.Value())
	assert.Equal(t, 2, m.histIdx)

	// up at top stays
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, "first", m.Value())
	assert.Equal(t, 2, m.histIdx)

	// down → "second"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, "second", m.Value())
	assert.Equal(t, 1, m.histIdx)

	// down → empty (current)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.histIdx)

	// down at bottom stays
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Empty(t, m.Value())
	assert.Equal(t, 0, m.histIdx)
}

func TestEditorHistoryEmpty(t *testing.T) {
	m := NewEditorModel().Focus()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Empty(t, m.Value())
}

func TestEditorInsertMidText(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetValue("helo")
	m.cursor = 2

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, "hello", m.Value())
	assert.Equal(t, 3, m.cursor)
}

func TestEditorUnfocusedIgnoresInput(t *testing.T) {
	m := NewEditorModel() // not focused
	assert.False(t, m.Focused())

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	assert.Empty(t, m.Value())
	assert.Nil(t, cmd)
}

func TestEditorSlashCommandAutocomplete(t *testing.T) {
	m := NewEditorModel().Focus()
	m = m.SetSize(40, 3)

	// type "/" triggers autocomplete
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	assert.True(t, m.showAC)
	assert.NotEmpty(t, m.acItems)

	// type "he" → narrows to /help
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'e'}})
	assert.True(t, m.showAC)
	assert.Contains(t, m.acItems, "/help")

	// tab accepts
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.False(t, m.showAC)
	assert.Equal(t, "/help", m.Value())
}

func TestEditorAutocompleteEscapeDismisses(t *testing.T) {
	m := NewEditorModel().Focus()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	assert.True(t, m.showAC)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.False(t, m.showAC)
}

func TestEditorAutocompleteNavigatesWithArrows(t *testing.T) {
	m := NewEditorModel().Focus()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	assert.True(t, m.showAC)
	initial := m.acIndex

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, initial+1, m.acIndex)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, initial, m.acIndex)
}

func TestEditorAutocompleteNoMatch(t *testing.T) {
	m := NewEditorModel().Focus()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/', 'z', 'z', 'z'}})
	assert.False(t, m.showAC)
}

func TestEditorAutocompleteEnterAccepts(t *testing.T) {
	m := NewEditorModel().Focus()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	assert.True(t, m.showAC)

	// enter with AC visible accepts the selection (doesn't submit)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, m.showAC)
	assert.NotEmpty(t, m.Value())
}

func TestEditorAutocompleteSpaceDisables(t *testing.T) {
	m := NewEditorModel().Focus()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	assert.True(t, m.showAC)

	// typing a space after "/" disables autocomplete
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	assert.False(t, m.showAC)
}

func TestEditorSetSize(t *testing.T) {
	m := NewEditorModel()
	m = m.SetSize(100, 5)
	assert.Equal(t, 100, m.Width())
	assert.Equal(t, 5, m.Height())
}

func TestEditorViewRendersCursor(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hi")
	view := m.View()
	assert.Contains(t, view, "hi")
	assert.Contains(t, view, "▎")
}

func TestEditorViewEmptyWidth(t *testing.T) {
	m := NewEditorModel().Focus()
	view := m.View()
	assert.Empty(t, view)
}

func TestWrapText(t *testing.T) {
	lines := wrapText("hello world", 5)
	assert.Equal(t, []string{"hello", " worl", "d"}, lines)
}

func TestWrapTextNewlines(t *testing.T) {
	lines := wrapText("a\nb", 10)
	assert.Equal(t, []string{"a", "b"}, lines)
}

func TestWrapTextEmpty(t *testing.T) {
	lines := wrapText("", 10)
	assert.Equal(t, []string{""}, lines)
}

func TestCursorPosition(t *testing.T) {
	line, col := cursorPosition([]rune("hello"), 3, 10)
	assert.Equal(t, 0, line)
	assert.Equal(t, 3, col)
}

func TestCursorPositionWrap(t *testing.T) {
	line, col := cursorPosition([]rune("hello world"), 7, 5)
	assert.Equal(t, 1, line)
	assert.Equal(t, 2, col)
}

func TestCursorPositionNewline(t *testing.T) {
	line, col := cursorPosition([]rune("hi\nworld"), 5, 10)
	assert.Equal(t, 1, line)
	assert.Equal(t, 2, col)
}

func TestEditorDefaultBorderColor(t *testing.T) {
	m := NewEditorModel()
	assert.Equal(t, "63", m.BorderColor)
}

func TestEditorSetBorderColor(t *testing.T) {
	m := NewEditorModel().SetBorderColor("177")
	assert.Equal(t, "177", m.BorderColor)
}

func TestEditorViewUsesBorderColor(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3).SetBorderColor("99")
	// Verify the field is set (ANSI codes are only rendered in terminal mode)
	assert.Equal(t, "99", m.BorderColor)
	// View should still render without error
	view := m.View()
	assert.NotEmpty(t, view)
}

// --- Draw tests (screen buffer rendering) ---

func TestEditorDraw_NoSize(t *testing.T) {
	m := NewEditorModel().Focus()
	scr := uv.NewScreenBuffer(80, 24)
	// Should not panic with zero dimensions
	m.Draw(scr, uv.Rect(0, 0, 0, 0))
	m.Draw(scr, uv.Rect(0, 0, 0, 10))
	m.Draw(scr, uv.Rect(0, 0, 80, 0))
}

func TestEditorDraw_NoWidth(t *testing.T) {
	m := NewEditorModel().Focus()
	// width not set (0) — Draw should be no-op
	scr := uv.NewScreenBuffer(80, 24)
	m.Draw(scr, uv.Rect(0, 0, 80, 10))
}

func TestEditorDraw_RendersText(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello world")

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello world")
}

func TestEditorDraw_RendersCursor(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hi")

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "▎")
}

func TestEditorDraw_NoCursorWhenBlurred(t *testing.T) {
	m := NewEditorModel().Blur().SetSize(40, 3)
	m = m.SetValue("hi")

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.NotContains(t, rendered, "▎")
}

func TestEditorDraw_WrapsText(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(20, 5)
	m = m.SetValue("hello world this is long")

	scr := uv.NewScreenBuffer(20, 8)
	m.Draw(scr, uv.Rect(0, 0, 20, 8))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello")
	assert.Contains(t, rendered, "long")
}

func TestEditorDraw_AutocompleteOverlay(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	require.True(t, m.AutocompleteVisible())

	scr := uv.NewScreenBuffer(40, 10)
	m.Draw(scr, uv.Rect(0, 0, 40, 10))
	rendered := scr.Render()

	// Should contain some slash command
	assert.Contains(t, rendered, "/help")
}

func TestEditorDraw_OffsetArea(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(30, 3)
	m = m.SetValue("test content")

	scr := uv.NewScreenBuffer(80, 24)
	m.Draw(scr, uv.Rect(20, 10, 30, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "test content")
}

func TestEditorDraw_EmptyContent(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	// Should render the border and cursor, even with no text
	assert.Contains(t, rendered, "▎")
}

func TestEditorDraw_MultilineContent(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 5)
	m = m.SetValue("line one\nline two\nline three")

	scr := uv.NewScreenBuffer(40, 8)
	m.Draw(scr, uv.Rect(0, 0, 40, 8))
	rendered := scr.Render()

	assert.Contains(t, rendered, "line one")
	assert.Contains(t, rendered, "line two")
	assert.Contains(t, rendered, "line three")
}

func TestEditorDraw_MatchesView(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello")

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	drawRendered := uv.TrimSpace(scr.Render())

	viewRendered := m.View()

	assert.Equal(t, viewRendered, drawRendered)
}

func TestEditorDraw_BorderColorApplied(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3).SetBorderColor("99")
	m = m.SetValue("test")

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	// Should render without error and contain the text
	assert.Contains(t, rendered, "test")
	assert.Equal(t, "99", m.BorderColor)
}

func TestEditorDraw_AfterTyping(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'b', 'c'}})

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "abc")
}

func TestEditorDraw_AfterBackspace(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello")
	m.cursor = 5
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hell")
	assert.NotContains(t, rendered, "hello")
}

func TestEditorDraw_AfterUndo(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello")
	m.cursor = 5
	// Delete 'o' (saves undo state)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "hell", m.Value())

	// Undo restores previous state
	m = m.Undo()
	assert.Equal(t, "hello", m.Value())

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello")
}

func TestEditorDraw_AfterHistoryNavigation(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("first")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetValue("second")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Navigate to "second" (most recent)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "second")
}

func TestEditorDraw_CursorMovementUpdates(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello")

	// Move cursor to start
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	assert.Equal(t, 0, m.cursor)

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	// Cursor should be at the beginning of text
	assert.Contains(t, rendered, "▎")
	assert.Contains(t, rendered, "hello")
}

func TestEditorDraw_InsertMidText(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("helo")
	m.cursor = 2

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, "hello", m.Value())

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	// Cursor splits the text: "hel▎lo"
	assert.Contains(t, rendered, "hel")
	assert.Contains(t, rendered, "lo")
}

func TestEditorDraw_WordMovement(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello world test")
	m.cursor = 0

	// Move word right
	m = m.CursorWordRight()
	assert.Equal(t, 6, m.cursor) // "hello "

	// Move word left
	m = m.CursorWordLeft()
	assert.Equal(t, 0, m.cursor)

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello world test")
}

func TestEditorDraw_DeleteWordBackward(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello world")
	m.cursor = 11

	m = m.DeleteWordBackward()

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello")
	assert.NotContains(t, rendered, "world")
}

func TestEditorDraw_DeleteWordForward(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello world")
	m.cursor = 5

	m = m.DeleteWordForward()
	// Deletes " world" (word break + word), leaving "helloworld"
	assert.Equal(t, "helloworld", m.Value())

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello")
	// The space between "hello" and "world" should be gone
	assert.NotContains(t, rendered, "hello world")
}

func TestEditorDraw_DeleteToLineStart(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello world")
	m.cursor = 11

	m = m.DeleteToLineStart()

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.NotContains(t, rendered, "hello")
}

func TestEditorDraw_DeleteToLineEnd(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("hello world")
	m.cursor = 5

	m = m.DeleteToLineEnd()

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello")
	assert.NotContains(t, rendered, "world")
}

func TestEditorDraw_AltEnterNewline(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 5)
	m = m.SetValue("hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})

	scr := uv.NewScreenBuffer(40, 8)
	m.Draw(scr, uv.Rect(0, 0, 40, 8))
	rendered := scr.Render()

	assert.Contains(t, rendered, "hello")
}

func TestEditorDraw_SubmitClearsContent(t *testing.T) {
	m := NewEditorModel().Focus().SetSize(40, 3)
	m = m.SetValue("submit me")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	scr := uv.NewScreenBuffer(40, 5)
	m.Draw(scr, uv.Rect(0, 0, 40, 5))
	rendered := scr.Render()

	assert.NotContains(t, rendered, "submit me")
}
