package tui

import (
	"fmt"

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

	chat      components.ChatModel
	prompted  bool
	toolPanels map[string]*messages.ToolPanel // track pending tool panels by ID
}

// newModel creates a new root model.
func newModel(bus sdk.Bus, cfg sdk.Config) Model {
	return Model{
		width:      80,
		height:     24,
		bus:        bus,
		cfg:        cfg,
		chat:       components.NewChatModel(),
		toolPanels: make(map[string]*messages.ToolPanel),
	}
}

// Init returns the initial command (none for now).
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chat = m.chat.SetSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "ctrl+d" {
			return m, tea.Quit
		}

	case TurnStartMsg:
		// New turn starting

	case MessageStartMsg:
		m.chat = m.chat.AddItem(messages.NewAssistantMessage())

	case MessageUpdateMsg:
		m.onMessageUpdate(msg.Content)

	case MessageEndMsg:
		m.onMessageEnd(msg)

	case ToolResultMsg:
		m.onToolResult(msg)

	case AgentEndMsg:
		// Agent finished

	case ShutdownMsg:
		return m, tea.Quit
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

	for _, tc := range msg.ToolCalls {
		args := fmt.Sprintf("%v", tc.Arguments)
		panel := messages.NewToolPanel(tc.ID, tc.Name, args)
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

// View renders the TUI.
func (m Model) View() string {
	return m.chat.View()
}
