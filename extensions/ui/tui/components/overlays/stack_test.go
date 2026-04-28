package overlays

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDialog is a test Dialog implementation.
type mockDialog struct {
	id       string
	handles  func(tea.Msg) bool
	doneVal  bool
	result   DialogResult
	lastMsg  tea.Msg
	sizeW    int
	sizeH    int
	drawn    bool
	updateFn func(tea.Msg) (Dialog, tea.Cmd)
}

func (d *mockDialog) ID() string                            { return d.id }
func (d *mockDialog) Done() bool                            { return d.doneVal }
func (d *mockDialog) Result() DialogResult                  { return d.result }
func (d *mockDialog) SetSize(w, h int) Dialog               { d.sizeW = w; d.sizeH = h; return d }
func (d *mockDialog) Draw(scr uv.Screen, area uv.Rectangle) { d.drawn = true }

func (d *mockDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	d.lastMsg = msg
	if d.updateFn != nil {
		return d.updateFn(msg)
	}

	return d, nil
}

func (d *mockDialog) Handles(msg tea.Msg) bool {
	if d.handles != nil {
		return d.handles(msg)
	}

	return false
}

// --- DialogStack tests ---

func TestDialogStack_Empty(t *testing.T) {
	s := NewDialogStack()
	assert.True(t, s.Empty())
	assert.Equal(t, 0, s.Len())
	assert.Nil(t, s.Peek())
}

func TestDialogStack_PushPop(t *testing.T) {
	s := NewDialogStack()
	d1 := &mockDialog{id: "d1"}
	d2 := &mockDialog{id: "d2"}

	s = s.Push(d1)
	assert.False(t, s.Empty())
	assert.Equal(t, 1, s.Len())
	assert.Equal(t, "d1", s.Peek().ID())

	s = s.Push(d2)
	assert.Equal(t, 2, s.Len())
	assert.Equal(t, "d2", s.Peek().ID())

	// Pop returns top (LIFO)
	s, popped := s.Pop()
	assert.Equal(t, "d2", popped.ID())
	assert.Equal(t, 1, s.Len())
	assert.Equal(t, "d1", s.Peek().ID())

	// Pop remaining
	s, popped = s.Pop()
	assert.Equal(t, "d1", popped.ID())
	assert.True(t, s.Empty())
	assert.Nil(t, s.Peek())
}

func TestDialogStack_PopEmpty(t *testing.T) {
	s := NewDialogStack()
	s, d := s.Pop()
	assert.Nil(t, d)
	assert.True(t, s.Empty())
}

func TestDialogStack_Update_EmptyStack(t *testing.T) {
	s := NewDialogStack()
	newS, cmd, completed := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	assert.Equal(t, s, newS)
	assert.Nil(t, cmd)
	assert.Nil(t, completed)
}

func TestDialogStack_Update_RoutesToTop(t *testing.T) {
	s := NewDialogStack()

	bottom := &mockDialog{id: "bottom", handles: func(msg tea.Msg) bool {
		_, ok := msg.(tea.KeyMsg)
		return ok
	}}
	top := &mockDialog{id: "top", handles: func(msg tea.Msg) bool {
		_, ok := msg.(tea.KeyMsg)
		return ok
	}}

	s = s.Push(bottom)
	s = s.Push(top)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	_, _, _ = s.Update(msg)

	// Top dialog should have received the message
	assert.Equal(t, msg, top.lastMsg)
	// Bottom dialog should NOT have received it
	assert.Nil(t, bottom.lastMsg)
}

func TestDialogStack_Update_FallThrough(t *testing.T) {
	s := NewDialogStack()

	bottom := &mockDialog{id: "bottom", handles: func(msg tea.Msg) bool {
		return true // handles everything
	}}
	top := &mockDialog{id: "top", handles: func(msg tea.Msg) bool {
		// Only handles SelectorSelectedMsg, not KeyMsg
		_, ok := msg.(SelectorSelectedMsg)
		return ok
	}}

	s = s.Push(bottom)
	s = s.Push(top)

	// KeyMsg not handled by top → falls through to bottom
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	_, _, _ = s.Update(msg)

	assert.Equal(t, msg, bottom.lastMsg)
	assert.Nil(t, top.lastMsg) // top Handles returned false, Update not called
}

func TestDialogStack_Update_NoFallThroughForNonKeyMsg(t *testing.T) {
	s := NewDialogStack()

	bottom := &mockDialog{id: "bottom", handles: func(msg tea.Msg) bool {
		return true
	}}
	top := &mockDialog{id: "top", handles: func(msg tea.Msg) bool {
		return false // doesn't handle anything
	}}

	s = s.Push(bottom)
	s = s.Push(top)

	// Non-key message should NOT fall through, even if top doesn't handle it
	msg := SelectorSelectedMsg{Index: 0}
	_, _, _ = s.Update(msg)

	assert.Nil(t, bottom.lastMsg)
}

func TestDialogStack_Draw_AllDialogs(t *testing.T) {
	s := NewDialogStack()
	d1 := &mockDialog{id: "d1"}
	d2 := &mockDialog{id: "d2"}

	s = s.Push(d1)
	s = s.Push(d2)

	canvas := uv.NewScreenBuffer(80, 24)
	s.Draw(canvas, canvas.Bounds())

	assert.True(t, d1.drawn)
	assert.True(t, d2.drawn)
}

func TestDialogStack_Resize(t *testing.T) {
	s := NewDialogStack()
	d1 := &mockDialog{id: "d1"}
	d2 := &mockDialog{id: "d2"}

	s = s.Push(d1)
	s = s.Push(d2)

	_ = s.Resize(120, 40)

	assert.Equal(t, 120, d1.sizeW)
	assert.Equal(t, 40, d1.sizeH)
	assert.Equal(t, 120, d2.sizeW)
	assert.Equal(t, 40, d2.sizeH)
}

