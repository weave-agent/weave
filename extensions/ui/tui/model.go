package tui

import (
	"fmt"
	"strings"

	"weave/ext/ui/tui/components"
	"weave/ext/ui/tui/components/messages"
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
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
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "ctrl+d" {
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd

	case components.SubmitMsg:
		return m.onSubmit(msg.Text)

	case TurnStartMsg:
		// New turn starting — show spinner
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
		// Turn ended — refresh git branch and hide spinner
		m.spinner = m.spinner.Hide()

	case AgentEndMsg:
		// Agent finished — hide spinner
		m.spinner = m.spinner.Hide()

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
	var sections []string

	sections = append(sections, m.chat.View())

	if m.spinner.Visible() {
		sections = append(sections, m.spinner.View())
	}

	sections = append(sections, m.editor.View())
	sections = append(sections, m.footer.View())

	return strings.Join(sections, "\n")
}
