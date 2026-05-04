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

func TestEditorAltEnterInsertsNewline(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("hello")

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	assert.Contains(t, m.Value(), "hello")
	assert.Contains(t, m.Value(), "\n")
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

// --- Completion tests ---

func TestEditorShowCompletion(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("/he")
	items := []CompletionItem{
		{Label: "help", Description: "Show help", Value: "/help "},
		{Label: "quit", Value: "/quit "},
	}

	m = m.ShowCompletion(CompletionSlash, items, "he", 0)
	assert.True(t, m.CompletionActive())
	assert.Equal(t, 1, m.Completion().FilteredCount())
	assert.Equal(t, "help", m.Completion().filtered[0].Label)
}

func TestEditorHideCompletion(t *testing.T) {
	m := NewEditorModel()
	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "help", Value: "/help "},
	}, "", 0)
	assert.True(t, m.CompletionActive())

	m = m.HideCompletion()
	assert.False(t, m.CompletionActive())
}

func TestEditorCompletionActive(t *testing.T) {
	m := NewEditorModel()
	assert.False(t, m.CompletionActive())

	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "help", Value: "/help "},
	}, "", 0)
	assert.True(t, m.CompletionActive())
}

func TestEditorCompletionTabCyclesDown(t *testing.T) {
	m := NewEditorModel()
	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "a", Value: "a"},
		{Label: "b", Value: "b"},
		{Label: "c", Value: "c"},
	}, "", 0)

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, 1, m.Completion().Cursor())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, 2, m.Completion().Cursor())
}

func TestEditorCompletionUpNavigates(t *testing.T) {
	m := NewEditorModel()
	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "a", Value: "a"},
		{Label: "b", Value: "b"},
		{Label: "c", Value: "c"},
	}, "", 0)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // cursor at 1

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, m.Completion().Cursor())
}

func TestEditorCompletionDownNavigates(t *testing.T) {
	m := NewEditorModel()
	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "a", Value: "a"},
		{Label: "b", Value: "b"},
	}, "", 0)

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, m.Completion().Cursor())
}

func TestEditorCompletionEnterAppliesAndSubmits(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("/he")
	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "help", Value: "/help "},
	}, "he", 0)

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, m.CompletionActive())
	assert.Empty(t, m.Value()) // submitted and cleared
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Equal(t, "/help", submit.Text)
}

func TestEditorCompletionEnterEmptyItemsSubmitsRaw(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("/xyz")
	m = m.ShowCompletion(CompletionSlash, []CompletionItem{}, "xyz", 0)

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, m.CompletionActive())
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Equal(t, "/xyz", submit.Text)
}

func TestEditorCompletionEscDismisses(t *testing.T) {
	m := NewEditorModel()
	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "help", Value: "/help "},
	}, "", 0)
	assert.True(t, m.CompletionActive())

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, m.CompletionActive())
	assert.Nil(t, cmd)
}

func TestEditorCompletionKeysPassThroughWhenNotVisible(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("hello")

	// Tab should NOT be intercepted when completion is not visible
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Equal(t, "hello", m.Value())

	// Up/Down should work as history navigation
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "hello", m.Value())
}

func TestEditorCompletionHidesOnHistoryUp(t *testing.T) {
	m := NewEditorModel()
	m = m.PushHistory("previous")
	// First enter history navigation mode
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, "previous", m.Value())

	// Now show completion while navigating history
	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "help", Value: "/help "},
	}, "", 0)
	assert.True(t, m.CompletionActive())

	// Up should hide completion and continue history navigation
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.False(t, m.CompletionActive())
	assert.Equal(t, "previous", m.Value())
}

func TestEditorCompletionHidesOnHistoryDown(t *testing.T) {
	m := NewEditorModel()
	m = m.PushHistory("first")
	m = m.PushHistory("second")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp}) // navigating=true, histIdx=1
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp}) // histIdx=2

	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "help", Value: "/help "},
	}, "", 0)
	assert.True(t, m.CompletionActive())

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.False(t, m.CompletionActive())
}

func TestEditorApplyCompletionSlash(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("/he")
	m = m.ShowCompletion(CompletionSlash, []CompletionItem{
		{Label: "help", Value: "/help "},
	}, "he", 0)

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, m.CompletionActive())
	assert.Empty(t, m.Value()) // submitted and cleared
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Equal(t, "/help", submit.Text)
}

func TestEditorApplyCompletionAtTrigger(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("text @sr")
	m = m.ShowCompletion(CompletionFile, []CompletionItem{
		{Label: "src/", Value: "src/"},
	}, "sr", 5) // @ is at byte offset 5

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, m.CompletionActive())
	assert.Empty(t, m.Value()) // submitted and cleared
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Equal(t, "text src/", submit.Text)
}

func TestEditorApplyCompletionSpaceTrigger(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("/help arg")
	m = m.ShowCompletion(CompletionFile, []CompletionItem{
		{Label: "argfile", Value: "argfile"},
	}, "arg", 6) // after the space at byte offset 6

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, m.CompletionActive())
	assert.Empty(t, m.Value()) // submitted and cleared
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Equal(t, "/help argfile", submit.Text)
}

func TestEditorCompletionNotVisibleKeysGoToTextarea(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("abc")

	// When completion is not visible, Tab is not intercepted
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	// Tab doesn't change value in textarea by default
	assert.Equal(t, "abc", m.Value())
}

func TestEditorShowCompletionFileKind(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("@fi")
	m = m.ShowCompletion(CompletionFile, []CompletionItem{
		{Label: "file.go", Value: "file.go"},
	}, "fi", 0)

	assert.True(t, m.CompletionActive())
	assert.Equal(t, CompletionFile, m.Completion().Kind())
	assert.Equal(t, 1, m.Completion().FilteredCount())
}

func TestEditorApplyCompletionPreservesTrailingText(t *testing.T) {
	m := NewEditorModel()
	// Type text so cursor is at end
	for _, ch := range "text @sr more" {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(ch), Code: ch})
	}
	// Move cursor left 5 times (before " more")
	for range 5 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	}

	m = m.ShowCompletion(CompletionFile, []CompletionItem{
		{Label: "src/", Value: "src/"},
	}, "sr", 5) // @ is at byte offset 5

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, m.CompletionActive())
	assert.Empty(t, m.Value()) // submitted and cleared
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Equal(t, "text src/ more", submit.Text)
}

func TestEditorApplyCompletionWithCorrectTriggerOffset(t *testing.T) {
	m := NewEditorModel()
	m = m.SetValue("prefix @sr")
	m = m.ShowCompletion(CompletionFile, []CompletionItem{
		{Label: "src/", Value: "src/"},
	}, "sr", 7) // @ is at byte offset 7

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, m.CompletionActive())
	assert.Empty(t, m.Value()) // submitted and cleared
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	require.True(t, ok)
	assert.Equal(t, "prefix src/", submit.Text)
}
