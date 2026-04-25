package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"weave/config"
	"weave/ext/ui/tui/components"
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"
	"weave/ext/ui/tui/palette"
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlaySession
	overlayModel
	overlayProvider
	overlayKeyInput
)

const doublePressWindow = 500 * time.Millisecond

const defaultEditorHeight = 3

const statusMessageTimeout = 2 * time.Second

// statusTimeoutMsg is sent when the transient status message should be cleared.
type statusTimeoutMsg struct {
	gen int
}

// Model is the root Bubble Tea model for the TUI.
type Model struct {
	width  int
	height int
	bus    sdk.Bus
	cfg    sdk.Config

	chat       components.ChatModel
	editor     components.EditorModel
	footer     components.FooterModel
	spinner    components.SpinnerModel
	prompted   bool
	toolPanels map[string]*messages.ToolPanel // track pending tool panels by ID
	commands   *CommandRegistry
	bindings   *BindingRegistry
	ui         *TUIImpl

	overlay           overlays.SelectorModel
	activeOverlay     overlayKind
	pendingSessions   []SessionEntry
	pendingModels     []ModelEntry
	pendingProviders  []ProviderEntry
	currentModel      ModelEntry
	prevModel         ModelEntry
	prevThinkingLevel sdk.ThinkingLevel

	keyInput       overlays.InputModel
	providerTarget string

	sessionDir string

	popup *popupState

	// double-press tracking
	lastCtrlC  time.Time
	lastEscape time.Time

	// thinking level state
	thinkingLevel sdk.ThinkingLevel

	// startup hints banner
	showHints bool

	// transient status message
	statusMsg   string
	statusTimer tea.Cmd
	statusGen   int
}

