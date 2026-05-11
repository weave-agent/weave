package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"weave/ext/ui/tui/components"
	"weave/ext/ui/tui/components/attachments"
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"
	"weave/ext/ui/tui/palette"
	"weave/internal/auth"
	"weave/sdk"
	sdkmodel "weave/sdk/model"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const doublePressWindow = 500 * time.Millisecond

const statusMessageTimeout = 2 * time.Second

// statusTimeoutMsg is sent when the transient status message should be cleared.
type statusTimeoutMsg struct {
	gen int
}

// doublePressTimeoutMsg is sent when the double-press window expires.
type doublePressTimeoutMsg struct {
	kind int // 0 = ctrl+c, 1 = escape
	gen  int // generation counter to ignore stale timers
}

const (
	doublePressCtrlC  = 0
	doublePressEscape = 1
)

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
	layout     LayoutEngine

	pendingSessions   []SessionEntry
	pendingModels     []ModelEntry
	pendingProviders  []ProviderEntry
	currentModel      ModelEntry
	prevModel         ModelEntry
	prevThinkingLevel sdkmodel.ThinkingLevel
	dialogStack       overlays.DialogStack
	attach            attachments.Model

	providerTarget string
	popupChans     map[string]chan overlayResponse
	popupSeq       int

	sessionDir string

	// double-press tracking
	ctrlCPressed   bool
	escapePressed  bool
	doublePressGen int

	// thinking level state
	thinkingLevel sdkmodel.ThinkingLevel

	// noConfigured is true when no provider has an API key set.
	noConfigured bool

	// startup hints banner
	showHints bool

	// landing screen shown before first prompt
	showLanding bool
	landing     LandingModel

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
	commands.register("/model", "Select or change model", false, func(_ string) CommandResult {
		return CommandResult{Command: listModelsCmd(cfg)}
	})

	commands.register("/providers", "Manage provider API keys", false, func(_ string) CommandResult {
		return CommandResult{Command: listProvidersCmd(cfg)}
	})

	commands.register("/thinking", "Set thinking level (off/minimal/low/medium/high/xhigh)", false, func(args string) CommandResult {
		if args == "" {
			return CommandResult{Notify: "Usage: /thinking <off|minimal|low|medium|high|xhigh>"}
		}

		level, err := sdkmodel.ParseThinkingLevel(args)
		if err != nil {
			return CommandResult{Notify: err.Error()}
		}

		return CommandResult{Command: func() tea.Msg {
			return ThinkingLevelSetMsg{Level: level}
		}}
	})

	editor := components.NewEditorModel()

	// Read UI settings from layered config
	var us Settings
	if cfg != nil {
		_ = cfg.UIConfig(&us)
	}

	if us.EditorMaxLines > 0 {
		editor = editor.SetMaxHeight(us.EditorMaxLines)
	}

	effectiveCfg := effectiveConfig(cfg)
	models := listModels(effectiveCfg)
	cur := currentModel(models, effectiveCfg)

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
		layout:        NewLayoutEngine(),
		currentModel:  cur,
		sessionDir:    sdir,
		thinkingLevel: initialThinkingLevel(effectiveCfg),
		noConfigured:  len(models) == 0,
		showHints:     true,
		showLanding:   true,
		landing:       NewLandingModel(cur.Model, cur.Provider),
		dialogStack:   overlays.NewDialogStack(),
		popupChans:    make(map[string]chan overlayResponse),
	}
	m.footer = m.footer.SetModel(cur.Model, cur.Provider)
	m.footer = m.footer.SetReasoning(modelReasoning(cur.Model))
	m.footer = m.footer.SetThinkingLevel(string(m.thinkingLevel))
	m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(m.thinkingLevel))

	if m.noConfigured {
		m.statusMsg = "No providers configured. Use /providers to set an API key."
	}

	return m
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	if m.bus != nil {
		cmds = append(cmds, PublishModelChange(m.bus, m.currentModel))
	}

	// Flush status updates buffered during UI extension wiring
	// (before the event loop was running).
	if m.ui != nil {
		for _, s := range m.ui.DrainStatuses() {
			cmds = append(cmds, func() tea.Msg {
				return extStatusMsg(s)
			})
		}
	}

	return tea.Batch(cmds...)
}

