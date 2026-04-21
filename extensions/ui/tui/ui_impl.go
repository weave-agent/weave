package tui

import (
	"fmt"
	"sync"

	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
)

// overlayRequest is an internal message sent to the Bubble Tea program
// to trigger an overlay (Select, Confirm, or Input).
type overlayRequest struct {
	kind    overlayRequestKind
	title   string
	message string
	items   []string
	result  chan overlayResponse
}

type overlayRequestKind int

const (
	requestSelect overlayRequestKind = iota
	requestConfirm
	requestInput
)

// overlayResponse carries the result back to the blocking caller.
type overlayResponse struct {
	index     int
	value     string
	confirmed bool
	err       error
}

// TUIImpl implements sdk.UI by delegating to the TUI's internal registries
// and overlay components.
type TUIImpl struct {
	program   Sender
	commands  *CommandRegistry
	bindings  *BindingRegistry
	renderers map[string]sdk.ToolRenderer

	mu     sync.Mutex
	popupQ []*overlayRequest
	active bool
}

// NewTUIImpl creates a UI implementation backed by the given registries.
// The program is set later via SetProgram once the tea.Program is running.
func NewTUIImpl(commands *CommandRegistry, bindings *BindingRegistry) *TUIImpl {
	return &TUIImpl{
		commands:  commands,
		bindings:  bindings,
		renderers: make(map[string]sdk.ToolRenderer),
	}
}

// SetProgram sets the Bubble Tea program for sending overlay requests.
func (u *TUIImpl) SetProgram(p Sender) {
	u.program = p
}

// Select shows a selection overlay and blocks until the user picks an item or cancels.
func (u *TUIImpl) Select(title string, items []string) (int, error) {
	req := &overlayRequest{
		kind:   requestSelect,
		title:  title,
		items:  items,
		result: make(chan overlayResponse, 1),
	}
	if err := u.enqueue(req); err != nil {
		return -1, err
	}
	resp := <-req.result
	return resp.index, resp.err
}

// Confirm shows a yes/no dialog and blocks until the user responds.
func (u *TUIImpl) Confirm(message string) (bool, error) {
	req := &overlayRequest{
		kind:    requestConfirm,
		message: message,
		result:  make(chan overlayResponse, 1),
	}
	if err := u.enqueue(req); err != nil {
		return false, err
	}
	resp := <-req.result
	return resp.confirmed, resp.err
}

// Input shows a single-line input modal and blocks until the user submits or cancels.
func (u *TUIImpl) Input(prompt string) (string, error) {
	req := &overlayRequest{
		kind:    requestInput,
		message: prompt,
		result:  make(chan overlayResponse, 1),
	}
	if err := u.enqueue(req); err != nil {
		return "", err
	}
	resp := <-req.result
	return resp.value, resp.err
}

// SetStatus updates the footer's extension status area.
func (u *TUIImpl) SetStatus(key, text string) {
	// SetStatus is a fire-and-forget operation — it sends a message
	// to the program to update footer state.
	if u.program != nil {
		u.program.Send(extStatusMsg{key: key, text: text})
	}
}

// Notify shows a temporary notification in the chat area.
func (u *TUIImpl) Notify(message string) {
	if u.program != nil {
		u.program.Send(notifyMsg{message: message})
	}
}

// RegisterCommand adds a command to the slash command registry.
func (u *TUIImpl) RegisterCommand(name string, handler func(args string) error) {
	u.commands.Register(name, "", func(args string) CommandResult {
		err := handler(args)
		msg := "ok"
		if err != nil {
			msg = fmt.Sprintf("error: %v", err)
		}
		return CommandResult{Notify: "/" + name + ": " + msg}
	})
}

// RegisterRenderer stores a tool renderer for use by tool panels.
func (u *TUIImpl) RegisterRenderer(toolName string, renderer sdk.ToolRenderer) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.renderers[toolName] = renderer
}

// RegisterKeybinding delegates to the binding registry.
func (u *TUIImpl) RegisterKeybinding(kb sdk.Keybinding) {
	u.bindings.Register(BindingAction(kb.Name), kb.Keys, kb.Description)
}

// GetRenderer returns a registered tool renderer, if any.
func (u *TUIImpl) GetRenderer(toolName string) (sdk.ToolRenderer, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	r, ok := u.renderers[toolName]
	return r, ok
}

// enqueue adds a request to the popup queue and notifies the program.
// Returns an error if the program is not running.
func (u *TUIImpl) enqueue(req *overlayRequest) error {
	u.mu.Lock()
	u.popupQ = append(u.popupQ, req)
	u.mu.Unlock()

	if u.program == nil {
		return fmt.Errorf("tui not running")
	}

	u.program.Send(popupPendingMsg{})
	return nil
}

// dequeue removes and returns the next popup request, or nil if empty.
func (u *TUIImpl) dequeue() *overlayRequest {
	u.mu.Lock()
	defer u.mu.Unlock()

	if len(u.popupQ) == 0 {
		return nil
	}

	req := u.popupQ[0]
	u.popupQ = u.popupQ[1:]
	return req
}

