package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"weave/ext/ui/tui/palette"
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

// TUIImpl implements sdk.UI and TUIExtAPI by delegating to the TUI's internal
// registries and overlay components.
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

	themeRegistry map[string]*palette.Theme
	activeTheme   string

	panelManager *PanelManager
	width        int
	height       int

	// Task 9: deferred implementation fields
	richRenderers         map[string]RichToolRenderer
	messageRenderers      map[string]MessageRenderer
	inputHandlers         []func(KeyEvent)
	autocompleteProviders []AutocompleteProvider
	workingFrames         []string
	workingInterval       time.Duration
}

// NewTUIImpl creates a UI implementation backed by the given registries.
// The program is set later via SetProgram once the tea.Program is running.
func NewTUIImpl(commands *CommandRegistry, bindings *BindingRegistry) *TUIImpl {
	return &TUIImpl{
		commands:  commands,
		bindings:  bindings,
		renderers: make(map[string]sdk.ToolRenderer),
		done:      make(chan struct{}),
		themeRegistry: map[string]*palette.Theme{
			"default": palette.DefaultTheme(),
		},
		activeTheme:      "default",
		panelManager:     NewPanelManager(),
		richRenderers:    make(map[string]RichToolRenderer),
		messageRenderers: make(map[string]MessageRenderer),
	}
}

// SetProgram sets the Bubble Tea program for sending overlay requests.
func (u *TUIImpl) SetProgram(p Sender) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.program = p
}