// newModel creates a new root model.
// If ui is non-nil, it is reused (production path) so that renderers registered
// via sdk.UI are visible to the model. If nil, a fresh TUIImpl is created (tests).
func newModel(bus sdk.Bus, cfg sdk.Config, ui *TUIImpl) Model {
	var cfgPath string
	if cfg != nil {
		cfgPath = cfg.FilePath()
	}

	sdir := resolveSessionDir(cfgPath)

	commands := NewCommandRegistry(bus, sdir)
	commands.register("/model", "Select or change model", func(_ string) CommandResult {
		return CommandResult{Command: listModelsCmd()}
	})

	commands.register("/providers", "Manage provider API keys", func(_ string) CommandResult {
		return CommandResult{Command: listProvidersCmd()}
	})

	commands.register("/thinking", "Set thinking level (off/minimal/low/medium/high/xhigh)", func(args string) CommandResult {
		if args == "" {
			return CommandResult{Notify: "Usage: /thinking <off|minimal|low|medium|high|xhigh>"}
		}

		level, err := sdk.ParseThinkingLevel(args)
		if err != nil {
			return CommandResult{Notify: err.Error()}
		}

		return CommandResult{Command: func() tea.Msg {
			return ThinkingLevelSetMsg{Level: level}
		}}
	})

	editor := components.NewEditorModel().Focus()
	editor = editor.SetSlashCommands(commands.Names())

	models := listModels()
	cur := initialModel(models)

	bindings := NewBindingRegistry()

	if cfg != nil && cfg.FilePath() != "" {
		if kbPath := loadKeybindings(cfg.FilePath()); kbPath != "" {
			_ = bindings.LoadUserConfig(kbPath)
		}
	}

	if ui == nil {
		ui = NewTUIImpl(commands, bindings)
	} else {
		ui.SetRegistries(commands, bindings)
	}

	m := Model{
		width:         80,
		height:        24,
		bus:           bus,
		cfg:           cfg,
		chat:          components.NewChatModel(),
		editor:        editor,
		footer:        components.NewFooterModel(),
		spinner:       components.NewSpinnerModel(),
		toolPanels:    make(map[string]*messages.ToolPanel),
		commands:      commands,
		bindings:      bindings,
		ui:            ui,
		currentModel:  cur,
		sessionDir:    sdir,
		thinkingLevel: sdk.DefaultThinkingLevel(),
		showHints:     true,
	}
	m.footer = m.footer.SetModel(cur.Model, cur.Provider)
	m.footer = m.footer.SetReasoning(modelReasoning(cur.Model))
	m.footer = m.footer.SetThinkingLevel(string(m.thinkingLevel))
	m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(m.thinkingLevel))

	return m
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
//
//nolint:gocyclo // central message dispatch for the TUI
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle spinner control messages
	if components.IsSpinnerMsg(msg) {
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.SpinnerUpdate(msg)

		return m, cmd
	}

	// Handle popup overlay messages first
	if m.popup != nil {
		switch msg.(type) {
		case tea.KeyMsg:
			return m.handlePopupUpdate(msg)
		case overlays.SelectorSelectedMsg:
			return m.handlePopupUpdate(msg)
		case overlays.SelectorCancelledMsg:
			return m.handlePopupUpdate(msg)
		case overlays.ConfirmResultMsg:
			return m.handlePopupUpdate(msg)
		case overlays.InputResultMsg:
			return m.handlePopupUpdate(msg)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chat = m.chat.SetSize(m.width, m.chatHeight(m.height))
		m.editor = m.editor.SetSize(m.width, defaultEditorHeight)
		m.footer = m.footer.SetSize(m.width)
		m.spinner = m.spinner.SetSize(m.width)
		m.overlay = m.overlay.SetSize(m.width, m.height)
		m.keyInput = m.keyInput.SetSize(m.width, m.height)

		return m, nil

	case tea.KeyMsg:
		// Overlay gets priority when active (ctrl+c dismisses it)
		if m.activeOverlay != overlayNone {
			if msg.String() == "ctrl+c" {
				m.activeOverlay = overlayNone
				m.overlay = m.overlay.Hide()
				m.keyInput = m.keyInput.Hide()
				m.pendingProviders = nil
				m.providerTarget = ""

				return m, nil
			}

			var cmd tea.Cmd

			if m.activeOverlay == overlayKeyInput {
				m.keyInput, cmd = m.keyInput.Update(msg)
			} else {
				m.overlay, cmd = m.overlay.Update(msg)
			}

			return m, cmd
		}

		// Dismiss startup hints on first keypress
		m.showHints = false

		// Handle ctrl+c with double-press: first clears editor, second quits
		if msg.Type == tea.KeyCtrlC {
			return m.handleCtrlC()
		}

		// Handle escape with double-press: first interrupts, second clears editor
		if msg.Type == tea.KeyEsc {
			return m.handleEscape()
		}

		// Try keybinding resolver
		if action, ok := m.bindings.Resolve(keyString(msg)); ok {
			return m.dispatchBinding(action)
		}

		// Fall through to editor
		var cmd tea.Cmd

		m.editor, cmd = m.editor.Update(msg)

		return m, cmd

	case externalEditorMsg:
		if msg.err == nil && msg.text != "" {
			m.editor = m.editor.SetValue(msg.text)
		}

		return m, nil

	case components.SubmitMsg:
		return m.onSubmit(msg.Text)

	case TurnStartMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.SpinnerUpdate(components.SpinnerShowMsg{})

		return m, cmd

	case MessageStartMsg:
		m.chat = m.chat.AddItem(messages.NewAssistantMessage())

	case MessageUpdateMsg:
		m.onMessageUpdate(msg.Content)

	case MessageEndMsg:
		m.onMessageEnd(msg)

	case ToolResultMsg:
		m.onToolResult(msg)

	case TurnEndMsg:
		m.spinner = m.spinner.Hide()

	case AgentEndMsg:
		m.spinner = m.spinner.Hide()

		if msg.Payload != nil {
			if errStr, ok := msg.Payload.(string); ok && errStr != "" {
				am := messages.NewAssistantMessage()
				am.Finalize("[error] " + errStr)
				m.chat = m.chat.AddItem(am)
			}
		}

	case SessionListResultMsg:
		return m.onSessionListResult(msg)

	case ModelListResultMsg:
		return m.onModelListResult(msg)

	case ModelChangedMsg:
		return m.onModelChanged(msg)

	case ModelChangeFailedMsg:
		return m.onModelChangeFailed(msg)

	case ProviderListResultMsg:
		return m.onProviderListResult(msg)

	case overlays.SelectorSelectedMsg:
		return m.onOverlaySelected(msg)

	case overlays.SelectorCancelledMsg:
		m.activeOverlay = overlayNone
		m.overlay = m.overlay.Hide()
		m.pendingSessions = nil
		m.pendingModels = nil
		m.pendingProviders = nil

		return m, nil

	case ShutdownMsg:
		return m, tea.Quit

	case popupPendingMsg:
		return m.handlePopupPending()

	case extStatusMsg:
		m.footer = m.footer.SetExtStatus(msg.key, msg.text)
		return m, nil

	case notifyMsg:
		m.chat = m.chat.AddItem(newNotifyAssistantMsg(msg.message))
		return m, nil

	case overlays.ConfirmResultMsg:
		return m, nil

	case overlays.InputResultMsg:
		return m.onKeyInputResult(msg)

	case statusTimeoutMsg:
		if msg.gen == m.statusGen {
			m.statusMsg = ""
			m.statusTimer = nil
		}

		return m, nil

	case ThinkingLevelSetMsg:
		return m.applyThinkingLevel(msg.Level)
	}

	// Forward spinner ticks
	if m.spinner.Visible() {
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
	}

	return m, nil
}