// Update handles messages.
//
//nolint:gocyclo // central message dispatch for the TUI
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Dialog stack gets priority when non-empty.
	if !m.dialogStack.Empty() {
		// Ctrl+C force-dismisses the top dialog.
		if km, ok := msg.(tea.KeyPressMsg); ok && km.String() == "ctrl+c" {
			var d overlays.Dialog

			m.dialogStack, d = m.dialogStack.Pop()

			return m.handleDialogForceCancel(d)
		}

		newStack, cmd, completed := m.dialogStack.Update(msg)
		m.dialogStack = newStack

		// Handle dialogs that completed during fall-through (at most one).
		if len(completed) > 0 {
			return m.handleDialogDone(completed[0], cmd)
		}

		// Check if the top dialog completed.
		if top := m.dialogStack.Peek(); top != nil && top.Done() {
			var d overlays.Dialog

			m.dialogStack, d = m.dialogStack.Pop()

			return m.handleDialogDone(d, cmd)
		}

		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chat = m.chat.SetSize(m.width, m.chatHeight(m.height))
		m.editor = m.editor.SetSize(m.width, m.editor.Height())
		m.footer = m.footer.SetSize(m.width)
		m.spinner = m.spinner.SetSize(m.width)
		m.dialogStack = m.dialogStack.Resize(m.width, m.height)

		return m, nil

	case tea.KeyPressMsg:
		// Dismiss startup hints on first keypress
		m.showHints = false

		// Attachment delete mode: intercept navigation keys
		if m.attach.InDeleteMode() {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.attach = m.attach.ToggleDeleteMode()
				return m, nil
			case "up", "left":
				m.attach = m.attach.DeleteModePrev()
				return m, nil
			case "down", "right":
				m.attach = m.attach.DeleteModeNext()
				return m, nil
			case "enter":
				m.attach = m.attach.Remove(m.attach.DeleteIdx())

				if len(m.attach.Items()) == 0 {
					m.attach = m.attach.ToggleDeleteMode()
				}

				return m, nil
			}
			// Fall through to binding resolver (ctrl+r handled there)
		}

		// Handle ctrl+c with double-press: first clears editor, second quits
		if msg.String() == "ctrl+c" {
			return m.handleCtrlC()
		}

		// Handle escape with double-press: first interrupts, second clears editor
		if msg.Code == tea.KeyEsc {
			// If completion is active, dismiss it instead
			if m.editor.CompletionActive() {
				m.editor = m.editor.HideCompletion()

				return m, nil
			}

			return m.handleEscape()
		}

		// Try keybinding resolver
		if action, ok := m.bindings.Resolve(keyString(msg)); ok {
			return m.dispatchBinding(action)
		}

		// Completion key interception
		if handled, model, cmd := m.handleCompletionKey(msg); handled {
			return model, cmd
		}

		// Fall through to editor
		oldValue := m.editor.Value()
		oldLine := m.editor.CursorLine()
		oldCol := m.editor.CursorColumn()

		var cmd tea.Cmd

		m.editor, cmd = m.editor.Update(msg)

		// Refresh completion state when editor content or cursor position changed
		if m.editor.Value() != oldValue || m.editor.CursorLine() != oldLine || m.editor.CursorColumn() != oldCol {
			m = m.refreshEditorCompletion()
		}

		return m, cmd

	case tea.PasteMsg:
		// Paste detection: auto-convert large pastes to file attachments
		if attachments.IsPastedContent(msg.Content) {
			m.attach = m.attach.AddPaste(msg.Content)
			m.showStatus(fmt.Sprintf("Pasted content added as attachment (%d lines)", m.attach.Items()[len(m.attach.Items())-1].Lines))

			return m, m.statusTimer
		}

		// Short paste: forward to editor
		var cmd tea.Cmd

		m.editor, cmd = m.editor.Update(msg)
		m = m.refreshEditorCompletion()

		return m, cmd

	case externalEditorMsg:
		if msg.err != nil {
			m.statusMsg = "Editor error: " + msg.err.Error()
			m.statusGen++
			gen := m.statusGen
			m.statusTimer = tea.Tick(statusMessageTimeout, func(_ time.Time) tea.Msg {
				return statusTimeoutMsg{gen: gen}
			})

			return m, m.statusTimer
		}

		if msg.text != "" {
			m.editor = m.editor.SetValue(msg.text)
		}

		return m, nil

	case components.SubmitMsg:
		return m.onSubmit(msg.Text)

	case components.SpinnerShowMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.SpinnerUpdate(msg)

		return m, cmd

	case components.SpinnerHideMsg:
		m.spinner, _ = m.spinner.SpinnerUpdate(msg)

		return m, nil

	case TurnStartMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.SpinnerUpdate(components.SpinnerShowMsg{})

		return m, cmd

	case MessageStartMsg:
		m.chat = m.chat.AddItem(messages.NewAssistantMessage())

		// Keep render loop active so progressive renders show through
		if m.spinner.Visible() {
			return m, components.StartSpinner()
		}

		return m, nil

	case MessageUpdateMsg:
		m.onMessageUpdate(msg)

		return m, nil

	case MessageEndMsg:
		m.onMessageEnd(msg)

		return m, nil

	case ToolResultMsg:
		m.onToolResult(msg)

		return m, nil

	case TurnEndMsg:
		m.spinner = m.spinner.Hide()

		if !m.chat.AtBottom() {
			m.chat = m.chat.SetTurnEndPending(true)
		}

		return m, nil

	case AgentEndMsg:
		m.spinner = m.spinner.Hide()
		m.footer = m.footer.SetTokenRate(0)

		if msg.Payload != nil {
			if errStr, ok := msg.Payload.(string); ok && errStr != "" {
				am := messages.NewAssistantMessage()
				am.Finalize("[error] " + errStr)
				m.chat = m.chat.AddItem(am)
			}
		}

		return m, nil

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

	case ShutdownMsg:
		return m, tea.Quit

	case reloadMsg:
		if err := handleReload(msg); err != nil {
			return m, func() tea.Msg { return notifyMsg{message: "/reload failed: " + err.Error()} }
		}

		return m, tea.Quit

	case popupPendingMsg:
		return m.handlePopupPending()

	case extStatusMsg:
		m.footer = m.footer.SetExtStatus(msg.key, msg.text)
		return m, nil

	case notifyMsg:
		m.showLanding = false
		m.chat = m.chat.AddItem(newNotifyAssistantMsg(msg.message))

		return m, nil

	case statusTimeoutMsg:
		if msg.gen == m.statusGen {
			m.statusMsg = ""
			m.statusTimer = nil
		}

		return m, nil

	case doublePressTimeoutMsg:
		if msg.gen == m.doublePressGen {
			switch msg.kind {
			case doublePressCtrlC:
				m.ctrlCPressed = false
			case doublePressEscape:
				m.escapePressed = false
			}
		}

		return m, nil

	case ThinkingLevelSetMsg:
		return m.applyThinkingLevel(msg.Level)

	case OutdatedNotificationMsg:
		return m.onOutdatedExtensions(msg)

	case slashCommandsUpdatedMsg:
		if m.editor.CompletionActive() {
			m = m.refreshEditorCompletion()
		}

		return m, nil
	}

	// Forward spinner ticks to advance animation.
	if _, ok := msg.(spinner.TickMsg); ok && m.spinner.Visible() {
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
	}

	return m, nil
}

