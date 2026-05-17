package tui

import (
	"sync"
	"testing"
	"time"

	"github.com/weave-agent/weave-tui/components/messages"
	"github.com/weave-agent/weave-tui/components/overlays"
	"github.com/weave-agent/weave-tui/palette"
	"github.com/weave-agent/weave/bus"
	"github.com/weave-agent/weave/sdk"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSender records messages sent via Send.
type mockSender struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (s *mockSender) Send(msg tea.Msg) {
	s.mu.Lock()
	s.msgs = append(s.msgs, msg)
	s.mu.Unlock()
}

func (s *mockSender) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.msgs)
}

func (s *mockSender) At(i int) tea.Msg {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.msgs[i]
}

func TestTUIImpl_SetStatus(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	ui.SetStatus("build", "compiling...")

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(extStatusMsg)
	require.True(t, ok)
	assert.Equal(t, "build", msg.key)
	assert.Equal(t, "compiling...", msg.text)
}

func TestTUIImpl_SetStatus_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// Should not panic
	ui.SetStatus("build", "compiling...")
}

func TestTUIImpl_Notify(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	ui.Notify("hello world")

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(notifyMsg)
	require.True(t, ok)
	assert.Equal(t, "hello world", msg.message)
}

func TestTUIImpl_Notify_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// Should not panic
	ui.Notify("hello world")
}

func TestTUIImpl_RegisterCommand_SendsSlashCommandsUpdated(t *testing.T) {
	b := bus.New()
	defer b.Close()

	commands := NewCommandRegistry(b, "")
	sender := &mockSender{}
	ui := NewTUIImpl(commands, nil)
	ui.SetProgram(sender)

	ui.RegisterCommand("/dynamic-cmd", func(_ string) error {
		return nil
	})

	// Should have sent a slashCommandsUpdatedMsg
	found := false

	for _, msg := range sender.msgs {
		if _, ok := msg.(slashCommandsUpdatedMsg); ok {
			found = true
		}
	}

	assert.True(t, found, "expected slashCommandsUpdatedMsg to be sent after RegisterCommand")
}

func TestTUIImpl_RegisterCommand_NoSendWithoutProgram(t *testing.T) {
	b := bus.New()
	defer b.Close()

	commands := NewCommandRegistry(b, "")
	ui := NewTUIImpl(commands, nil)
	// No program set — should not panic

	ui.RegisterCommand("/no-program-cmd", func(_ string) error {
		return nil
	})
}

func TestModel_SlashCommandsUpdatedMsg_RefreshesEditor(t *testing.T) {
	b := bus.New()
	defer b.Close()

	m := newModel(b, nil, nil, nil)
	m.width = 80
	m.height = 24

	// Count initial slash commands
	initialNames := m.commands.Names()
	initialCount := len(initialNames)

	// Register a new command on the model's registry
	m.commands.Register("/dynamic-test", "dynamic test command", false, func(_ string) CommandResult {
		return CommandResult{}
	})

	// Send the update message
	updated, _ := m.Update(slashCommandsUpdatedMsg{})
	m = updated.(Model)

	// Verify command list grew
	newNames := m.commands.Names()
	assert.Greater(t, len(newNames), initialCount, "expected more commands after registration")
	assert.Contains(t, newNames, "/dynamic-test")
}

func TestTUIImpl_RegisterCommand(t *testing.T) {
	b := bus.New()
	defer b.Close()

	commands := NewCommandRegistry(b, "")
	ui := NewTUIImpl(commands, nil)

	ui.RegisterCommand("/test-cmd", func(args string) error {
		assert.Equal(t, "arg1", args)
		return nil
	})

	// Command should be registered in the command registry
	_, ok := commands.Lookup("/test-cmd")
	require.True(t, ok)

	// Dispatch it
	handled, result := commands.Dispatch("/test-cmd arg1")
	assert.True(t, handled)
	assert.Contains(t, result.Notify, "/test-cmd: ok")
}

func TestTUIImpl_RegisterCommand_Error(t *testing.T) {
	b := bus.New()
	defer b.Close()

	commands := NewCommandRegistry(b, "")
	ui := NewTUIImpl(commands, nil)

	ui.RegisterCommand("/err-cmd", func(args string) error {
		return assert.AnError
	})

	handled, result := commands.Dispatch("/err-cmd")
	assert.True(t, handled)
	assert.Contains(t, result.Notify, "error:")
}

func TestTUIImpl_RegisterRenderer(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	renderer := &mockRenderer{}
	ui.RegisterRenderer("bash", renderer)

	got, ok := ui.GetRenderer("bash")
	assert.True(t, ok)
	assert.Equal(t, renderer, got)

	_, ok = ui.GetRenderer("nonexistent")
	assert.False(t, ok)
}