// handleCtrlC implements double-press ctrl+c: first press clears editor,
// second press within the window quits.
func (m Model) handleCtrlC() (tea.Model, tea.Cmd) {
	now := time.Now()

	if m.editor.Value() != "" {
		m.editor = m.editor.SetValue("")
		m.lastCtrlC = now

		return m, nil
	}

	// Editor is empty — check for double press
	if now.Sub(m.lastCtrlC) < doublePressWindow {
		return m, tea.Quit
	}

	m.lastCtrlC = now

	return m, nil
}

// handleEscape implements double-press escape: first press interrupts streaming,
// second press within the window clears the editor.
func (m Model) handleEscape() (tea.Model, tea.Cmd) {
	now := time.Now()

	// Check for double press — clear editor
	if now.Sub(m.lastEscape) < doublePressWindow {
		m.editor = m.editor.SetValue("")
		m.lastEscape = time.Time{}

		return m, nil
	}

	m.lastEscape = now

	// First press — interrupt streaming if active
	return m.interruptStreaming()
}

// dispatchBinding handles a resolved keybinding action.
//
//nolint:gocyclo // central keybinding dispatch
func (m Model) dispatchBinding(action BindingAction) (tea.Model, tea.Cmd) {
	switch action {
	case ActionExit:
		return m, tea.Quit
	case ActionModelSelect:
		return m, listModelsCmd()
	case ActionModelCycle:
		models := listModels()
		if len(models) <= 1 {
			m.showStatus("Only one model available")
			return m, m.statusTimer
		}

		next := cycleModel(models, m.currentModel)

		return m, func() tea.Msg { return ModelChangedMsg{Entry: next} }

	// Editor navigation
	case ActionCursorLineStart:
		m.editor = m.editor.CursorLineStart()
		return m, nil
	case ActionCursorLineEnd:
		m.editor = m.editor.CursorLineEnd()
		return m, nil
	case ActionCursorWordLeft:
		m.editor = m.editor.CursorWordLeft()
		return m, nil
	case ActionCursorWordRight:
		m.editor = m.editor.CursorWordRight()
		return m, nil

	// Chat scroll
	case ActionScrollUp:
		m.chat = m.chat.ScrollUp(m.chatHeight(m.height))
		return m, nil
	case ActionScrollDown:
		m.chat = m.chat.ScrollDown(m.chatHeight(m.height))
		return m, nil

	// Editor deletion
	case ActionDeleteWordBackward:
		m.editor = m.editor.DeleteWordBackward()
		return m, nil
	case ActionDeleteWordForward:
		m.editor = m.editor.DeleteWordForward()
		return m, nil
	case ActionDeleteToLineStart:
		m.editor = m.editor.DeleteToLineStart()
		return m, nil
	case ActionDeleteToLineEnd:
		m.editor = m.editor.DeleteToLineEnd()
		return m, nil

	// Undo
	case ActionUndo:
		m.editor = m.editor.Undo()
		return m, nil

	// App control
	case ActionSuspend:
		return m, func() tea.Msg { return tea.SuspendMsg{} }
	case ActionExternalEditor:
		return m.openExternalEditor()

	// Display
	case ActionToggleToolOutput:
		m.toggleLastToolOutput()
		return m, nil
	case ActionToggleThinking:
		m.toggleLastThinkingBlock()
		return m, nil

	case ActionThinkingCycle:
		return m.cycleThinkingLevel()

	// Session
	case ActionNewSession:
		m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
		m.toolPanels = make(map[string]*messages.ToolPanel)
		m.prompted = false

		return m, nil

	default:
		return m, nil
	}
}

