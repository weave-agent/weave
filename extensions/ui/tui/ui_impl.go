package tui

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"weave/sdk"

	tea "charm.land/bubbletea/v2"
)

// pendingCommand holds a command registered before the registry was set.
type pendingCommand struct {
	name    string
	handler func(args string) error
}

// pendingStatus holds a status update registered before the program was running.
type pendingStatus struct {
	key  string
	text string
}

// TUIImpl implements sdk.UI by delegating to the TUI's internal registries
// and overlay components.
type TUIImpl struct {
	program   Sender
	commands  *CommandRegistry
	bindings  *BindingRegistry
	renderers map[string]sdk.ToolRenderer

	mu              sync.Mutex
	popupQ          []*overlayRequest
	pending         []pendingCommand
	pendingStatuses []pendingStatus
	done            chan struct{}
}

// NewTUIImpl creates a UI implementation backed by the given registries.
// The program is set later via SetProgram once the tea.Program is running.
func NewTUIImpl(commands *CommandRegistry, bindings *BindingRegistry) *TUIImpl {
	return &TUIImpl{
		commands:  commands,
		bindings:  bindings,
		renderers: make(map[string]sdk.ToolRenderer),
		done:      make(chan struct{}),
	}
}

// SetProgram sets the Bubble Tea program for sending overlay requests.
func (u *TUIImpl) SetProgram(p Sender) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.program = p
}

// SetRegistries sets the command and binding registries under lock.
// Any commands registered before the registry was available are flushed.
func (u *TUIImpl) SetRegistries(commands *CommandRegistry, bindings *BindingRegistry) {
	u.mu.Lock()
	pending := u.pending
	u.pending = nil
	u.commands = commands
	u.bindings = bindings
	u.mu.Unlock()

	for _, pc := range pending {
		u.registerCommand(commands, pc.name, pc.handler)
	}
}

// Close signals that the TUI is shutting down, unblocking any pending overlay calls.
func (u *TUIImpl) Close() {
	u.mu.Lock()
	defer u.mu.Unlock()

	select {
	case <-u.done:
		// Already closed
	default:
		close(u.done)
	}
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

	select {
	case resp := <-req.result:
		return resp.index, resp.err
	case <-u.done:
		return -1, errors.New("tui shutting down")
	}
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

	select {
	case resp := <-req.result:
		return resp.confirmed, resp.err
	case <-u.done:
		return false, errors.New("tui shutting down")
	}
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

	select {
	case resp := <-req.result:
		return resp.value, resp.err
	case <-u.done:
		return "", errors.New("tui shutting down")
	}
}

// SetStatus updates the footer's extension status area.
// If the program is not yet set, the update is buffered and flushed
// when the event loop starts (via DrainStatuses).
func (u *TUIImpl) SetStatus(key, text string) {
	u.mu.Lock()
	p := u.program
	if p == nil {
		u.pendingStatuses = append(u.pendingStatuses, pendingStatus{key: key, text: text})
		u.mu.Unlock()
		return
	}
	u.mu.Unlock()

	p.Send(extStatusMsg{key: key, text: text})
}

// DrainStatuses returns and clears pending status updates buffered before
// the program was available. Called from Model.Init to flush initial statuses.
func (u *TUIImpl) DrainStatuses() []pendingStatus {
	u.mu.Lock()
	defer u.mu.Unlock()

	statuses := u.pendingStatuses
	u.pendingStatuses = nil
	return statuses
}

// Notify shows a temporary notification in the chat area.
func (u *TUIImpl) Notify(message string) {
	u.mu.Lock()
	p := u.program
	u.mu.Unlock()

	if p != nil {
		p.Send(notifyMsg{message: message})
	}
}

// RegisterCommand adds a command to the slash command registry.
// If the registry is not yet set, the command is buffered and applied
// when SetRegistries is called.
func (u *TUIImpl) RegisterCommand(name string, handler func(args string) error) {
	u.mu.Lock()

	if u.commands == nil {
		u.pending = append(u.pending, pendingCommand{name: name, handler: handler})
		u.mu.Unlock()

		return
	}

	commands := u.commands
	u.mu.Unlock()

	u.registerCommand(commands, name, handler)
}

func (u *TUIImpl) registerCommand(commands *CommandRegistry, name string, handler func(args string) error) {
	displayName := strings.TrimPrefix(name, "/")

	commands.Register(name, "", false, func(args string) CommandResult {
		err := handler(args)
		if err != nil {
			return CommandResult{Notify: fmt.Sprintf("/%s: error: %v", displayName, err)}
		}

		if strings.HasPrefix(name, "/skill:") {
			return CommandResult{}
		}

		return CommandResult{Notify: "/" + displayName + ": ok"}
	})

	u.mu.Lock()
	p := u.program
	u.mu.Unlock()

	if p != nil {
		p.Send(slashCommandsUpdatedMsg{})
	}
}

// RegisterRenderer stores a tool renderer for use by tool panels.
func (u *TUIImpl) RegisterRenderer(toolName string, renderer sdk.ToolRenderer) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.renderers[toolName] = renderer
}

// RegisterKeybinding delegates to the binding registry.
func (u *TUIImpl) RegisterKeybinding(kb sdk.Keybinding) {
	u.mu.Lock()
	bindings := u.bindings
	u.mu.Unlock()

	if bindings == nil {
		return
	}

	bindings.Register(BindingAction(kb.Name), kb.Keys, kb.Description)
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

	if u.program == nil {
		u.mu.Unlock()
		return errors.New("tui not running")
	}

	select {
	case <-u.done:
		u.mu.Unlock()
		return errors.New("tui shutting down")
	default:
	}

	u.popupQ = append(u.popupQ, req)
	p := u.program
	u.mu.Unlock()

	p.Send(popupPendingMsg{})

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

// handlePopupPending processes queued popup requests by pushing them onto the dialog stack.
func (m Model) handlePopupPending() (Model, tea.Cmd) {
	if m.ui == nil {
		return m, nil
	}

	req := m.ui.dequeue()
	if req == nil {
		return m, nil
	}

	return pushPopupDialog(m, req)
}