// handleCtrlC implements double-press ctrl+c: first press clears editor,
// second press within the window quits.
func (m Model) handleCtrlC() (tea.Model, tea.Cmd) {
	if m.editor.Value() != "" {
		m.editor = m.editor.SetValue("")
		m.doublePressGen++
		m.ctrlCPressed = true

		return m, tea.Tick(doublePressWindow, func(_ time.Time) tea.Msg {
			return doublePressTimeoutMsg{kind: doublePressCtrlC, gen: m.doublePressGen}
		})
	}

	// Editor is empty — check for double press
	if m.ctrlCPressed {
		return m, tea.Quit
	}

	m.doublePressGen++
	m.ctrlCPressed = true

	return m, tea.Tick(doublePressWindow, func(_ time.Time) tea.Msg {
		return doublePressTimeoutMsg{kind: doublePressCtrlC, gen: m.doublePressGen}
	})
}

// handleEscape implements double-press escape: first press interrupts streaming,
// second press within the window clears the editor.
func (m Model) handleEscape() (tea.Model, tea.Cmd) {
	// Check for double press — clear editor
	if m.escapePressed {
		m.editor = m.editor.SetValue("")
		m.escapePressed = false

		return m, nil
	}

	m.doublePressGen++
	m.escapePressed = true

	// First press — interrupt streaming if active, start timeout
	model, cmd := m.interruptStreaming()

	return model, tea.Batch(cmd, tea.Tick(doublePressWindow, func(_ time.Time) tea.Msg {
		return doublePressTimeoutMsg{kind: doublePressEscape, gen: m.doublePressGen}
	}))
}

// dispatchBinding handles a resolved keybinding action.
//
//nolint:gocyclo // central keybinding dispatch
func (m Model) dispatchBinding(action BindingAction) (tea.Model, tea.Cmd) {
	switch action {
	case ActionExit:
		return m, tea.Quit
	case ActionModelSelect:
		return m, listModelsCmd(m.cfg)
	case ActionModelCycle:
		models := listModels(m.cfg)
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
	case ActionScrollToBottom:
		m.chat = m.chat.JumpToBottom()
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
		m.showLanding = true
		m.attach = m.attach.Clear()

		return m, nil

	// Attachments
	case ActionAttachDelete:
		if m.attach.InDeleteMode() {
			// ctrl+r while in delete mode: delete highlighted attachment
			m.attach = m.attach.Remove(m.attach.DeleteIdx())
		} else {
			m.attach = m.attach.ToggleDeleteMode()
		}

		return m, nil

	// Sandbox
	case ActionSandboxCycle:
		return m.cycleSandboxMode()

	default:
		return m, nil
	}
}

// toggleLastToolOutput expands or collapses the last tool output panel or skill message.
func (m *Model) toggleLastToolOutput() {
	items := m.chat.Items()
	for i, item := range slices.Backward(items) {
		if tp, ok := item.(*messages.ToolPanel); ok {
			tp.ToggleExpanded()
			m.chat = m.chat.UpdateItemByID(tp)

			return
		}

		if um, ok := item.(*messages.UserMessage); ok && um.IsSkillInvocation() {
			um.ToggleExpanded()
			m.chat = m.chat.UpdateItemAt(i, um)

			return
		}
	}
}