// toggleLastToolOutput expands or collapses the last tool output panel.
func (m *Model) toggleLastToolOutput() {
	items := m.chat.Items()
	for i := len(items) - 1; i >= 0; i-- {
		if tp, ok := items[i].(*messages.ToolPanel); ok {
			tp.ToggleExpanded()
			m.chat = m.chat.UpdateItemByID(tp)

			return
		}
	}
}

// toggleLastThinkingBlock expands or collapses the last thinking block.
func (m *Model) toggleLastThinkingBlock() {
	items := m.chat.Items()
	for i := len(items) - 1; i >= 0; i-- {
		if tb, ok := items[i].(*messages.ThinkingBlock); ok {
			tb.ToggleExpanded()
			m.chat = m.chat.UpdateItemByID(tb)

			return
		}
	}
}

// externalEditorMsg is sent when the external editor finishes.
type externalEditorMsg struct {
	text string
	err  error
}

// openExternalEditor opens the current editor content in an external editor.
func (m Model) openExternalEditor() (tea.Model, tea.Cmd) {
	text := m.editor.Value()

	tmpFile, err := os.CreateTemp("", "weave-editor-*.md")
	if err != nil {
		return m, nil
	}

	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(text); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)

		return m, nil
	}

	_ = tmpFile.Close()

	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}

	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, tmpPath) //nolint:gosec,noctx // editor path comes from env

	return m, tea.ExecProcess(cmd, func(procErr error) tea.Msg {
		if procErr != nil {
			_ = os.Remove(tmpPath)

			return externalEditorMsg{err: procErr}
		}

		data, readErr := os.ReadFile(tmpPath)
		_ = os.Remove(tmpPath)

		if readErr != nil {
			return externalEditorMsg{err: readErr}
		}

		return externalEditorMsg{text: string(data)}
	})
}

// onMessageUpdate appends a delta to the current assistant message.
func (m *Model) onMessageUpdate(delta string) {
	items := m.chat.Items()
	if len(items) == 0 {
		return
	}

	am, ok := items[len(items)-1].(*messages.AssistantMessage)
	if !ok || !am.IsStreaming() {
		return
	}

	am.Append(delta)
	m.chat = m.chat.UpdateItem(am)
}

// onMessageEnd finalizes the current assistant message and creates pending tool panels.
func (m *Model) onMessageEnd(msg MessageEndMsg) {
	items := m.chat.Items()
	if len(items) == 0 {
		return
	}

	am, ok := items[len(items)-1].(*messages.AssistantMessage)
	if !ok {
		return
	}

	// If the message was already finalized (e.g. by interrupt), don't overwrite.
	if !am.IsStreaming() {
		return
	}

	am.Finalize(msg.Content)
	m.chat = m.chat.UpdateItem(am)

	if msg.Thinking != "" {
		m.chat = m.chat.AddItem(messages.NewThinkingBlock(msg.Thinking))
	}

	for _, tc := range msg.ToolCalls {
		args, _ := json.Marshal(tc.Arguments)
		argsStr := string(args)

		panel := messages.NewToolPanel(tc.ID, tc.Name, argsStr)
		if m.ui != nil {
			if r, ok := m.ui.GetRenderer(tc.Name); ok {
				panel.SetRenderer(r)
			} else {
				panel.SetDiffRenderer(messages.NewDiffRenderer())
			}
		} else {
			panel.SetDiffRenderer(messages.NewDiffRenderer())
		}

		m.toolPanels[tc.ID] = panel
		m.chat = m.chat.AddItem(panel)
	}
}

