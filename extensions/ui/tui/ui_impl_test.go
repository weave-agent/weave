package tui

import (
	"testing"

	"weave/bus"
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSender records messages sent via Send.
type mockSender struct {
	msgs []tea.Msg
}

func (s *mockSender) Send(msg tea.Msg) {
	s.msgs = append(s.msgs, msg)
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

func TestTUIImpl_RegisterCommand(t *testing.T) {
	b := bus.New()
	defer b.Close()

	commands := NewCommandRegistry(b)
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

	commands := NewCommandRegistry(b)
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
	ui.enqueue(req) //nolint:errcheck // test

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
	m := newModel(nil, nil)
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
	require.NotNil(t, m.popup)
	assert.Equal(t, requestSelect, m.popup.kind)
	assert.Len(t, m.popup.items, 3)
}

func TestModel_HandlePopupPending_Confirm(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestConfirm,
		message: "Are you sure?",
		result:  make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	require.NotNil(t, m.popup)
	assert.Equal(t, requestConfirm, m.popup.kind)
}

func TestModel_HandlePopupPending_Input(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestInput,
		message: "Enter value:",
		result:  make(chan overlayResponse, 1),
	}

	m = activatePopup(m, ui, req)
	require.NotNil(t, m.popup)
	assert.Equal(t, requestInput, m.popup.kind)
}

func TestModel_HandlePopupPending_NilUI(t *testing.T) {
	m := newModel(nil, nil)
	m.ui = nil

	updated, cmd := m.handlePopupPending()
	assert.Nil(t, cmd)
	assert.Nil(t, updated.popup)
}

func TestModel_PopupView(t *testing.T) {
	m := newModel(nil, nil)
	m.width = 80
	m.height = 24

	// No popup → empty view
	assert.Equal(t, "", m.popupView())

	// With confirm popup
	m.popup = &popupState{
		kind: requestConfirm,
		confirm: overlays.NewConfirmModel("Sure?").SetSize(80, 24).Show(),
	}
	view := m.popupView()
	assert.Contains(t, view, "Sure?")

	// With input popup
	m.popup = &popupState{
		kind: requestInput,
		input: overlays.NewInputModel("Name:").SetSize(80, 24).Show(),
	}
	view = m.popupView()
	assert.Contains(t, view, "Name:")

	// With select popup
	m.popup = &popupState{
		kind: requestSelect,
		select_: overlays.NewSelectorModel("Pick", []overlays.SelectorItem{
			{Title: "A"}, {Title: "B"},
		}).SetSize(80, 24).Show(),
	}
	view = m.popupView()
	assert.Contains(t, view, "Pick")
}

func TestModel_PopupConfirmYes(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestConfirm,
		message: "Proceed?",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.NotNil(t, m.popup)

	updated, _ := m.Update(overlays.ConfirmResultMsg{Confirmed: true})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.True(t, resp.confirmed)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.Nil(t, m.popup)
}

func TestModel_PopupConfirmNo(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestConfirm,
		message: "Proceed?",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.NotNil(t, m.popup)

	updated, _ := m.Update(overlays.ConfirmResultMsg{Confirmed: false})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.False(t, resp.confirmed)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.Nil(t, m.popup)
}

func TestModel_PopupSelectCancel(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil)
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
	require.NotNil(t, m.popup)

	updated, _ := m.Update(overlays.SelectorCancelledMsg{})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.Equal(t, -1, resp.index)
		assert.Error(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.Nil(t, m.popup)
}

func TestModel_PopupSelectConfirm(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil)
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
	require.NotNil(t, m.popup)

	updated, _ := m.Update(overlays.SelectorSelectedMsg{Index: 1, Item: overlays.SelectorItem{Title: "b"}})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.Equal(t, 1, resp.index)
		assert.NoError(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.Nil(t, m.popup)
}

func TestModel_PopupInputSubmit(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestInput,
		message: "Name:",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.NotNil(t, m.popup)

	updated, _ := m.Update(overlays.InputResultMsg{Value: "hi", Ok: true})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.Equal(t, "hi", resp.value)
		assert.NoError(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.Nil(t, m.popup)
}

func TestModel_PopupInputCancel(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil)
	m.width = 80
	m.height = 24
	m.ui = ui

	req := &overlayRequest{
		kind:    requestInput,
		message: "Name:",
		result:  make(chan overlayResponse, 1),
	}
	m = activatePopup(m, ui, req)
	require.NotNil(t, m.popup)

	updated, _ := m.Update(overlays.InputResultMsg{Ok: false})
	m = updated.(Model)

	select {
	case resp := <-req.result:
		assert.Error(t, resp.err)
	default:
		t.Fatal("expected response on result channel")
	}

	assert.Nil(t, m.popup)
}

func TestModel_PopupSequentialQueuing(t *testing.T) {
	ui := NewTUIImpl(nil, nil)
	m := newModel(nil, nil)
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

	// First popup should be activated
	m, _ = m.handlePopupPending()
	require.NotNil(t, m.popup)
	assert.Equal(t, requestSelect, m.popup.kind)

	// Resolve first popup
	updated, _ := m.Update(overlays.SelectorSelectedMsg{Index: 0, Item: overlays.SelectorItem{Title: "a"}})
	m = updated.(Model)

	// Second should still be queued
	assert.True(t, ui.hasPendingPopups())

	m, _ = m.handlePopupPending()
	require.NotNil(t, m.popup)
	assert.Equal(t, requestConfirm, m.popup.kind)

	// Resolve second popup
	updated, _ = m.Update(overlays.ConfirmResultMsg{Confirmed: true})
	m = updated.(Model)

	assert.Nil(t, m.popup)
	assert.False(t, ui.hasPendingPopups())
}

func TestModel_ExtStatusMsgUpdatesFooter(t *testing.T) {
	m := newModel(nil, nil)
	m.width = 80
	m.height = 24

	updated, _ := m.Update(extStatusMsg{key: "test", text: "running"})
	m = updated.(Model)

	assert.Equal(t, "running", m.footer.ExtStatus()["test"])
}

func TestModel_NotifyMsgAddsToChat(t *testing.T) {
	m := newModel(nil, nil)
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
	m := newModel(nil, nil)
	assert.NotNil(t, m.ui)
}

func TestModel_ViewWithPopup(t *testing.T) {
	m := newModel(nil, nil)
	m.width = 80
	m.height = 24

	m.popup = &popupState{
		kind: requestConfirm,
		confirm: overlays.NewConfirmModel("Sure?").SetSize(80, 24).Show(),
	}

	view := m.View()
	assert.Contains(t, view, "Sure?")
}

// mockRenderer implements sdk.ToolRenderer for testing.
type mockRenderer struct{}

func (m *mockRenderer) Render(content string, width int) string {
	return content
}