// hasPendingPopups returns true if there are queued popup requests.
func (u *TUIImpl) hasPendingPopups() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.popupQ) > 0
}

// extStatusMsg is an internal tea.Msg to update footer extension status.
type extStatusMsg struct {
	key   string
	text  string
}

// notifyMsg is an internal tea.Msg to show a notification in chat.
type notifyMsg struct {
	message string
}

// popupPendingMsg signals that a popup request is queued.
type popupPendingMsg struct{}

// selectResultMsg carries the result of a Select overlay back to the UI impl.
type selectResultMsg struct {
	index int
	err   error
}

// confirmResultMsg carries the result of a Confirm overlay back to the UI impl.
type confirmResultMsg struct {
	confirmed bool
}

// inputResultMsg carries the result of an Input overlay back to the UI impl.
type inputResultMsg struct {
	value string
	ok    bool
}

// popupState tracks the active cross-extension popup and its response channel.
type popupState struct {
	kind    overlayRequestKind
	confirm overlays.ConfirmModel
	input   overlays.InputModel
	select_ overlays.SelectorModel
	items   []string
	result  chan overlayResponse
}

// handlePopupPending processes queued popup requests.
// Returns a tea.Cmd if an overlay was activated.
func (m Model) handlePopupPending() (Model, tea.Cmd) {
	if m.ui == nil {
		return m, nil
	}

	req := m.ui.dequeue()
	if req == nil {
		return m, nil
	}

	m.popup = &popupState{
		kind:   req.kind,
		result: req.result,
	}

	switch req.kind {
	case requestSelect:
		items := make([]overlays.SelectorItem, len(req.items))
		for i, title := range req.items {
			items[i] = overlays.SelectorItem{Title: title}
		}
		m.popup.select_ = overlays.NewSelectorModel(req.title, items)
		m.popup.select_ = m.popup.select_.SetSize(m.width, m.height)
		m.popup.select_ = m.popup.select_.Show()
		m.popup.items = req.items

	case requestConfirm:
		m.popup.confirm = overlays.NewConfirmModel(req.message)
		m.popup.confirm = m.popup.confirm.SetSize(m.width, m.height)
		m.popup.confirm = m.popup.confirm.Show()

	case requestInput:
		m.popup.input = overlays.NewInputModel(req.message)
		m.popup.input = m.popup.input.SetSize(m.width, m.height)
		m.popup.input = m.popup.input.Show()
	}

	return m, nil
}

// handlePopupUpdate routes messages to the active popup overlay.
func (m Model) handlePopupUpdate(msg tea.Msg) (Model, tea.Cmd) {
	if m.popup == nil {
		return m, nil
	}

	switch msg := msg.(type) {
	case overlays.SelectorSelectedMsg:
		if m.popup.kind == requestSelect {
			idx := msg.Index
			m.popup.result <- overlayResponse{index: idx}
			m.popup = nil
			return m, checkNextPopupCmd(m.ui)
		}

	case overlays.SelectorCancelledMsg:
		if m.popup.kind == requestSelect {
			m.popup.result <- overlayResponse{index: -1, err: fmt.Errorf("cancelled")}
			m.popup = nil
			return m, checkNextPopupCmd(m.ui)
		}

	case overlays.ConfirmResultMsg:
		if m.popup.kind == requestConfirm {
			m.popup.result <- overlayResponse{confirmed: msg.Confirmed}
			m.popup = nil
			return m, checkNextPopupCmd(m.ui)
		}

	case overlays.InputResultMsg:
		if m.popup.kind == requestInput {
			if msg.Ok {
				m.popup.result <- overlayResponse{value: msg.Value}
			} else {
				m.popup.result <- overlayResponse{err: fmt.Errorf("cancelled")}
			}
			m.popup = nil
			return m, checkNextPopupCmd(m.ui)
		}

	case tea.KeyMsg:
		switch m.popup.kind {
		case requestSelect:
			var cmd tea.Cmd
			m.popup.select_, cmd = m.popup.select_.Update(msg)
			return m, cmd
		case requestConfirm:
			var cmd tea.Cmd
			m.popup.confirm, cmd = m.popup.confirm.Update(msg)
			return m, cmd
		case requestInput:
			var cmd tea.Cmd
			m.popup.input, cmd = m.popup.input.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// popupView returns the view for the active popup overlay.
func (m Model) popupView() string {
	if m.popup == nil {
		return ""
	}

	switch m.popup.kind {
	case requestSelect:
		return m.popup.select_.View()
	case requestConfirm:
		return m.popup.confirm.View()
	case requestInput:
		return m.popup.input.View()
	}

	return ""
}

// checkNextPopupCmd returns a tea.Cmd that sends popupPendingMsg
// if there are more queued popups.
func checkNextPopupCmd(ui *TUIImpl) tea.Cmd {
	if ui != nil && ui.hasPendingPopups() {
		return func() tea.Msg { return popupPendingMsg{} }
	}
	return nil
}

// newNotifyAssistantMsg creates a finalized assistant message for notifications.
func newNotifyAssistantMsg(text string) *messages.AssistantMessage {
	am := messages.NewAssistantMessage()
	am.Finalize(text)
	return am
}