// onToolResult updates the tool panel with the result.
func (m *Model) onToolResult(msg ToolResultMsg) {
	panel, ok := m.toolPanels[msg.ToolID]
	if !ok {
		panel = messages.NewToolPanel(msg.ToolID, msg.Tool, "")
		m.toolPanels[msg.ToolID] = panel
		m.chat = m.chat.AddItem(panel)
	}

	panel.SetResult(msg.Result.Content, msg.Result.IsError)
	m.chat = m.chat.UpdateItemByID(panel)
}

// interruptStreaming finalizes the current streaming assistant message with
// an [interrupted] tag, hides the spinner, and publishes an interrupt event.
func (m Model) interruptStreaming() (tea.Model, tea.Cmd) {
	items := m.chat.Items()
	if len(items) == 0 {
		return m, nil
	}

	am, ok := items[len(items)-1].(*messages.AssistantMessage)
	if !ok || !am.IsStreaming() {
		return m, nil
	}

	am.Interrupt()
	m.chat = m.chat.UpdateItem(am)
	m.spinner = m.spinner.Hide()

	var cmds []tea.Cmd
	if m.bus != nil {
		cmds = append(cmds, PublishInterrupt(m.bus))
	}

	return m, tea.Batch(cmds...)
}

// AddUserMessage adds a user message to the chat.
func (m *Model) AddUserMessage(content string) {
	m.chat = m.chat.AddItem(messages.NewUserMessage(content))
}

// onSubmit handles editor submit — routes slash commands or publishes prompt/followup.
func (m Model) onSubmit(text string) (tea.Model, tea.Cmd) {
	// Try slash command dispatch first.
	if handled, result := m.commands.Dispatch(text); handled { //nolint:nestif // command dispatch has multiple optional outcomes
		if result.Quit {
			return m, tea.Quit
		}

		if result.ClearChat {
			m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
			m.toolPanels = make(map[string]*messages.ToolPanel)
		}

		if result.ResetPrompt {
			m.prompted = false
		}

		if result.Notify != "" {
			m.chat = m.chat.AddItem(messages.NewAssistantMessage())

			items := m.chat.Items()
			if am, ok := items[len(items)-1].(*messages.AssistantMessage); ok {
				am.Finalize(result.Notify)
				m.chat = m.chat.UpdateItem(am)
			}
		}

		return m, result.Command
	}

	m.AddUserMessage(text)

	if !m.prompted {
		m.prompted = true
		return m, PublishPrompt(m.bus, text)
	}

	return m, PublishFollowup(m.bus, text)
}

func (m Model) onSessionListResult(msg SessionListResultMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("Error listing sessions: %v", msg.Err))
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	if len(msg.Sessions) == 0 {
		am := messages.NewAssistantMessage()
		am.Finalize("No sessions found.")
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	items := make([]overlays.SelectorItem, len(msg.Sessions))
	for i, s := range msg.Sessions {
		items[i] = overlays.SelectorItem{
			Title:    shortenCWD(s.CWD),
			Subtitle: s.CreatedAt.Format("2006-01-02 15:04"),
		}
	}

	m.pendingSessions = msg.Sessions
	m.overlay = overlays.NewSelectorModel("Resume Session", items)
	m.overlay = m.overlay.SetSize(m.width, m.height)
	m.overlay = m.overlay.Show()
	m.activeOverlay = overlaySession

	return m, nil
}

