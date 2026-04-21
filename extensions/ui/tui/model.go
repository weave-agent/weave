package tui

import (
	"fmt"
	"strings"

	"weave/ext/ui/tui/components"
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlaySession
	overlayModel
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

	overlay         overlays.SelectorModel
	activeOverlay   overlayKind
	pendingSessions []SessionEntry
	pendingModels   []ModelEntry
	currentModel    ModelEntry

	popup *popupState
}

// newModel creates a new root model.
func newModel(bus sdk.Bus, cfg sdk.Config) Model {
	commands := NewCommandRegistry(bus)
	commands.register("/model", "Select or change model", func(_ string) CommandResult {
		return CommandResult{Command: listModelsCmd()}
	})

	editor := components.NewEditorModel().Focus()
	editor.SetSlashCommands(commands.Names())

	models := listModels()
	cur := currentModel(models)

	bindings := NewBindingRegistry()

	if cfg != nil && cfg.FilePath() != "" {
		if kbPath := loadKeybindings(cfg.FilePath()); kbPath != "" {
			_ = bindings.LoadUserConfig(kbPath)
		}
	}

	ui := NewTUIImpl(commands, bindings)

	m := Model{
		width:        80,
		height:       24,
		bus:          bus,
		cfg:          cfg,
		chat:         components.NewChatModel(),
		editor:       editor,
		footer:       components.NewFooterModel(),
		spinner:      components.NewSpinnerModel(),
		toolPanels:   make(map[string]*messages.ToolPanel),
		commands:     commands,
		bindings:     bindings,
		ui:           ui,
		currentModel: cur,
	}
	m.footer = m.footer.SetModel(cur.Model, cur.Provider)
	return m
}

// Init returns the initial command (none for now).
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
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
		m.editor = m.editor.SetSize(m.width, 3)
		m.footer = m.footer.SetSize(m.width)
		m.spinner = m.spinner.SetSize(m.width)
		m.overlay = m.overlay.SetSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		// Overlay gets priority when active (ctrl+c dismisses it)
		if m.activeOverlay != overlayNone {
			if msg.String() == "ctrl+c" {
				m.activeOverlay = overlayNone
				m.overlay = m.overlay.Hide()
				return m, nil
			}

			var cmd tea.Cmd
			m.overlay, cmd = m.overlay.Update(msg)
			return m, cmd
		}

		// Try keybinding resolver
		if action, ok := m.bindings.Resolve(keyString(msg)); ok {
			return m.dispatchBinding(action)
		}

		// Fall through to editor
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd

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
				am.Finalize(fmt.Sprintf("⚠ %s", errStr))
				m.chat = m.chat.AddItem(am)
			}
		}

	case SessionListResultMsg:
		return m.onSessionListResult(msg)

	case ModelListResultMsg:
		return m.onModelListResult(msg)

	case ModelChangedMsg:
		return m.onModelChanged(msg)

	case overlays.SelectorSelectedMsg:
		return m.onOverlaySelected(msg)

	case overlays.SelectorCancelledMsg:
		m.activeOverlay = overlayNone
		m.overlay = m.overlay.Hide()
		m.pendingSessions = nil
		m.pendingModels = nil
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
		return m, nil
	}

	// Forward spinner ticks
	if m.spinner.Visible() {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// dispatchBinding handles a resolved keybinding action.
func (m Model) dispatchBinding(action BindingAction) (tea.Model, tea.Cmd) {
	switch action {
	case ActionExit:
		return m, tea.Quit
	case ActionClear:
		return m, tea.Quit
	case ActionInterrupt:
		return m.interruptStreaming()
	case ActionModelSelect:
		return m, listModelsCmd()
	case ActionModelCycle:
		models := listModels()
		if len(models) > 1 {
			next := cycleModel(models, m.currentModel)
			return m, func() tea.Msg { return ModelChangedMsg{Entry: next} }
		}
		return m, nil
	case ActionToolExpand, ActionThinkToggle:
		// Placeholder: no-op for now, will be wired in task 17
		return m, nil
	default:
		return m, nil
	}
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

	am.Finalize(msg.Content)
	m.chat = m.chat.UpdateItem(am)

	if msg.Thinking != "" {
		m.chat = m.chat.AddItem(messages.NewThinkingBlock(msg.Thinking))
	}

	for _, tc := range msg.ToolCalls {
		args := fmt.Sprintf("%v", tc.Arguments)
		panel := messages.NewToolPanel(tc.ID, tc.Name, args)
		panel.SetDiffRenderer(messages.NewDiffRenderer())
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
	if handled, result := m.commands.Dispatch(text); handled {
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
		am.Finalize(fmt.Sprintf("Only one model available: %s", msg.Models[0].Display()))
		m.chat = m.chat.AddItem(am)
		return m, nil
	}

	items := make([]overlays.SelectorItem, len(msg.Models))
	for i, model := range msg.Models {
		items[i] = overlays.SelectorItem{
			Title:    model.Display(),
			Subtitle: model.Provider,
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
	m.currentModel = msg.Entry
	m.footer = m.footer.SetModel(msg.Entry.Model, msg.Entry.Provider)

	if m.bus != nil {
		return m, PublishModelChange(m.bus, msg.Entry)
	}

	return m, nil
}

func (m Model) onOverlaySelected(msg overlays.SelectorSelectedMsg) (tea.Model, tea.Cmd) {
	switch m.activeOverlay {
	case overlaySession:
		return m.onSessionSelected(msg)
	case overlayModel:
		return m.onModelSelected(msg)
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
	entries, err := loadSessionEntries(sessionID)
	if err != nil {
		m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("Error loading session: %v", err))
		m.chat = m.chat.AddItem(am)
		return
	}

	m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
	m.toolPanels = make(map[string]*messages.ToolPanel)
	m.prompted = true

	for _, entry := range entries {
		switch entry.Role {
		case sdk.RoleUser:
			m.chat = m.chat.AddItem(messages.NewUserMessage(entry.Content))
		case sdk.RoleAssistant:
			am := messages.NewAssistantMessage()
			am.Finalize(entry.Content)
			m.chat = m.chat.AddItem(am)
		}
	}
}

// chatHeight returns the height allocated to the chat area.
func (m Model) chatHeight(totalHeight int) int {
	editorLines := 3 + 2 // editor height + border
	footerLines := 2
	spinnerLines := 0
	if m.spinner.Visible() {
		spinnerLines = 1
	}

	reserved := editorLines + footerLines + spinnerLines
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

	if m.activeOverlay != overlayNone && m.overlay.Visible() {
		return m.overlay.View()
	}

	var sections []string

	sections = append(sections, m.chat.View())

	if m.spinner.Visible() {
		sections = append(sections, m.spinner.View())
	}

	sections = append(sections, m.editor.View())
	sections = append(sections, m.footer.View())

	return strings.Join(sections, "\n")
}
