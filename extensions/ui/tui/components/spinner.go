package components

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SpinnerCharSet is the set of frames used by the spinner.
var SpinnerCharSet = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// tickMsg is sent on each spinner interval.
type tickMsg time.Time

// SpinnerModel displays an animated spinner with a "Thinking..." label during streaming.
type SpinnerModel struct {
	visible bool
	frame   int
	label   string
	width   int
}

// NewSpinnerModel creates a new spinner model.
func NewSpinnerModel() SpinnerModel {
	return SpinnerModel{
		label: "Thinking...",
	}
}

// SetSize updates the spinner width.
func (m SpinnerModel) SetSize(width int) SpinnerModel {
	m.width = width
	return m
}

// Show makes the spinner visible.
func (m SpinnerModel) Show() SpinnerModel {
	m.visible = true
	return m
}

// Hide hides the spinner.
func (m SpinnerModel) Hide() SpinnerModel {
	m.visible = false
	m.frame = 0
	return m
}

// Visible returns whether the spinner is currently shown.
func (m SpinnerModel) Visible() bool { return m.visible }

// Frame returns the current spinner frame index.
func (m SpinnerModel) Frame() int { return m.frame }

// SetLabel updates the spinner label text.
func (m SpinnerModel) SetLabel(label string) SpinnerModel {
	m.label = label
	return m
}

// Update handles messages for the spinner.
func (m SpinnerModel) Update(msg tea.Msg) (SpinnerModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg.(type) {
	case tickMsg:
		m.frame = (m.frame + 1) % len(SpinnerCharSet)
		return m, spinTick()
	}

	return m, nil
}

// View renders the spinner.
func (m SpinnerModel) View() string {
	if !m.visible {
		return ""
	}

	char := SpinnerCharSet[m.frame]
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	return style.Render(fmt.Sprintf("%s %s", char, m.label))
}

// spinTick returns a tea.Cmd that sends a tickMsg after the spinner interval.
func spinTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// StartSpinner returns a tea.Cmd that shows the spinner and begins ticking.
func StartSpinner() tea.Cmd {
	return func() tea.Msg {
		return SpinnerShowMsg{}
	}
}

// StopSpinner returns a tea.Cmd that hides the spinner.
func StopSpinner() tea.Cmd {
	return func() tea.Msg {
		return SpinnerHideMsg{}
	}
}

// SpinnerShowMsg is a tea.Msg that shows the spinner.
type SpinnerShowMsg struct{}

// SpinnerHideMsg is a tea.Msg that hides the spinner.
type SpinnerHideMsg struct{}

// SpinnerUpdate returns the updated model and cmd for spinner show/hide messages.
func (m SpinnerModel) SpinnerUpdate(msg tea.Msg) (SpinnerModel, tea.Cmd) {
	switch msg.(type) {
	case SpinnerShowMsg:
		m = m.Show()
		return m, spinTick()
	case SpinnerHideMsg:
		m = m.Hide()
		return m, nil
	}
	return m, nil
}

// IsSpinnerMsg returns true if the message is a spinner control message.
func IsSpinnerMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case SpinnerShowMsg, SpinnerHideMsg, tickMsg:
		return true
	}
	return false
}

// RenderSpinnerClean renders without lipgloss (for testing).
func RenderSpinnerClean(frame int, label string, width int) string {
	if frame < 0 || frame >= len(SpinnerCharSet) {
		frame = 0
	}
	char := SpinnerCharSet[frame]
	text := fmt.Sprintf("%s %s", char, label)
	if width > 0 && len(text) > width {
		text = text[:width]
	}
	return text
}