// SetSize updates the cached terminal dimensions.
func (u *TUIImpl) SetSize(width, height int) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.width = width
	u.height = height
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
func (u *TUIImpl) Select(title string, items []string, opts ...sdk.SelectOption) (int, error) {
	config := sdk.SelectConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	req := &overlayRequest{
		kind:        requestSelect,
		title:       title,
		items:       items,
		keepContent: config.KeepContent,
		result:      make(chan overlayResponse, 1),
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
func (u *TUIImpl) Confirm(message string, opts ...sdk.ConfirmOption) (bool, error) {
	config := sdk.ConfirmConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	req := &overlayRequest{
		kind:        requestConfirm,
		message:     message,
		keepContent: config.KeepContent,
		result:      make(chan overlayResponse, 1),
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
func (u *TUIImpl) Input(prompt string, opts ...sdk.InputOption) (string, error) {
	config := sdk.InputConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	req := &overlayRequest{
		kind:        requestInput,
		message:     prompt,
		keepContent: config.KeepContent,
		result:      make(chan overlayResponse, 1),
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

// MultiSelect shows a multi-selection overlay and blocks until the user responds.
func (u *TUIImpl) MultiSelect(title string, items []string, defaults []bool, opts ...sdk.SelectOption) ([]int, error) {
	config := sdk.SelectConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	req := &overlayRequest{
		kind:        requestMultiSelect,
		title:       title,
		items:       items,
		defaults:    defaults,
		keepContent: config.KeepContent,
		result:      make(chan overlayResponse, 1),
	}
	if err := u.enqueue(req); err != nil {
		return nil, err
	}

	select {
	case resp := <-req.result:
		return resp.selected, resp.err
	case <-u.done:
		return nil, errors.New("tui shutting down")
	}
}

// Editor shows an editor overlay and blocks until the user responds.
func (u *TUIImpl) Editor(prompt, initial string, opts ...sdk.EditorOption) (string, error) {
	config := sdk.EditorConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	req := &overlayRequest{
		kind:        requestEditor,
		title:       prompt,
		initial:     initial,
		keepContent: config.KeepContent,
		result:      make(chan overlayResponse, 1),
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

// NotifyTyped shows a typed notification in the chat area.
func (u *TUIImpl) NotifyTyped(message string, level sdk.NotifyLevel) {
	u.mu.Lock()
	p := u.program
	u.mu.Unlock()

	if p != nil {
		p.Send(notifyTypedMsg{message: message, level: level})
	}
}

// ShowError shows an error notification in the chat area.
func (u *TUIImpl) ShowError(message string) {
	u.NotifyTyped(message, sdk.NotifyError)
}

// SetWorking sets a working indicator in the UI.
func (u *TUIImpl) SetWorking(message string) {
	u.SetStatus("working", message)
}

// ClearWorking clears the working indicator.
func (u *TUIImpl) ClearWorking() {
	u.SetStatus("working", "")
}

// SetTheme sets the active UI theme.
func (u *TUIImpl) SetTheme(name string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	t, ok := u.themeRegistry[name]
	if !ok {
		return fmt.Errorf("unknown theme: %s", name)
	}

	u.activeTheme = name

	if u.program != nil {
		u.program.Send(themeChangedMsg{theme: t})
	}

	return nil
}

// ListThemes returns available theme names.
func (u *TUIImpl) ListThemes() []string {
	u.mu.Lock()
	defer u.mu.Unlock()

	names := make([]string, 0, len(u.themeRegistry))
	for name := range u.themeRegistry {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// RegisterTheme implements TUIExtAPI.
func (u *TUIImpl) RegisterTheme(name string, theme ThemeDef) error {
	if name == "" {
		return errors.New("theme name cannot be empty")
	}

	if name == "default" {
		return errors.New("cannot override default theme")
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	u.themeRegistry[name] = theme.toPaletteTheme()

	return nil
}

// Theme implements TUIExtAPI.
func (u *TUIImpl) Theme() sdk.ThemeInfo {
	u.mu.Lock()
	t := u.themeRegistry[u.activeTheme]
	name := u.activeTheme
	u.mu.Unlock()

	if t == nil {
		t = palette.DefaultTheme()
	}

	return sdk.ThemeInfo{
		Name:             name,
		Primary:          t.Primary,
		PrimaryDim:       t.PrimaryDim,
		PrimaryBright:    t.PrimaryBright,
		Success:          t.Success,
		Error:            t.Error,
		Warning:          t.Warning,
		Muted:            t.Muted,
		MutedBright:      t.MutedBright,
		Border:           t.Border,
		BorderFocused:    t.BorderFocused,
		Foreground:       t.Foreground,
		ForegroundBright: t.ForegroundBright,
	}
}

// --- TUIExtAPI: Panels ---

// ShowPanel registers and shows a panel.
func (u *TUIImpl) ShowPanel(config PanelConfig, drawer PanelDrawer) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.panelManager.Register(config, drawer)
	u.panelManager.Show(config.ID)

	if u.program != nil {
		u.program.Send(panelChangedMsg{})
	}
}

// HidePanel hides a panel.
func (u *TUIImpl) HidePanel(id string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.panelManager.Hide(id)

	if u.program != nil {
		u.program.Send(panelChangedMsg{})
	}
}

// RemovePanel fully removes a panel.
func (u *TUIImpl) RemovePanel(id string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.panelManager.Remove(id)

	if u.program != nil {
		u.program.Send(panelChangedMsg{})
	}
}

// PanelVisible returns whether a panel is currently visible.
func (u *TUIImpl) PanelVisible(id string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.panelManager.PanelVisible(id)
}

// PanelTray returns the panel tray API.
func (u *TUIImpl) PanelTray() PanelTrayAPI {
	return u
}

// SetOrder implements PanelTrayAPI.
func (u *TUIImpl) SetOrder(ids []string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.panelManager.order = ids
}

// GetOrder implements PanelTrayAPI.
func (u *TUIImpl) GetOrder() []string {
	u.mu.Lock()
	defer u.mu.Unlock()

	result := make([]string, len(u.panelManager.order))
	copy(result, u.panelManager.order)

	return result
}

// --- TUIExtAPI: Read-only ---

// Size returns the terminal dimensions.
func (u *TUIImpl) Size() (int, int) {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.width, u.height
}

// --- TUIExtAPI: Editor (stubs for Task 9) ---

// EditorText returns the current editor content.
func (u *TUIImpl) EditorText() string {
	// TODO: implement in Task 9
	return ""
}

// SetEditorText replaces the editor content.
func (u *TUIImpl) SetEditorText(text string) {
	// TODO: implement in Task 9
	if u.program != nil {
		u.program.Send(setEditorTextMsg{text: text})
	}
}

// PasteToEditor inserts text at the cursor position.
func (u *TUIImpl) PasteToEditor(text string) {
	// TODO: implement in Task 9
	if u.program != nil {
		u.program.Send(pasteToEditorMsg{text: text})
	}
}

// --- TUIExtAPI: Rendering (stubs for Task 9) ---

// RegisterRichRenderer registers a theme-aware tool renderer.
func (u *TUIImpl) RegisterRichRenderer(tool string, renderer RichToolRenderer) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.richRenderers[tool] = renderer
}

// RegisterMessageRenderer registers a custom message type renderer.
func (u *TUIImpl) RegisterMessageRenderer(msgType string, renderer MessageRenderer) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.messageRenderers[msgType] = renderer
}

// --- TUIExtAPI: Footer/Header (stubs for Task 9) ---

// SetFooter replaces the footer component.
func (u *TUIImpl) SetFooter(component TUIComponent) {
	// TODO: implement in Task 9
}

// SetHeader replaces the header component.
func (u *TUIImpl) SetHeader(component TUIComponent) {
	// TODO: implement in Task 9
}

// --- TUIExtAPI: Input (stubs for Task 9) ---

// OnTerminalInput registers a raw key event handler.
func (u *TUIImpl) OnTerminalInput(handler func(KeyEvent)) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.inputHandlers = append(u.inputHandlers, handler)
}

// AddAutocomplete registers an autocomplete provider.
func (u *TUIImpl) AddAutocomplete(provider AutocompleteProvider) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.autocompleteProviders = append(u.autocompleteProviders, provider)
}

// --- TUIExtAPI: Cosmetic (stubs for Task 9) ---

// SetWorkingFrames sets custom spinner animation frames.
func (u *TUIImpl) SetWorkingFrames(frames []string, interval time.Duration) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.workingFrames = frames
	u.workingInterval = interval
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

// Internal tea.Msg types for TUIExtAPI.

type panelChangedMsg struct{}

type setEditorTextMsg struct {
	text string
}

type pasteToEditorMsg struct {
	text string
}