func TestTUIImpl_RegisterKeybinding(t *testing.T) {
	bindings := NewBindingRegistry()
	ui := NewTUIImpl(nil, bindings)

	ui.RegisterKeybinding(sdk.Keybinding{
		Name:        "custom.action",
		Keys:        []string{"ctrl+f"},
		Description: "Custom action",
	})

	action, ok := bindings.Resolve("ctrl+f")
	assert.True(t, ok)
	assert.Equal(t, BindingAction("custom.action"), action)
}

func TestTUIImpl_PopupQueue(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	assert.False(t, ui.hasPendingPopups())

	req := &overlayRequest{
		kind:   requestSelect,
		title:  "Pick one",
		items:  []string{"a", "b"},
		result: make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req))

	assert.True(t, ui.hasPendingPopups())

	dequeued := ui.dequeue()
	require.NotNil(t, dequeued)
	assert.Equal(t, requestSelect, dequeued.kind)
	assert.Equal(t, "Pick one", dequeued.title)

	assert.False(t, ui.hasPendingPopups())
	assert.Nil(t, ui.dequeue())
}

func TestTUIImpl_PopupQueueFIFO(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	req1 := &overlayRequest{kind: requestSelect, title: "first", result: make(chan overlayResponse, 1)}
	req2 := &overlayRequest{kind: requestConfirm, message: "second", result: make(chan overlayResponse, 1)}

	require.NoError(t, ui.enqueue(req1))
	require.NoError(t, ui.enqueue(req2))

	first := ui.dequeue()
	require.NotNil(t, first)
	assert.Equal(t, requestSelect, first.kind)

	second := ui.dequeue()
	require.NotNil(t, second)
	assert.Equal(t, requestConfirm, second.kind)
}

func TestTUIImpl_EnqueueSendsPopupPendingMsg(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	req := &overlayRequest{
		kind:   requestSelect,
		title:  "Pick",
		items:  []string{"a"},
		result: make(chan overlayResponse, 1),
	}
	ui.enqueue(req) //nolint:errcheck,gosec // test

	require.Len(t, sender.msgs, 1)
	_, ok := sender.msgs[0].(popupPendingMsg)
	assert.True(t, ok)
}

// activatePopup is a helper that enqueues a request, dequeues it via handlePopupPending,
// and returns the updated model.
func activatePopup(m Model, ui *TUIImpl, req *overlayRequest) Model {
	ui.SetProgram(&mockSender{})
	_ = ui.enqueue(req)
	updated, _ := m.handlePopupPending()

	return updated
}

func TestModel_HandlePopupPending_Select(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:   requestSelect,
		title:  "Choose",
		items:  []string{"opt1", "opt2", "opt3"},
		result: make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	assert.False(t, m.dialogStack.Empty())
	top := m.dialogStack.Peek()
	require.NotNil(t, top)
	_, ok := top.(*overlays.SelectorDialog)
	assert.True(t, ok, "expected SelectorDialog on stack")
}

func TestModel_HandlePopupPending_Confirm(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestConfirm,
		message: "Are you sure?",
		result:  make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	assert.False(t, m.dialogStack.Empty())
	top := m.dialogStack.Peek()
	require.NotNil(t, top)
	_, ok := top.(*overlays.ConfirmDialog)
	assert.True(t, ok, "expected ConfirmDialog on stack")
}

func TestModel_HandlePopupPending_Input(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestInput,
		message: "Enter value:",
		result:  make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	assert.False(t, m.dialogStack.Empty())
	top := m.dialogStack.Peek()
	require.NotNil(t, top)
	_, ok := top.(*overlays.InputDialog)
	assert.True(t, ok, "expected InputDialog on stack")
}

func TestModel_HandlePopupPending_InputWithMask(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestInput,
		message: "Password:",
		mask:    '*',
		result:  make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	assert.False(t, m.dialogStack.Empty())
	top := m.dialogStack.Peek()
	require.NotNil(t, top)
	dialog, ok := top.(*overlays.InputDialog)
	require.True(t, ok, "expected InputDialog on stack")
	assert.Equal(t, rune('*'), dialog.Model().Mask())
}

func TestModel_HandlePopupPending_NilUI(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.ui = nil

	updated, cmd := m.handlePopupPending()
	assert.Nil(t, cmd)
	assert.True(t, updated.dialogStack.Empty())
}

