package components

import (
	"fmt"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// SpinnerModel displays an animated spinner with a "Thinking..." label during streaming.
type SpinnerModel struct {
	sp      spinner.Model
	visible bool
	label   string
	width   int
}

// NewSpinnerModel creates a new spinner model.
func NewSpinnerModel() SpinnerModel {
	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    time.Second / 10,
	}
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))

	return SpinnerModel{
		sp:    sp,
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
	return m
}

// Visible returns whether the spinner is currently shown.
func (m SpinnerModel) Visible() bool { return m.visible }

// Frame returns the current spinner frame index.
func (m SpinnerModel) Frame() int { return 0 }

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

	var cmd tea.Cmd

	m.sp, cmd = m.sp.Update(msg)

	return m, cmd
}

// View renders the spinner.
func (m SpinnerModel) View() string {
	if !m.visible {
		return ""
	}

	return fmt.Sprintf("%s %s", m.sp.View(), m.label)
}

// Draw renders the spinner into a screen buffer region.
func (m SpinnerModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if !m.visible || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
}

// SpinnerUpdate returns the updated model and cmd for spinner show/hide messages.
func (m SpinnerModel) SpinnerUpdate(msg tea.Msg) (SpinnerModel, tea.Cmd) {
	switch msg.(type) {
	case SpinnerShowMsg:
		m = m.Show()
		return m, m.sp.Tick
	case SpinnerHideMsg:
		m = m.Hide()
		return m, nil
	}

	return m, nil
}

// IsSpinnerMsg returns true if the message is a spinner control or tick message.
func IsSpinnerMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case spinner.TickMsg, SpinnerShowMsg, SpinnerHideMsg:
		return true
	}

	return false
}

// StartSpinner returns a tea.Cmd that shows the spinner.
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

// SpinnerCharSet is kept for compatibility.
var SpinnerCharSet = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// RenderSpinnerClean renders without lipgloss (for testing).
func RenderSpinnerClean(frame int, label string, width int) string {
	if frame < 0 || frame >= len(SpinnerCharSet) {
		frame = 0
	}

	char := SpinnerCharSet[frame]

	text := fmt.Sprintf("%s %s", char, label)
	if width > 0 && utf8.RuneCountInString(text) > width {
		runes := []rune(text)
		text = string(runes[:width])
	}

	return text
}
