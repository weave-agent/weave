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

	overlay       overlays.SelectorModel
	activeOverlay overlayKind
	pendingSessions []SessionEntry
}

// newModel creates a new root model.
func newModel(bus sdk.Bus, cfg sdk.Config) Model {
	commands := NewCommandRegistry(bus)

	editor := components.NewEditorModel().Focus()
	editor.SetSlashCommands(commands.Names())

	return Model{
		width:      80,
		height:     24,
		bus:        bus,
		cfg:        cfg,
		chat:       components.NewChatModel(),
		editor:     editor,
		footer:     components.NewFooterModel(),
		spinner:    components.NewSpinnerModel(),
		toolPanels: make(map[string]*messages.ToolPanel),
		commands:   commands,
	}
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
		if msg.String() == "ctrl+c" {
			if m.activeOverlay != overlayNone {
				m.activeOverlay = overlayNone
				m.overlay = m.overlay.Hide()
				return m, nil
			}

			return m, tea.Quit
		}

		if msg.String() == "ctrl+d" {
			return m, tea.Quit
		}

		if m.activeOverlay != overlayNone {
			var cmd tea.Cmd
			m.overlay, cmd = m.overlay.Update(msg)
			return m, cmd
		}

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

	case SessionListResultMsg:
		return m.onSessionListResult(msg)

	case overlays.SelectorSelectedMsg:
		return m.onOverlaySelected(msg)

	case overlays.SelectorCancelledMsg:
		m.activeOverlay = overlayNone
		m.overlay = m.overlay.Hide()
		m.pendingSessions = nil
		return m, nil

	case ShutdownMsg:
		return m, tea.Quit
	}

	// Forward spinner ticks
	if m.spinner.Visible() {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
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

func (m Model) onOverlaySelected(msg overlays.SelectorSelectedMsg) (tea.Model, tea.Cmd) {
	switch m.activeOverlay {
	case overlaySession:
		return m.onSessionSelected(msg)
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