func TestModel_PopupView(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// No dialogs → no overlay in view
	assert.True(t, m.dialogStack.Empty())
	view := m.View()
	assert.NotContains(t, view.Content, "Sure?")

	// With confirm dialog on stack
	m.dialogStack = m.dialogStack.Push(overlays.NewConfirmDialog(
		"popup-confirm-1",
		overlays.NewConfirmModel("Sure?").SetSize(80, 24).Show(),
	))
	view = m.View()
	assert.Contains(t, view.Content, "Sure?")

	// With input dialog on stack
	m.dialogStack = overlays.NewDialogStack()
	m.dialogStack = m.dialogStack.Push(overlays.NewInputDialog(
		"popup-input-1",
		overlays.NewInputModel("Name:").SetSize(80, 24).Show(),
	))
	view = m.View()
	assert.Contains(t, view.Content, "Name:")

	// With select dialog on stack
	m.dialogStack = overlays.NewDialogStack()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(
		"popup-select-1",
		overlays.NewSelectorModel("Pick", []overlays.SelectorItem{
			{Title: "A"}, {Title: "B"},
		}).SetSize(80, 24).Show(),
	))
	view = m.View()
	assert.Contains(t, view.Content, "Pick")
}

func TestModel_PopupConfirmYes(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestConfirm,
		message: "Proceed?",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.ConfirmResultMsg{Confirmed: true})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.True(t, resp.confirmed)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_PopupConfirmNo(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestConfirm,
		message: "Proceed?",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.ConfirmResultMsg{Confirmed: false})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.False(t, resp.confirmed)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_PopupSelectCancel(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:   requestSelect,
		title:  "Pick",
		items:  []string{"a", "b"},
		result: make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.SelectorCancelledMsg{})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.Equal(t, -1, resp.index)
		require.Error(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_PopupSelectConfirm(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:   requestSelect,
		title:  "Pick",
		items:  []string{"a", "b", "c"},
		result: make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.SelectorSelectedMsg{Index: 1, Item: overlays.SelectorItem{Title: "b"}})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.Equal(t, 1, resp.index)
		require.NoError(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_PopupInputSubmit(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestInput,
		message: "Name:",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.InputResultMsg{Value: "hi", Ok: true})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.Equal(t, "hi", resp.value)
		require.NoError(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_PopupInputCancel(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestInput,
		message: "Name:",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.InputResultMsg{Ok: false})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		require.Error(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_HandlePopupPending_Editor(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestEditor,
		title:   "Edit note:",
		initial: "prefill",
		result:  make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	assert.False(t, m.dialogStack.Empty())
	top := m.dialogStack.Peek()
	require.NotNil(t, top)
	_, ok := top.(*overlays.EditorDialog)
	assert.True(t, ok, "expected EditorDialog on stack")
}

func TestModel_PopupEditorSubmit(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestEditor,
		title:   "Edit:",
		initial: "",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.EditorResultMsg{Value: "edited text", Ok: true})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.Equal(t, "edited text", resp.value)
		require.NoError(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_PopupEditorCancel(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestEditor,
		title:   "Edit:",
		initial: "",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.EditorResultMsg{Ok: false})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		require.Error(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_PopupView_Editor(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.dialogStack = overlays.NewDialogStack()
	m.dialogStack = m.dialogStack.Push(overlays.NewEditorDialog(
		"popup-editor-1",
		overlays.NewEditorModel("Edit note:", "").SetSize(80, 24).Show(),
	))
	view := m.View()
	assert.Contains(t, view.Content, "Edit note:")
	assert.Contains(t, view.Content, "Ctrl+Enter")
}

func TestModel_HandlePopupPending_MultiSelect(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:     requestMultiSelect,
		title:    "Pick fruits",
		items:    []string{"apple", "banana", "cherry"},
		defaults: []bool{true, false, true},
		result:   make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	assert.False(t, m.dialogStack.Empty())
	top := m.dialogStack.Peek()
	require.NotNil(t, top)
	_, ok := top.(*overlays.MultiSelectDialog)
	assert.True(t, ok, "expected MultiSelectDialog on stack")
}

func TestModel_PopupMultiSelectConfirm(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:     requestMultiSelect,
		title:    "Pick",
		items:    []string{"a", "b", "c"},
		defaults: []bool{false, true, false},
		result:   make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.MultiSelectResultMsg{Selected: []int{0, 2}, Ok: true})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.Equal(t, []int{0, 2}, resp.selected)
		require.NoError(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_PopupMultiSelectCancel(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:   requestMultiSelect,
		title:  "Pick",
		items:  []string{"a", "b"},
		result: make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.False(t, m.dialogStack.Empty())

	updated, _ := m.Update(overlays.MultiSelectResultMsg{Ok: false})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		require.Error(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_PopupView_MultiSelect(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.dialogStack = overlays.NewDialogStack()
	m.dialogStack = m.dialogStack.Push(overlays.NewMultiSelectDialog(
		"popup-multiselect-1",
		overlays.NewMultiSelectModel("Pick items:", []string{"A", "B", "C"}, nil).SetSize(80, 24).Show(),
	))
	view := m.View()
	assert.Contains(t, view.Content, "Pick items:")
	assert.Contains(t, view.Content, "☐")
}

func TestModel_PopupSequentialQueuing(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req1 := &overlayRequest{
		kind:   requestSelect,
		title:  "First",
		items:  []string{"a"},
		result: make(chan overlayResponse, 1),
	}
	req2 := &overlayRequest{
		kind:    requestConfirm,
		message: "Second?",
		result:  make(chan overlayResponse, 1),
	}

	ui.SetProgram(&mockSender{})
	require.NoError(t, ui.enqueue(req1))
	require.NoError(t, ui.enqueue(req2))

	// First popup should be activated on dialog stack
	m, _ = m.handlePopupPending()
	require.False(t, m.dialogStack.Empty())

	// Resolve first popup
	updated, _ := m.Update(overlays.SelectorSelectedMsg{Index: 0, Item: overlays.SelectorItem{Title: "a"}})
	m = updated.(Model)

	// Second should still be queued
	assert.True(t, ui.hasPendingPopups())

	m, _ = m.handlePopupPending()
	require.False(t, m.dialogStack.Empty())

	// Resolve second popup
	updated, _ = m.Update(overlays.ConfirmResultMsg{Confirmed: true})
	m = updated.(Model)

	assert.True(t, m.dialogStack.Empty())
	assert.False(t, ui.hasPendingPopups())
}

func TestTUIImpl_NotifyTyped(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	ui.NotifyTyped("warning text", sdk.NotifyWarning)

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(notifyTypedMsg)
	require.True(t, ok)
	assert.Equal(t, "warning text", msg.message)
	assert.Equal(t, sdk.NotifyWarning, msg.level)
}

func TestTUIImpl_NotifyTyped_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// Should not panic
	ui.NotifyTyped("test", sdk.NotifyInfo)
}

func TestTUIImpl_ShowError(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	ui.ShowError("something went wrong")

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(notifyTypedMsg)
	require.True(t, ok)
	assert.Equal(t, "something went wrong", msg.message)
	assert.Equal(t, sdk.NotifyError, msg.level)
}

func TestTUIImpl_SetWorking(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	ui.SetWorking("compiling...")

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(extStatusMsg)
	require.True(t, ok)
	assert.Equal(t, "working", msg.key)
	assert.Equal(t, "compiling...", msg.text)
}

func TestTUIImpl_ClearWorking(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	ui.ClearWorking()

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(extStatusMsg)
	require.True(t, ok)
	assert.Equal(t, "working", msg.key)
	assert.Empty(t, msg.text)
}

func TestModel_NotifyTypedMsgAddsNotificationToChat(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	updated, _ := m.Update(notifyTypedMsg{message: "error occurred", level: sdk.NotifyError})
	m = updated.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	nm, ok := items[0].(*messages.NotificationMessage)
	require.True(t, ok)
	assert.Equal(t, "error occurred", nm.Content())
	assert.Equal(t, sdk.NotifyError, nm.Level())
	assert.False(t, m.showLanding)
}

func TestModel_NotifyTypedMsg_InfoLevel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	updated, _ := m.Update(notifyTypedMsg{message: "info msg", level: sdk.NotifyInfo})
	m = updated.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	nm, ok := items[0].(*messages.NotificationMessage)
	require.True(t, ok)
	assert.Equal(t, sdk.NotifyInfo, nm.Level())
}

func TestModel_NotifyTypedMsg_WarningLevel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	updated, _ := m.Update(notifyTypedMsg{message: "warning msg", level: sdk.NotifyWarning})
	m = updated.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	nm, ok := items[0].(*messages.NotificationMessage)
	require.True(t, ok)
	assert.Equal(t, sdk.NotifyWarning, nm.Level())
}

func TestModel_ExtStatusMsgUpdatesFooter(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	updated, _ := m.Update(extStatusMsg{key: "test", text: "running"})
	m = updated.(Model)

	assert.Equal(t, "running", m.footer.ExtStatus()["test"])
}

func TestModel_NotifyMsgAddsToChat(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	updated, _ := m.Update(notifyMsg{message: "notification text"})
	m = updated.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Equal(t, "notification text", am.Content())
}

func TestNewNotifyAssistantMsg(t *testing.T) {
	am := newNotifyAssistantMsg("test message")
	assert.Equal(t, "test message", am.Content())
	assert.False(t, am.IsStreaming())
}

func TestModel_UIFieldSet(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	assert.NotNil(t, m.ui)
}

func TestModel_ViewWithPopup(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.dialogStack = m.dialogStack.Push(overlays.NewConfirmDialog(
		"popup-confirm-1",
		overlays.NewConfirmModel("Sure?").SetSize(80, 24).Show(),
	))

	view := m.View()
	assert.Contains(t, view.Content, "Sure?")
}

// mockRenderer implements sdk.ToolRenderer for testing.
type mockRenderer struct{}

func (m *mockRenderer) Render(content string, width int) string {
	return content
}

func TestModel_OutdatedNotificationAddsBanner(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	msg := OutdatedNotificationMsg{
		Extensions: []sdk.OutdatedInfo{
			{Name: "mcp", LocalHead: "abc", RemoteHead: "def"},
			{Name: "diff-viewer", LocalHead: "111", RemoteHead: "222"},
		},
	}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Extension Updates Available")
	assert.Contains(t, am.Content(), "mcp, diff-viewer")
	assert.Contains(t, am.Content(), "weave update")
	assert.False(t, m.showLanding)
}

func TestModel_OutdatedNotificationEmptyList(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	msg := OutdatedNotificationMsg{Extensions: nil}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	assert.Empty(t, m.chat.Items())
}

func TestModel_OutdatedNotificationSingleExtension(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	msg := OutdatedNotificationMsg{
		Extensions: []sdk.OutdatedInfo{
			{Name: "mcp", LocalHead: "abc", RemoteHead: "def"},
		},
	}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "mcp")
}

func TestFormatOutdatedBanner(t *testing.T) {
	banner := formatOutdatedBanner([]string{"mcp", "diff-viewer"})
	assert.Contains(t, banner, "Extension Updates Available")
	assert.Contains(t, banner, "mcp, diff-viewer")
	assert.Contains(t, banner, "weave update")
	assert.Contains(t, banner, "weave update <name>")
}

func TestFormatOutdatedBanner_Single(t *testing.T) {
	banner := formatOutdatedBanner([]string{"mcp"})
	assert.Contains(t, banner, "mcp has a newer version available.")
}

// --- Task 6: Theme support tests ---

func TestTUIImpl_SetTheme_SwitchesActiveTheme(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	// Register a custom theme
	err := ui.RegisterTheme("custom", ThemeDef{Accent: "123", Foreground: "255"})
	require.NoError(t, err)

	// Switch to it
	err = ui.SetTheme("custom")
	require.NoError(t, err)

	// Should have sent a themeChangedMsg
	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(themeChangedMsg)
	require.True(t, ok)
	assert.Equal(t, "123", msg.theme.Accent)
	assert.Equal(t, "255", msg.theme.Foreground)
}

func TestTUIImpl_SetTheme_UnknownTheme(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	err := ui.SetTheme("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown theme")
}

func TestTUIImpl_SetTheme_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// No program set

	err := ui.RegisterTheme("dark", ThemeDef{Accent: "60"})
	require.NoError(t, err)

	// Should not panic and should succeed
	err = ui.SetTheme("dark")
	require.NoError(t, err)
}

func TestTUIImpl_ListThemes(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	// Default theme should be present
	names := ui.ListThemes()
	assert.Contains(t, names, "default")

	// Register more themes
	err := ui.RegisterTheme("dark", ThemeDef{})
	require.NoError(t, err)
	err = ui.RegisterTheme("light", ThemeDef{})
	require.NoError(t, err)

	names = ui.ListThemes()
	assert.Len(t, names, 3)
	assert.Contains(t, names, "default")
	assert.Contains(t, names, "dark")
	assert.Contains(t, names, "light")

	// Should be sorted
	assert.Equal(t, []string{"dark", "default", "light"}, names)
}

func TestTUIImpl_RegisterTheme(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	err := ui.RegisterTheme("ocean", ThemeDef{
		Accent:     "33",
		Foreground: "15",
	})
	require.NoError(t, err)

	// Should be listable
	names := ui.ListThemes()
	assert.Contains(t, names, "ocean")

	// Should be settable
	err = ui.SetTheme("ocean")
	require.NoError(t, err)

	info := ui.Theme()
	assert.Equal(t, "ocean", info.Name)
	assert.Equal(t, "33", info.Accent)
	assert.Equal(t, "15", info.Foreground)
}

func TestTUIImpl_SetOrder_SendsPanelChangedMsg(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	ui.ShowPanel(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	ui.ShowPanel(PanelConfig{ID: "p2"}, &mockPanelDrawer{})

	sender.msgs = nil

	ui.SetOrder([]string{"p2", "p1"})

	require.Len(t, sender.msgs, 1)
	_, ok := sender.msgs[0].(panelChangedMsg)
	assert.True(t, ok)
	assert.Equal(t, []string{"p2", "p1"}, ui.GetOrder())
}

func TestTUIImpl_SetOrder_PreservesOmittedPanels(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	ui.ShowPanel(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	ui.ShowPanel(PanelConfig{ID: "p2"}, &mockPanelDrawer{})
	ui.ShowPanel(PanelConfig{ID: "p3"}, &mockPanelDrawer{})

	ui.SetOrder([]string{"p3", "missing"})

	assert.Equal(t, []string{"p3", "p1", "p2"}, ui.GetOrder())
	assert.Equal(t, []string{"p3", "p1", "p2"}, ui.panelManager.VisiblePanels())
}

func TestTUIImpl_RegisterTheme_EmptyName(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	err := ui.RegisterTheme("", ThemeDef{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestTUIImpl_RegisterTheme_ReservedDefault(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	err := ui.RegisterTheme("default", ThemeDef{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default")
}

func TestTUIImpl_Theme_ReturnsActiveTheme(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	info := ui.Theme()
	assert.Equal(t, "default", info.Name)
	assert.NotEmpty(t, info.Accent)
}

func TestTUIImpl_Theme_AfterSwitch(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	err := ui.RegisterTheme("red", ThemeDef{Accent: "196", Error: "196"})
	require.NoError(t, err)

	_ = ui.SetTheme("red")

	info := ui.Theme()
	assert.Equal(t, "red", info.Name)
	assert.Equal(t, "196", info.Accent)
	assert.Equal(t, "196", info.Error)
}

// --- Task 5: WithKeepContent docked overlay tests ---

func TestTUIImpl_Select_WithKeepContent(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	// Enqueue with KeepContent option
	req := &overlayRequest{
		kind:        requestSelect,
		title:       "Pick",
		items:       []string{"a", "b"},
		keepContent: true,
		result:      make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req))

	dequeued := ui.dequeue()
	require.NotNil(t, dequeued)
	assert.True(t, dequeued.keepContent, "keepContent should flow through to request")
}

func TestTUIImpl_Confirm_WithKeepContent(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	req := &overlayRequest{
		kind:        requestConfirm,
		message:     "Sure?",
		keepContent: true,
		result:      make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req))

	dequeued := ui.dequeue()
	require.NotNil(t, dequeued)
	assert.True(t, dequeued.keepContent)
}

func TestTUIImpl_Input_WithKeepContent(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	req := &overlayRequest{
		kind:        requestInput,
		message:     "Name:",
		keepContent: true,
		result:      make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req))

	dequeued := ui.dequeue()
	require.NotNil(t, dequeued)
	assert.True(t, dequeued.keepContent)
}

func TestTUIImpl_Input_WithMask(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	req := &overlayRequest{
		kind:    requestInput,
		message: "Password:",
		mask:    '*',
		result:  make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req))

	dequeued := ui.dequeue()
	require.NotNil(t, dequeued)
	assert.Equal(t, rune('*'), dequeued.mask)
}

func TestTUIImpl_Editor_WithKeepContent(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	req := &overlayRequest{
		kind:        requestEditor,
		title:       "Edit:",
		keepContent: true,
		result:      make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req))

	dequeued := ui.dequeue()
	require.NotNil(t, dequeued)
	assert.True(t, dequeued.keepContent)
}

func TestTUIImpl_MultiSelect_WithKeepContent(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	req := &overlayRequest{
		kind:        requestMultiSelect,
		title:       "Pick",
		items:       []string{"a", "b"},
		keepContent: true,
		result:      make(chan overlayResponse, 1),
	}
	require.NoError(t, ui.enqueue(req))

	dequeued := ui.dequeue()
	require.NotNil(t, dequeued)
	assert.True(t, dequeued.keepContent)
}

func TestModel_DockedOverlay_SetsFlag(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:        requestSelect,
		title:       "Choose",
		items:       []string{"opt1", "opt2"},
		keepContent: true,
		result:      make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	assert.True(t, m.dockedOverlay, "dockedOverlay should be set for keepContent popup")
}

func TestModel_DockedOverlay_ClearsOnDone(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:        requestSelect,
		title:       "Choose",
		items:       []string{"opt1", "opt2"},
		keepContent: true,
		result:      make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	require.True(t, m.dockedOverlay)

	// Resolve the popup
	updated, _ := m.Update(overlays.SelectorSelectedMsg{Index: 0, Item: overlays.SelectorItem{Title: "opt1"}})
	m = updated.(Model)

	assert.False(t, m.dockedOverlay, "dockedOverlay should be cleared when dialog completes")
}

func TestModel_DockedOverlay_ClearsOnCancel(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:        requestConfirm,
		message:     "Sure?",
		keepContent: true,
		result:      make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	require.True(t, m.dockedOverlay)

	// Cancel via ctrl+c
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = updated.(Model)

	assert.False(t, m.dockedOverlay, "dockedOverlay should be cleared on force cancel")
}

func TestModel_DockedOverlay_ChatStillVisible(t *testing.T) {
	m := newModelNoLanding()
	m.width = 80
	m.height = 30
	m.chat = m.chat.SetSize(80, m.chatHeight(30))

	m.AddUserMessage("hello world")

	// Activate a docked confirm dialog
	m.dockedOverlay = true
	m.dialogStack = m.dialogStack.Push(overlays.NewConfirmDialog(
		"popup-confirm-1",
		overlays.NewConfirmModel("Sure?").SetSize(80, dockedOverlayHeight).Show(),
	))

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()

	// Dialog should be visible
	assert.Contains(t, rendered, "Sure?")
	// Chat content should still be visible (not dimmed out)
	assert.Contains(t, rendered, "hello world")
}

func TestModel_DockedOverlay_NoBackdropDimming(t *testing.T) {
	m := newModelNoLanding()
	m.width = 40
	m.height = 20
	m.chat = m.chat.SetSize(40, m.chatHeight(20))

	m.AddUserMessage("test")

	// First, render without any dialog to establish baseline
	canvasNoDialog := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvasNoDialog, canvasNoDialog.Bounds())

	// Now render with docked dialog
	m.dockedOverlay = true
	m.dialogStack = m.dialogStack.Push(overlays.NewConfirmDialog(
		"popup-confirm-1",
		overlays.NewConfirmModel("Sure?").SetSize(40, dockedOverlayHeight).Show(),
	))

	canvasDocked := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvasDocked, canvasDocked.Bounds())

	// The chat area (top portion, above the docked dialog) should not have
	// been dimmed. Compare a cell in the chat area between the two renders.
	// With backdrop dimming, the foreground would change to muted.
	// Without it, the cell should be identical.
	mutedColor := lipgloss.Color(palette.DefaultTheme().Muted)
	foundDimmed := false

	// Check the top half of the screen (chat area, above docked region)
	chatAreaEnd := m.height - dockedOverlayHeight - 2 // above docked + footer
	for y := range chatAreaEnd {
		for x := range 40 {
			cellNoDialog := canvasNoDialog.CellAt(x, y)
			cellDocked := canvasDocked.CellAt(x, y)

			if cellNoDialog == nil || cellDocked == nil || cellNoDialog.IsZero() {
				continue
			}

			// If the cell was changed to muted color by the docked dialog render,
			// that's dimming
			if cellDocked.Style.Fg == mutedColor && cellNoDialog.Style.Fg != mutedColor {
				foundDimmed = true
				break
			}
		}

		if foundDimmed {
			break
		}
	}

	assert.False(t, foundDimmed, "docked overlay should not apply backdrop dimming to chat area")
}

func TestModel_DockedOverlay_DialogSizedForDockedArea(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:        requestSelect,
		title:       "Choose",
		items:       []string{"a", "b", "c"},
		keepContent: true,
		result:      make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	top := m.dialogStack.Peek()
	require.NotNil(t, top)

	sd, ok := top.(*overlays.SelectorDialog)
	require.True(t, ok)
	// Dialog should be sized to docked height, not full terminal height
	assert.Equal(t, 80, sd.Model().Width())
	assert.Equal(t, dockedOverlayHeight, sd.Model().Height())
}

// --- Task 9: TUIExtAPI method tests ---

func TestTUIImpl_EditorText(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	// EditorText blocks waiting for a response; run it in a goroutine
	textCh := make(chan string, 1)
	go func() {
		textCh <- ui.EditorText()
	}()

	// Wait for the goroutine to call p.Send and block on the response
	require.Eventually(t, func() bool {
		return sender.Len() == 1
	}, time.Second, 10*time.Millisecond)

	msg, ok := sender.At(0).(editorTextRequestMsg)
	require.True(t, ok)

	// Send response to unblock
	msg.response <- "editor contents"

	select {
	case text := <-textCh:
		assert.Equal(t, "editor contents", text)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for EditorText")
	}
}

func TestTUIImpl_EditorText_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// Should not panic and return empty
	assert.Empty(t, ui.EditorText())
}

func TestTUIImpl_SetEditorText(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	ui.SetEditorText("hello world")

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(setEditorTextMsg)
	require.True(t, ok)
	assert.Equal(t, "hello world", msg.text)
}

func TestTUIImpl_SetEditorText_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// Should not panic
	ui.SetEditorText("test")
}

func TestTUIImpl_PasteToEditor(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	ui.PasteToEditor("pasted text")

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(pasteToEditorMsg)
	require.True(t, ok)
	assert.Equal(t, "pasted text", msg.text)
}

func TestTUIImpl_PasteToEditor_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// Should not panic
	ui.PasteToEditor("test")
}

func TestTUIImpl_RegisterRichRenderer(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	renderer := &mockRichRenderer{}
	ui.RegisterRichRenderer("custom-tool", renderer)

	got, ok := ui.GetRichRenderer("custom-tool")
	assert.True(t, ok)
	assert.Equal(t, renderer, got)

	_, ok = ui.GetRichRenderer("nonexistent")
	assert.False(t, ok)
}

func TestTUIImpl_RegisterMessageRenderer(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	renderer := &mockMessageRenderer{}
	ui.RegisterMessageRenderer("jira-ticket", renderer)

	got, ok := ui.GetMessageRenderer("jira-ticket")
	assert.True(t, ok)
	assert.Equal(t, renderer, got)

	_, ok = ui.GetMessageRenderer("nonexistent")
	assert.False(t, ok)
}

func TestTUIImpl_SetFooter(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	comp := &mockTUIComponent{}
	ui.SetFooter(comp)

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(setFooterMsg)
	require.True(t, ok)
	assert.Equal(t, comp, msg.component)
}

func TestTUIImpl_SetFooter_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// Should not panic
	ui.SetFooter(&mockTUIComponent{})
}