func (m Model) onModelListResult(msg ModelListResultMsg) (tea.Model, tea.Cmd) {
	if len(msg.Models) == 0 {
		am := messages.NewAssistantMessage()
		am.Finalize("No models available.")
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	if len(msg.Models) == 1 {
		am := messages.NewAssistantMessage()
		am.Finalize("Only one model available: " + msg.Models[0].Display())
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	items := make([]overlays.SelectorItem, len(msg.Models))
	for i, model := range msg.Models {
		title := model.DisplayName()
		if model.Provider == m.currentModel.Provider && model.Model == m.currentModel.Model {
			title += " ✓"
		}

		items[i] = overlays.SelectorItem{
			Title:    title,
			Subtitle: "[" + model.Provider + "]",
		}
	}

	m.pendingModels = msg.Models
	m.overlay = overlays.NewSelectorModel("Select Model", items)
	m.overlay = m.overlay.SetSize(m.width, m.height)
	m.overlay = m.overlay.Show()
	m.activeOverlay = overlayModel

	return m, nil
}

func (m Model) onModelChanged(msg ModelChangedMsg) (tea.Model, tea.Cmd) {
	m.prevModel = m.currentModel
	m.prevThinkingLevel = m.thinkingLevel
	m.currentModel = msg.Entry
	m.footer = m.footer.SetModel(msg.Entry.Model, msg.Entry.Provider)
	m.footer = m.footer.SetReasoning(modelReasoning(msg.Entry.Model))

	thinkingChanged := false

	if modelDef, ok := sdk.GetModel(msg.Entry.Model); ok {
		if !modelDef.Reasoning {
			thinkingChanged = m.thinkingLevel != sdk.ThinkingOff
			m.thinkingLevel = sdk.ThinkingOff
			m.footer = m.footer.SetThinkingLevel(string(sdk.ThinkingOff))
			m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(sdk.ThinkingOff))
		} else if clamped := sdk.ClampForModel(m.thinkingLevel, modelDef); clamped != m.thinkingLevel {
			thinkingChanged = true
			m.thinkingLevel = clamped
			m.footer = m.footer.SetThinkingLevel(string(clamped))
			m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(clamped))
		}
	}

	displayName := msg.Entry.DisplayName()
	m.showStatus(fmt.Sprintf("Switched to %s (thinking: %s)", displayName, m.thinkingLevel))

	if m.bus != nil {
		var cmds []tea.Cmd

		cmds = append(cmds, PublishModelChange(m.bus, msg.Entry))

		if thinkingChanged {
			cmds = append(cmds, PublishThinkingChange(m.bus, m.thinkingLevel))
		}

		cmds = append(cmds, m.statusTimer)

		return m, tea.Batch(cmds...)
	}

	return m, m.statusTimer
}

func (m Model) onModelChangeFailed(msg ModelChangeFailedMsg) (tea.Model, tea.Cmd) {
	m.currentModel = m.prevModel
	m.footer = m.footer.SetModel(m.prevModel.Model, m.prevModel.Provider)
	m.footer = m.footer.SetReasoning(modelReasoning(m.prevModel.Model))

	m.thinkingLevel = m.prevThinkingLevel
	m.footer = m.footer.SetThinkingLevel(string(m.thinkingLevel))
	m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(m.thinkingLevel))

	am := messages.NewAssistantMessage()
	am.Finalize("Failed to switch provider: " + msg.Error)
	m.chat = m.chat.AddItem(am)

	if m.bus != nil {
		return m, PublishThinkingChange(m.bus, m.thinkingLevel)
	}

	return m, nil
}