// toggleLastThinkingBlock expands or collapses the last thinking block.
func (m *Model) toggleLastThinkingBlock() {
	items := m.chat.Items()
	for _, item := range slices.Backward(items) {
		if tb, ok := item.(*messages.ThinkingBlock); ok {
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
		m.statusMsg = "Failed to create temp file: " + err.Error()
		m.statusGen++
		gen := m.statusGen
		m.statusTimer = tea.Tick(statusMessageTimeout, func(_ time.Time) tea.Msg {
			return statusTimeoutMsg{gen: gen}
		})

		return m, m.statusTimer
	}

	_ = tmpFile.Chmod(0o600)

	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(text); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)

		m.statusMsg = "Failed to write temp file: " + err.Error()
		m.statusGen++
		gen := m.statusGen
		m.statusTimer = tea.Tick(statusMessageTimeout, func(_ time.Time) tea.Msg {
			return statusTimeoutMsg{gen: gen}
		})

		return m, m.statusTimer
	}

	_ = tmpFile.Close()

	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}

	if editor == "" {
		editor = "vi"
	}

	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], tmpPath)...) //nolint:gosec,noctx // editor path comes from env

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

// findStreamingAssistant scans chat items backwards for the active streaming assistant message.
// Returns the message and its index, or nil and -1 if not found.
func (m *Model) findStreamingAssistant() (*messages.AssistantMessage, int) {
	for i, item := range slices.Backward(m.chat.Items()) {
		if am, ok := item.(*messages.AssistantMessage); ok && am.IsStreaming() {
			return am, i
		}
	}

	return nil, -1
}

// onMessageUpdate appends a delta to the current assistant message and updates token rate.
func (m *Model) onMessageUpdate(msg MessageUpdateMsg) {
	if msg.TokenRate > 0 {
		m.footer = m.footer.SetTokenRate(msg.TokenRate)
	}

	am, idx := m.findStreamingAssistant()
	if am == nil {
		return
	}

	am.Append(msg.Content)
	m.chat = m.chat.UpdateItemAt(idx, am)
}

// onMessageEnd finalizes the current assistant message and creates pending tool panels.
func (m *Model) onMessageEnd(msg MessageEndMsg) {
	m.footer = m.footer.SetTokenRate(0)

	am, idx := m.findStreamingAssistant()
	if am == nil {
		return
	}

	am.Finalize(msg.Content)
	m.chat = m.chat.UpdateItemAt(idx, am)

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
	am, idx := m.findStreamingAssistant()
	if am == nil {
		return m, nil
	}

	am.Interrupt()
	m.chat = m.chat.UpdateItemAt(idx, am)
	m.spinner = m.spinner.Hide()
	m.footer = m.footer.SetTokenRate(0)

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

// onOutdatedExtensions renders a notification banner for outdated extensions.
func (m Model) onOutdatedExtensions(msg OutdatedNotificationMsg) (tea.Model, tea.Cmd) {
	if len(msg.Extensions) == 0 {
		return m, nil
	}

	names := make([]string, len(msg.Extensions))
	for i, ext := range msg.Extensions {
		names[i] = ext.Name
	}

	text := formatOutdatedBanner(names)

	m.showLanding = false
	m.chat = m.chat.AddItem(newNotifyAssistantMsg(text))

	return m, nil
}

// formatOutdatedBanner formats outdated extension names into a notification message.
func formatOutdatedBanner(names []string) string {
	hint := "Run `weave update` to update all, or `weave update <name>`"

	if len(names) == 1 {
		return fmt.Sprintf("Extension Updates Available\n%s has a newer version available.\n%s", names[0], hint)
	}

	nameList := strings.Join(names, ", ")

	return fmt.Sprintf("Extension Updates Available\n%s have newer versions available.\n%s", nameList, hint)
}

// onSubmit handles editor submit — routes slash commands or publishes prompt/followup.
func (m Model) onSubmit(text string) (tea.Model, tea.Cmd) {
	// Reject empty submissions without attachments.
	if text == "" && len(m.attach.Items()) == 0 {
		return m, nil
	}

	// Try slash command dispatch first.
	if handled, result := m.commands.Dispatch(text); handled { //nolint:nestif // command dispatch has multiple optional outcomes
		cmdName, cmdArgs := parseCommand(text)
		if skillName, ok := strings.CutPrefix(cmdName, "/skill:"); ok {
			xmlContent := fmt.Sprintf("<skill name=%q>\n</skill>", skillName)

			if cmdArgs != "" {
				xmlContent += "\n\n" + cmdArgs
			}

			m.chat = m.chat.AddItem(messages.NewUserMessage(xmlContent))
			m.prompted = true
			m.showLanding = false
		}

		if result.Quit {
			return m, tea.Quit
		}

		if result.ClearChat {
			m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
			m.toolPanels = make(map[string]*messages.ToolPanel)
			m.attach = m.attach.Clear()
			m.showLanding = true
		}

		if result.ResetPrompt {
			m.prompted = false
		}

		if result.Notify != "" {
			m.showLanding = false

			m.chat = m.chat.AddItem(messages.NewAssistantMessage())

			items := m.chat.Items()
			if am, ok := items[len(items)-1].(*messages.AssistantMessage); ok {
				am.Finalize(result.Notify)
				m.chat = m.chat.UpdateItem(am)
			}
		}

		return m, result.Command
	}

	// Merge attachments into prompt text and clear them
	promptText := m.attach.RenderPrompt(text)
	m.attach = m.attach.Clear()

	m.AddUserMessage(promptText)

	if !m.prompted {
		m.prompted = true
		m.showLanding = false

		if m.bus != nil {
			m.bus.Publish(sdk.NewEvent(topicPrompt, promptText))
		}

		if !m.spinner.Visible() {
			var tickCmd tea.Cmd

			m.spinner, tickCmd = m.spinner.SpinnerUpdate(components.SpinnerShowMsg{})

			return m, tickCmd
		}

		return m, nil
	}

	if m.bus != nil {
		m.bus.Publish(sdk.NewEvent(topicFollowup, promptText))
	}

	if !m.spinner.Visible() {
		var tickCmd tea.Cmd

		m.spinner, tickCmd = m.spinner.SpinnerUpdate(components.SpinnerShowMsg{})

		return m, tickCmd
	}

	return m, nil
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
	sel := overlays.NewSelectorModel("Resume Session", items)
	sel = sel.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogSessionSelect, sel))

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
	sel := overlays.NewSelectorModel("Select Model", items)
	sel = sel.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogModelSelect, sel))

	return m, nil
}

