package tui

import (
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
)

// Model is the root Bubble Tea model for the TUI.
type Model struct {
	width  int
	height int
	bus    sdk.Bus
	cfg    sdk.Config
}

// newModel creates a new root model.
func newModel(bus sdk.Bus, cfg sdk.Config) Model {
	return Model{
		width:  80,
		height: 24,
		bus:    bus,
		cfg:    cfg,
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
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "ctrl+d" {
			return m, tea.Quit
		}
	}

	return m, nil
}

// View renders the TUI.
func (m Model) View() string {
	return "weave tui"
}