func (m Model) onProviderListResult(msg ProviderListResultMsg) (tea.Model, tea.Cmd) {
	if len(msg.Providers) == 0 {
		am := messages.NewAssistantMessage()
		am.Finalize("No providers available.")
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	items := make([]overlays.SelectorItem, len(msg.Providers))
	for i, p := range msg.Providers {
		statusText := "no key"
		if p.HasKey {
			statusText = "key set"
		}

		items[i] = overlays.SelectorItem{
			Title:    p.Name,
			Subtitle: statusText,
		}
	}

	m.pendingProviders = msg.Providers
	m.overlay = overlays.NewSelectorModel("Manage Provider Keys", items)
	m.overlay = m.overlay.SetSize(m.width, m.height)
	m.overlay = m.overlay.Show()
	m.activeOverlay = overlayProvider

	return m, nil
}

func (m Model) onProviderSelected(msg overlays.SelectorSelectedMsg) (tea.Model, tea.Cmd) {
	if msg.Index < 0 || msg.Index >= len(m.pendingProviders) {
		m.activeOverlay = overlayNone
		m.overlay = m.overlay.Hide()
		m.pendingProviders = nil

		return m, nil
	}

	selected := m.pendingProviders[msg.Index]
	m.providerTarget = selected.Name

	m.overlay = m.overlay.Hide()
	m.activeOverlay = overlayKeyInput

	m.keyInput = overlays.NewInputModel("Enter API key for " + selected.Name)
	m.keyInput = m.keyInput.SetSize(m.width, m.height)
	m.keyInput = m.keyInput.Show()

	return m, nil
}

func (m Model) onKeyInputResult(msg overlays.InputResultMsg) (tea.Model, tea.Cmd) {
	m.activeOverlay = overlayNone
	m.keyInput = m.keyInput.Hide()

	if !msg.Ok || m.providerTarget == "" {
		m.pendingProviders = nil
		m.providerTarget = ""

		return m, nil
	}

	apiKey := strings.TrimSpace(msg.Value)
	if apiKey == "" {
		m.pendingProviders = nil
		m.providerTarget = ""

		return m, nil
	}

	providerName := m.providerTarget
	err := config.SetProviderKey(providerName, apiKey)

	m.pendingProviders = nil
	m.providerTarget = ""

	am := messages.NewAssistantMessage()
	if err != nil {
		am.Finalize(fmt.Sprintf("Failed to save API key for %s: %v", providerName, err))
	} else {
		am.Finalize(fmt.Sprintf("API key saved for %s.", providerName))
	}

	m.chat = m.chat.AddItem(am)

	return m, nil
}

func (m Model) onOverlaySelected(msg overlays.SelectorSelectedMsg) (tea.Model, tea.Cmd) {
	switch m.activeOverlay {
	case overlaySession:
		return m.onSessionSelected(msg)
	case overlayModel:
		return m.onModelSelected(msg)
	case overlayProvider:
		return m.onProviderSelected(msg)
	default:
		m.activeOverlay = overlayNone
		m.overlay = m.overlay.Hide()

		return m, nil
	}
}

func (m Model) onSessionSelected(msg overlays.SelectorSelectedMsg) (tea.Model, tea.Cmd) {
	m.activeOverlay = overlayNone
	m.overlay = m.overlay.Hide()

	if msg.Index < 0 || msg.Index >= len(m.pendingSessions) {
		m.pendingSessions = nil
		return m, nil
	}

	session := m.pendingSessions[msg.Index]
	m.pendingSessions = nil

	m.rebuildChatFromSession(session.ID)
	m.prompted = false

	if m.bus != nil {
		return m, PublishSessionResume(m.bus, session.ID)
	}

	return m, nil
}

func (m Model) onModelSelected(msg overlays.SelectorSelectedMsg) (tea.Model, tea.Cmd) {
	m.activeOverlay = overlayNone
	m.overlay = m.overlay.Hide()

	if msg.Index < 0 || msg.Index >= len(m.pendingModels) {
		m.pendingModels = nil
		return m, nil
	}

	selected := m.pendingModels[msg.Index]
	m.pendingModels = nil

	return m.onModelChanged(ModelChangedMsg{Entry: selected})
}

func (m *Model) rebuildChatFromSession(sessionID string) {
	entries, err := loadSessionEntries(m.sessionDir, sessionID)
	if err != nil {
		m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("Error loading session: %v", err))
		m.chat = m.chat.AddItem(am)

		return
	}

	m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
	m.toolPanels = make(map[string]*messages.ToolPanel)

	for _, entry := range entries {
		switch entry.Role {
		case sdk.RoleUser:
			m.chat = m.chat.AddItem(messages.NewUserMessage(entry.Content))
		case sdk.RoleAssistant:
			am := messages.NewAssistantMessage()
			am.Finalize(entry.Content)
			m.chat = m.chat.AddItem(am)
		case sdk.RoleToolResult:
			toolName, toolContent, toolIsError := parseToolEntry(entry.Tool)
			panel := messages.NewToolPanel("", toolName, "")
			panel.SetResult(toolContent, toolIsError)
			m.chat = m.chat.AddItem(panel)
		}
	}
}

// parseToolEntry extracts tool name, content, and error flag from a stored tool entry.
func parseToolEntry(raw json.RawMessage) (name, content string, isError bool) {
	if len(raw) == 0 {
		return "tool", "", false
	}

	var toolData struct {
		Tool   string `json:"tool"`
		Result struct {
			Content string `json:"content"`
			IsError bool   `json:"is_error"`
		} `json:"result"`
	}

	if err := json.Unmarshal(raw, &toolData); err != nil {
		return "tool", "", false
	}

	name = toolData.Tool
	if name == "" {
		name = "tool"
	}

	return name, toolData.Result.Content, toolData.Result.IsError
}