func (m Model) onModelChanged(msg ModelChangedMsg) (tea.Model, tea.Cmd) {
	m.prevModel = m.currentModel
	m.prevThinkingLevel = m.thinkingLevel
	m.currentModel = msg.Entry
	m.footer = m.footer.SetModel(msg.Entry.Model, msg.Entry.Provider)
	m.footer = m.footer.SetReasoning(modelReasoning(msg.Entry.Model))

	thinkingChanged := false

	if modelDef, ok := sdkmodel.GetModel(msg.Entry.Model); ok {
		if !modelDef.Reasoning {
			thinkingChanged = m.thinkingLevel != sdkmodel.ThinkingOff
			m.thinkingLevel = sdkmodel.ThinkingOff
			m.footer = m.footer.SetThinkingLevel(string(sdkmodel.ThinkingOff))
			m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(sdkmodel.ThinkingOff))
		} else if clamped := sdkmodel.ClampForModel(m.thinkingLevel, modelDef); clamped != m.thinkingLevel {
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

		if m.cfg != nil {
			cmds = append(cmds, saveSettingsCmd(m.cfg, m.currentModel, m.thinkingLevel))
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
	sel := overlays.NewSelectorModel("Manage Provider Keys", items)
	sel = sel.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogProviderSelect, sel))

	return m, nil
}

func (m Model) onProviderDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if result.Err != nil || result.Index < 0 || result.Index >= len(m.pendingProviders) {
		m.pendingProviders = nil
		return m, pendingCmd
	}

	selected := m.pendingProviders[result.Index]
	m.providerTarget = selected.Name

	// Push key input dialog for entering the API key.
	input := overlays.NewInputModel("Enter API key for " + selected.Name)
	input = input.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewInputDialog(dialogKeyInput, input))

	return m, pendingCmd
}

func (m Model) onKeyInputDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.pendingProviders = nil
	providerName := m.providerTarget
	m.providerTarget = ""

	if result.Err != nil || providerName == "" {
		return m, pendingCmd
	}

	apiKey := strings.TrimSpace(result.Value)
	if apiKey == "" {
		return m, pendingCmd
	}

	if m.cfg == nil {
		am := messages.NewAssistantMessage()
		am.Finalize("No config available to save API key.")
		m.chat = m.chat.AddItem(am)

		return m, pendingCmd
	}

	err := auth.SetProviderKey(providerName, apiKey)

	am := messages.NewAssistantMessage()
	if err != nil {
		am.Finalize(fmt.Sprintf("Failed to save API key for %s: %v", providerName, err))
		m.chat = m.chat.AddItem(am)

		return m, pendingCmd
	}

	am.Finalize(fmt.Sprintf("API key saved for %s.", providerName))
	m.chat = m.chat.AddItem(am)

	// If we were in noConfigured state, re-evaluate now that a key exists.
	if m.noConfigured {
		models := listModels(m.cfg)
		if len(models) > 0 {
			m.noConfigured = false
			cur := currentModel(models, m.cfg)
			m.currentModel = cur
			m.footer = m.footer.SetModel(cur.Model, cur.Provider)
			m.footer = m.footer.SetReasoning(modelReasoning(cur.Model))
		}
	}

	return m, pendingCmd
}