func TestTUIImpl_SetHeader(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	comp := &mockTUIComponent{}
	ui.SetHeader(comp)

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(setHeaderMsg)
	require.True(t, ok)
	assert.Equal(t, comp, msg.component)
}

func TestTUIImpl_SetHeader_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// Should not panic
	ui.SetHeader(&mockTUIComponent{})
}

func TestTUIImpl_OnTerminalInput(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	var received KeyEvent

	ui.OnTerminalInput(func(ev KeyEvent) {
		received = ev
	})

	require.Len(t, ui.inputHandlers, 1)
	ui.inputHandlers[0](KeyEvent{Code: 'a', Mod: 0, String: "a"})
	assert.Equal(t, 'a', received.Code)
}

func TestTUIImpl_AddAutocomplete(t *testing.T) {
	ui := NewTUIImpl(nil, nil)

	provider := &mockAutocompleteProvider{}
	ui.AddAutocomplete(provider)

	require.Len(t, ui.autocompleteProviders, 1)
	assert.Equal(t, provider, ui.autocompleteProviders[0])
}

func TestTUIImpl_SetWorkingFrames(t *testing.T) {
	sender := &mockSender{}
	ui := NewTUIImpl(nil, nil)
	ui.SetProgram(sender)

	frames := []string{"|", "/", "-", "\\"}
	ui.SetWorkingFrames(frames, 100*time.Millisecond)

	assert.Equal(t, frames, ui.workingFrames)
	assert.Equal(t, 100*time.Millisecond, ui.workingInterval)

	require.Len(t, sender.msgs, 1)
	msg, ok := sender.msgs[0].(setWorkingFramesMsg)
	require.True(t, ok)
	assert.Equal(t, frames, msg.frames)
	assert.Equal(t, 100*time.Millisecond, msg.interval)
}

func TestTUIImpl_SetWorkingFrames_NoProgram(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	// Should not panic
	ui.SetWorkingFrames([]string{"|"}, 100*time.Millisecond)
}

// Mock types for Task 9 tests

type mockRichRenderer struct{}

func (m *mockRichRenderer) Render(content string, theme sdk.ThemeInfo, width int) string {
	return "rich:" + content
}

type mockMessageRenderer struct{}

func (m *mockMessageRenderer) Render(content string, theme sdk.ThemeInfo, width int) string {
	return "msg:" + content
}

type mockTUIComponent struct{}

func (m *mockTUIComponent) Draw(_ uv.Screen, _ uv.Rectangle) {}

type mockAutocompleteProvider struct{}

func (m *mockAutocompleteProvider) Name() string { return "mock" }

func (m *mockAutocompleteProvider) Suggestions(_ AutocompleteContext) []AutocompleteSuggestion {
	return nil
}