// cycleThinkingLevel returns the next distinct thinking level, skipping
// levels that would clamp to the same effective value as the current one.
func (m Model) cycleThinkingLevel() (tea.Model, tea.Cmd) {
	cur := m.thinkingLevel
	if modelDef, ok := sdk.GetModel(m.currentModel.Model); ok && modelDef.Reasoning {
		cur = sdk.ClampForModel(cur, modelDef)
	}

	for i, lvl := range sdk.AllThinkingLevels {
		if lvl != cur {
			continue
		}

		for j := 1; j <= len(sdk.AllThinkingLevels); j++ {
			candidate := sdk.AllThinkingLevels[(i+j)%len(sdk.AllThinkingLevels)]

			var effective sdk.ThinkingLevel

			if modelDef, ok := sdk.GetModel(m.currentModel.Model); ok {
				if modelDef.Reasoning {
					effective = sdk.ClampForModel(candidate, modelDef)
				} else {
					effective = sdk.ThinkingOff
				}
			} else {
				effective = candidate
			}

			if effective != cur {
				return m.applyThinkingLevel(candidate)
			}
		}

		return m, nil
	}

	return m, nil
}

// applyThinkingLevel applies a thinking level change.
// It clamps xhigh for models that don't support it, updates UI elements,
// shows a status message, and publishes the bus event.
func (m Model) applyThinkingLevel(level sdk.ThinkingLevel) (tea.Model, tea.Cmd) {
	if modelDef, ok := sdk.GetModel(m.currentModel.Model); ok {
		if !modelDef.Reasoning {
			level = sdk.ThinkingOff
		} else {
			level = sdk.ClampForModel(level, modelDef)
		}
	}

	m.thinkingLevel = level
	m.footer = m.footer.SetThinkingLevel(string(level))
	m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(level))

	m.showStatus(fmt.Sprintf("Thinking level: %s", level))

	var cmds []tea.Cmd

	if m.bus != nil {
		cmds = append(cmds, PublishThinkingChange(m.bus, level))
	}

	if m.statusTimer != nil {
		cmds = append(cmds, m.statusTimer)
	}

	return m, tea.Batch(cmds...)
}

// showStatus sets a transient status message that clears after a timeout.
func (m *Model) showStatus(msg string) {
	m.statusMsg = msg
	m.statusGen++
	gen := m.statusGen
	m.statusTimer = tea.Tick(statusMessageTimeout, func(_ time.Time) tea.Msg {
		return statusTimeoutMsg{gen: gen}
	})
}

// chatHeight returns the height allocated to the chat area.
func (m Model) chatHeight(totalHeight int) int {
	editorLines := defaultEditorHeight + 2 // editor height + border
	footerLines := 2

	spinnerLines := 0
	if m.spinner.Visible() {
		spinnerLines = 1
	}

	statusLines := 0
	if m.statusMsg != "" {
		statusLines = 1
	}

	hintsLines := 0
	if m.showHints && !m.prompted && len(m.chat.Items()) == 0 {
		hintsLines = 1
	}

	reserved := editorLines + footerLines + spinnerLines + statusLines + hintsLines
	if totalHeight > reserved+1 {
		return totalHeight - reserved
	}

	return 1
}

// View renders the TUI.
func (m Model) View() string {
	// Popup overlay takes highest priority
	if m.popup != nil {
		if view := m.popupView(); view != "" {
			return view
		}
	}

	if m.activeOverlay != overlayNone {
		if m.activeOverlay == overlayKeyInput && m.keyInput.Visible() {
			return m.keyInput.View()
		}

		if m.overlay.Visible() {
			return m.overlay.View()
		}
	}

	var sections []string

	if m.showHints && !m.prompted && len(m.chat.Items()) == 0 {
		hintsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		sections = append(sections, hintsStyle.Render(
			"ctrl+p cycle model · ctrl+l select model · shift+tab cycle thinking · ctrl+t toggle thinking",
		))
	}

	sections = append(sections, m.chat.View())

	if m.spinner.Visible() {
		sections = append(sections, m.spinner.View())
	}

	if m.statusMsg != "" {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		sections = append(sections, statusStyle.Render(m.statusMsg))
	}

	sections = append(sections, m.editor.View(), m.footer.View())

	return strings.Join(sections, "\n")
}