// handleDialogDone processes a completed dialog from the stack.
func (m Model) handleDialogDone(d overlays.Dialog, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	id := d.ID()
	result := d.Result()

	switch id {
	case dialogSessionSelect:
		return m.onSessionDialogDone(result, pendingCmd)
	case dialogModelSelect:
		return m.onModelDialogDone(result, pendingCmd)
	case dialogProviderSelect:
		return m.onProviderDialogDone(result, pendingCmd)
	case dialogKeyInput:
		return m.onKeyInputDialogDone(result, pendingCmd)
	default:
		// Popup dialogs: send result on channel.
		if ch, ok := m.popupChans[id]; ok {
			resp := overlayResponse{
				index:     result.Index,
				value:     result.Value,
				confirmed: result.Confirmed,
				err:       result.Err,
			}
			ch <- resp

			delete(m.popupChans, id)
		}

		return m, tea.Batch(pendingCmd, checkNextPopupCmd(m.ui))
	}
}

// handleDialogForceCancel handles ctrl+c dismissal of the top dialog.
func (m Model) handleDialogForceCancel(d overlays.Dialog) (tea.Model, tea.Cmd) {
	id := d.ID()

	// Clean up pending data based on dialog purpose.
	switch id {
	case dialogSessionSelect:
		m.pendingSessions = nil
	case dialogModelSelect:
		m.pendingModels = nil
	case dialogProviderSelect:
		m.pendingProviders = nil
		m.providerTarget = ""
	case dialogKeyInput:
		m.pendingProviders = nil
		m.providerTarget = ""
	default:
		// Popup dialog cancellation.
		if ch, ok := m.popupChans[id]; ok {
			ch <- overlayResponse{err: errors.New("canceled")}

			delete(m.popupChans, id)
		}
	}

	return m, checkNextPopupCmd(m.ui)
}

func (m Model) onSessionDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if result.Err != nil || result.Index < 0 || result.Index >= len(m.pendingSessions) {
		m.pendingSessions = nil
		return m, pendingCmd
	}

	session := m.pendingSessions[result.Index]
	m.pendingSessions = nil

	m.rebuildChatFromSession(session.ID)
	m.showLanding = false
	m.prompted = false

	if m.bus != nil {
		return m, tea.Batch(pendingCmd, PublishSessionResume(m.bus, session.ID))
	}

	return m, pendingCmd
}