// --- SelectorDialog tests ---

func TestSelectorDialog_HandlesKeyAndResult(t *testing.T) {
	model := NewSelectorModel("Pick", []SelectorItem{{Title: "A"}, {Title: "B"}})
	model = model.SetSize(80, 24).Show()
	d := NewSelectorDialog("test-select", model)

	// Handles KeyMsg
	assert.True(t, d.Handles(tea.KeyMsg{Type: tea.KeyUp}))
	// Handles SelectorSelectedMsg
	assert.True(t, d.Handles(SelectorSelectedMsg{Index: 0}))
	// Handles SelectorCancelledMsg
	assert.True(t, d.Handles(SelectorCancelledMsg{}))
	// Does NOT handle random messages
	assert.False(t, d.Handles(tea.WindowSizeMsg{}))
}

func TestSelectorDialog_DoneOnSelect(t *testing.T) {
	model := NewSelectorModel("Pick", []SelectorItem{{Title: "A"}})
	model = model.SetSize(80, 24).Show()
	d := NewSelectorDialog("test-select", model)

	assert.False(t, d.Done())

	// Simulate selection
	newD, cmd := d.Update(SelectorSelectedMsg{Index: 0})
	d = newD.(*SelectorDialog)

	assert.True(t, d.Done())
	require.NoError(t, d.Result().Err)
	assert.Equal(t, 0, d.Result().Index)
	assert.Nil(t, cmd)
}

func TestSelectorDialog_DoneOnCancel(t *testing.T) {
	model := NewSelectorModel("Pick", []SelectorItem{{Title: "A"}})
	model = model.SetSize(80, 24).Show()
	d := NewSelectorDialog("test-select", model)

	newD, _ := d.Update(SelectorCancelledMsg{})
	d = newD.(*SelectorDialog)

	assert.True(t, d.Done())
	assert.Equal(t, -1, d.Result().Index)
	assert.Error(t, d.Result().Err)
}

func TestSelectorDialog_KeyEventUpdatesModel(t *testing.T) {
	model := NewSelectorModel("Pick", []SelectorItem{{Title: "A"}, {Title: "B"}})
	model = model.SetSize(80, 24).Show()
	d := NewSelectorDialog("test-select", model)

	// Press 'a' to add to filter
	newD, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	d = newD.(*SelectorDialog)

	assert.False(t, d.Done())
	assert.Equal(t, "a", d.Model().Filter())
}

func TestSelectorDialog_SetSize(t *testing.T) {
	model := NewSelectorModel("Pick", nil)
	d := NewSelectorDialog("test-select", model)

	newD := d.SetSize(120, 40)
	sd := newD.(*SelectorDialog)

	assert.Equal(t, 120, sd.Model().Width())
	assert.Equal(t, 40, sd.Model().Height())
}

// --- ConfirmDialog tests ---

func TestConfirmDialog_DoneOnConfirm(t *testing.T) {
	model := NewConfirmModel("Sure?")
	model = model.SetSize(80, 24).Show()
	d := NewConfirmDialog("test-confirm", model)

	assert.False(t, d.Done())

	newD, _ := d.Update(ConfirmResultMsg{Confirmed: true})
	d = newD.(*ConfirmDialog)

	assert.True(t, d.Done())
	assert.True(t, d.Result().Confirmed)
}

func TestConfirmDialog_DoneOnDeny(t *testing.T) {
	model := NewConfirmModel("Sure?")
	model = model.SetSize(80, 24).Show()
	d := NewConfirmDialog("test-confirm", model)

	newD, _ := d.Update(ConfirmResultMsg{Confirmed: false})
	d = newD.(*ConfirmDialog)

	assert.True(t, d.Done())
	assert.False(t, d.Result().Confirmed)
}

// --- InputDialog tests ---

func TestInputDialog_DoneOnSubmit(t *testing.T) {
	model := NewInputModel("Name:")
	model = model.SetSize(80, 24).Show()
	d := NewInputDialog("test-input", model)

	newD, _ := d.Update(InputResultMsg{Value: "hello", Ok: true})
	d = newD.(*InputDialog)

	assert.True(t, d.Done())
	assert.Equal(t, "hello", d.Result().Value)
	assert.NoError(t, d.Result().Err)
}

func TestInputDialog_DoneOnCancel(t *testing.T) {
	model := NewInputModel("Name:")
	model = model.SetSize(80, 24).Show()
	d := NewInputDialog("test-input", model)

	newD, _ := d.Update(InputResultMsg{Ok: false})
	d = newD.(*InputDialog)

	assert.True(t, d.Done())
	require.Error(t, d.Result().Err)
	assert.EqualError(t, d.Result().Err, "canceled")
}

// --- Integration: dialog stack with real SelectorDialog ---

func TestDialogStack_SelectorFlow(t *testing.T) {
	items := []SelectorItem{{Title: "A"}, {Title: "B"}}
	model := NewSelectorModel("Pick", items)
	model = model.SetSize(80, 24).Show()

	s := NewDialogStack()
	s = s.Push(NewSelectorDialog("test", model))

	// Simulate pressing Enter (selects first item)
	newS, cmd, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = newS

	require.NotNil(t, cmd)

	// Execute the command to get SelectorSelectedMsg
	msg := cmd()
	selMsg, ok := msg.(SelectorSelectedMsg)
	require.True(t, ok)
	assert.Equal(t, 0, selMsg.Index)

	// Feed the result back
	newS, _, _ = s.Update(selMsg)
	s = newS

	// Dialog should be done now
	top := s.Peek()
	require.NotNil(t, top)
	assert.True(t, top.Done())
	assert.Equal(t, 0, top.Result().Index)
}