func (m Model) onModelDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if result.Err != nil || result.Index < 0 || result.Index >= len(m.pendingModels) {
		m.pendingModels = nil
		return m, pendingCmd
	}

	selected := m.pendingModels[result.Index]
	m.pendingModels = nil

	model, cmd := m.onModelChanged(ModelChangedMsg{Entry: selected})

	return model, tea.Batch(pendingCmd, cmd)
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
	if modelDef, ok := sdkmodel.GetModel(m.currentModel.Model); ok && modelDef.Reasoning {
		cur = sdkmodel.ClampForModel(cur, modelDef)
	}

	for i, lvl := range sdkmodel.AllThinkingLevels {
		if lvl != cur {
			continue
		}

		for j := 1; j <= len(sdkmodel.AllThinkingLevels); j++ {
			candidate := sdkmodel.AllThinkingLevels[(i+j)%len(sdkmodel.AllThinkingLevels)]

			var effective sdkmodel.ThinkingLevel

			if modelDef, ok := sdkmodel.GetModel(m.currentModel.Model); ok {
				if modelDef.Reasoning {
					effective = sdkmodel.ClampForModel(candidate, modelDef)
				} else {
					effective = sdkmodel.ThinkingOff
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
func (m Model) applyThinkingLevel(level sdkmodel.ThinkingLevel) (tea.Model, tea.Cmd) {
	if modelDef, ok := sdkmodel.GetModel(m.currentModel.Model); ok {
		if !modelDef.Reasoning {
			level = sdkmodel.ThinkingOff
		} else {
			level = sdkmodel.ClampForModel(level, modelDef)
		}
	}

	m.thinkingLevel = level
	m.footer = m.footer.SetThinkingLevel(string(level))
	m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(level))

	m.showStatus(fmt.Sprintf("Thinking level: %s", level))

	var cmds []tea.Cmd

	if m.bus != nil {
		cmds = append(cmds, PublishThinkingChange(m.bus, level))

		if m.cfg != nil {
			cmds = append(cmds, saveSettingsCmd(m.cfg, m.currentModel, level))
		}
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

// cycleSandboxMode advances the sandbox to the next mode in the cycle order,
// updates the footer status pill.
func (m Model) cycleSandboxMode() (tea.Model, tea.Cmd) {
	sb := sdk.GetSandboxer()
	if sb == nil {
		m.showStatus("Sandbox: not available")
		return m, m.statusTimer
	}

	current := sb.Mode()
	next := sdk.NextSandboxMode(current)
	sb.SetMode(next)

	if m.bus != nil {
		m.bus.Publish(sdk.NewEvent("sandbox.mode.change", next))
	}

	m.footer = m.footer.SetExtStatus("sandbox", "SB:"+next)

	m.showStatus("Sandbox mode: " + next)

	return m, m.statusTimer
}

// handleCompletionKey processes keys when the completion popup is active.
// Returns true if the key was handled (intercepted for completion navigation).
func (m Model) handleCompletionKey(msg tea.KeyPressMsg) (bool, Model, tea.Cmd) {
	if !m.editor.CompletionActive() {
		return false, m, nil
	}

	switch msg.Code {
	case tea.KeyTab, tea.KeyUp, tea.KeyDown, tea.KeyEnter:
		var cmd tea.Cmd

		m.editor, cmd = m.editor.Update(msg)

		return true, m, cmd
	}

	return false, m, nil
}

// refreshEditorCompletion reads the editor value and shows/hides the
// completion popup based on the current context. Uses the cursor's current
// line for multiline-aware completion.
func (m Model) refreshEditorCompletion() Model {
	value := m.editor.Value()

	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		m.editor = m.editor.HideCompletion()

		return m
	}

	lineIdx := m.editor.CursorLine()
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}

	if lineIdx < 0 {
		lineIdx = 0
	}

	line := lines[lineIdx]

	lineStart := 0
	for i := range lineIdx {
		lineStart += len(lines[i]) + 1
	}

	if lineIdx == 0 && strings.HasPrefix(line, "/") {
		return m.slashCommandCompletion(line, lineStart)
	}

	return m.atFileCompletion(line, lineStart)
}

// slashCommandCompletion handles "/" command and file completions at the start of a line.
func (m Model) slashCommandCompletion(line string, lineStart int) Model {
	cmdName, afterSpace, hasSpace := strings.Cut(line, " ")
	if !hasSpace {
		filter := strings.TrimPrefix(line, "/")
		names := m.commands.Names()

		items := make([]components.CompletionItem, 0, len(names))
		for _, name := range names {
			info, ok := m.commands.Lookup(name)
			if !ok {
				continue
			}

			items = append(items, components.CompletionItem{
				Label:       strings.TrimPrefix(name, "/"),
				Description: info.Description,
				Value:       name + " ",
			})
		}

		comp := m.editor.ShowCompletion(components.CompletionSlash, items, filter, lineStart)
		if comp.Completion().FilteredCount() == 0 {
			m.editor = m.editor.HideCompletion()
		} else {
			m.editor = comp
		}

		return m
	}

	if info, ok := m.commands.Lookup(cmdName); ok && info.AcceptsFiles {
		items := components.PathCompletions(".", afterSpace)
		triggerOffset := lineStart + len(cmdName) + 1
		// PathCompletions already filters by prefix, pass empty filter to avoid
		// double-filtering: SetFilter checks HasPrefix(Label, filter) but Label
		// is just the filename (e.g. "main.go"), not the full path.
		comp := m.editor.ShowCompletion(components.CompletionFile, items, "", triggerOffset)
		if comp.Completion().FilteredCount() == 0 {
			m.editor = m.editor.HideCompletion()
		} else {
			m.editor = comp
		}

		return m
	}

	m.editor = m.editor.HideCompletion()

	return m
}

// atFileCompletion handles "@" file path completions after whitespace.
func (m Model) atFileCompletion(line string, lineStart int) Model {
	atIdx := strings.LastIndex(line, "@")
	if atIdx < 0 || (atIdx > 0 && !isWhitespace(line[atIdx-1])) {
		m.editor = m.editor.HideCompletion()

		return m
	}

	cursorCol := m.editor.CursorColumn()
	atRunePos := len([]rune(line[:atIdx]))

	if cursorCol <= atRunePos {
		m.editor = m.editor.HideCompletion()

		return m
	}

	tokenLen := cursorCol - atRunePos - 1

	afterAt := []rune(line[atIdx+1:])
	if tokenLen > len(afterAt) {
		tokenLen = len(afterAt)
	}

	token := string(afterAt[:tokenLen])
	if strings.Contains(token, " ") {
		m.editor = m.editor.HideCompletion()

		return m
	}

	filter := token

	items := components.PathCompletions(".", filter)
	triggerOffset := lineStart + atIdx
	// PathCompletions already filters by prefix, pass empty filter to avoid
	// double-filtering: SetFilter checks HasPrefix(Label, filter) but Label
	// is just the filename (e.g. "main.go"), not the full path.
	comp := m.editor.ShowCompletion(components.CompletionFile, items, "", triggerOffset)
	if comp.Completion().FilteredCount() == 0 {
		m.editor = m.editor.HideCompletion()
	} else {
		m.editor = comp
	}

	return m
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t'
}

// chatHeight returns the height allocated to the chat area.
func (m Model) chatHeight(totalHeight int) int {
	headerRows := 0
	if !m.showLanding && m.showHints && !m.prompted && len(m.chat.Items()) == 0 {
		headerRows = 1
	}

	pillRows := 0
	if m.spinner.Visible() {
		pillRows++
	}

	if m.statusMsg != "" {
		pillRows++
	}

	editorH := m.editor.Height() + m.attach.Height()
	lt := m.layout.ComputeFull(m.width, totalHeight, editorH, headerRows, pillRows)

	return max(lt.Main.Dy(), 1)
}

// Draw renders the TUI into an ultraviolet screen buffer.
// It computes layout regions and delegates to each component.
func (m Model) Draw(scr uv.Screen, area uv.Rectangle) {
	// Dialog stack takes highest priority — renders all dialogs bottom-to-top.
	if !m.dialogStack.Empty() {
		m.dialogStack.Draw(scr, area)

		return
	}

	// Compute dynamic layout parameters
	headerRows := 0
	if !m.showLanding && m.showHints && !m.prompted && len(m.chat.Items()) == 0 {
		headerRows = 1
	}

	pillRows := 0
	if m.spinner.Visible() {
		pillRows++
	}

	if m.statusMsg != "" {
		pillRows++
	}

	// Compute layout to determine actual component sizes
	editorH := m.editor.Height() + m.attach.Height()
	lt := m.layout.ComputeFull(
		area.Dx(), area.Dy(),
		editorH, headerRows, pillRows,
	)

	// Sync chat size to allocated main area so it renders only visible content
	if lt.Main.Dy() > 0 {
		m.chat = m.chat.SetSize(lt.Main.Dx(), lt.Main.Dy())
	}

	// Render header
	if headerRows > 0 {
		hintsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		uv.NewStyledString(hintsStyle.Render(
			"ctrl+p model · ctrl+l select · shift+tab thinking · ctrl+t toggle",
		)).Draw(scr, lt.Header)
	}

	// Render main (chat or landing)
	if m.showLanding {
		m.landing = m.landing.SetSize(lt.Main.Dx(), lt.Main.Dy())
		m.landing.Draw(scr, lt.Main)
	} else {
		m.chat.Draw(scr, lt.Main)
	}

	// Render pills (spinner + status)
	if pillRows > 0 && lt.Pills.Dy() > 0 {
		y := lt.Pills.Min.Y

		if m.spinner.Visible() {
			spArea := uv.Rect(lt.Pills.Min.X, y, lt.Pills.Dx(), 1)
			m.spinner.Draw(scr, spArea)

			y++
		}

		if m.statusMsg != "" {
			statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
			stArea := uv.Rect(lt.Pills.Min.X, y, lt.Pills.Dx(), 1)
			uv.NewStyledString(statusStyle.Render(m.statusMsg)).Draw(scr, stArea)
		}
	}

	// Render attachments + editor
	attachH := m.attach.Height()

	var editorArea uv.Rectangle

	if attachH > 0 && lt.Editor.Dy() > attachH {
		attachArea := uv.Rect(lt.Editor.Min.X, lt.Editor.Min.Y, lt.Editor.Dx(), attachH)
		editorArea = uv.Rect(lt.Editor.Min.X, lt.Editor.Min.Y+attachH, lt.Editor.Dx(), lt.Editor.Dy()-attachH)

		m.attach.Draw(scr, attachArea)
		m.editor.Draw(scr, editorArea)
	} else {
		editorArea = lt.Editor
		m.editor.Draw(scr, editorArea)
	}

	// Render completion popup above cursor
	if m.editor.CompletionActive() {
		comp := m.editor.Completion()
		if comp.FilteredCount() > 0 {
			m.drawCompletionPopup(scr, editorArea)
		}
	}

	// Render footer
	m.footer.Draw(scr, lt.Footer)
}

// drawCompletionPopup renders the completion popup positioned relative to the
// editor cursor. It renders above the cursor when there's enough space,
// otherwise below.
func (m Model) drawCompletionPopup(scr uv.Screen, editorArea uv.Rectangle) {
	comp := m.editor.Completion()

	popupW := min(50, editorArea.Dx())
	visibleItems := min(comp.FilteredCount(), 8) // maxVisible default is 8
	popupH := visibleItems + 2                   // content rows + top/bottom border

	// Cursor position within editor content area.
	// Account for left border (1) + left padding (1).
	cursorX := editorArea.Min.X + 2 + m.editor.CursorColumn()
	cursorY := editorArea.Min.Y + 1 + m.editor.VisualCursorLine()

	// Default: render above cursor
	popupX := cursorX
	popupY := cursorY - popupH

	// If not enough space above, render below
	if popupY < 0 {
		popupY = cursorY + 1
	}

	// Clamp to screen bottom
	if popupY+popupH > m.height {
		popupH = m.height - popupY
	}

	// Clamp X so popup doesn't overflow right edge
	if popupX+popupW > m.width {
		popupX = m.width - popupW
	}

	if popupX < 0 {
		popupX = 0
	}

	if popupH <= 0 || popupW < 4 {
		return
	}

	// Sync popup width so View() renders at the correct size
	comp = comp.SetWidth(popupW)

	popupArea := uv.Rect(popupX, popupY, popupW, popupH)
	comp.Draw(scr, popupArea)
}

// View renders the TUI using ultraviolet screen buffers.
func (m Model) View() tea.View {
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	v := tea.NewView(uv.TrimSpace(canvas.Render()))
	v.AltScreen = true

	return v
}
