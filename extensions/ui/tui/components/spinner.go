package components

import (
	"fmt"
	"time"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// SpinnerModel displays an animated spinner with a "Thinking..." label during streaming.
type SpinnerModel struct {
	sp        spinner.Model
	visible   bool
	label     string
	width     int
	tickCount int
	theme     *palette.Theme
}

// NewSpinnerModel creates a new spinner model.
func NewSpinnerModel(theme *palette.Theme) SpinnerModel {
	if theme == nil {
		theme = palette.DefaultTheme()
	}

	sp := spinner.New(
		spinner.WithSpinner(spinner.Spinner{
			Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
			FPS:    time.Second / 10,
		}),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent))),
	)

	return SpinnerModel{
		sp:    sp,
		label: "Thinking...",
		theme: theme,
	}
}

// SetTheme updates the spinner's theme for dynamic accent colors.
func (m SpinnerModel) SetTheme(theme *palette.Theme) SpinnerModel {
	m.theme = theme
	return m
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

// SetLabel updates the spinner label text.
func (m SpinnerModel) SetLabel(label string) SpinnerModel {
	m.label = label
	return m
}

// SetCustomFrames updates the spinner animation frames and interval.
func (m SpinnerModel) SetCustomFrames(frames []string, interval time.Duration) SpinnerModel {
	if len(frames) == 0 {
		return m
	}

	accent := palette.DefaultTheme().Accent
	if m.theme != nil {
		accent = m.theme.Accent
	}

	m.sp = spinner.New(
		spinner.WithSpinner(spinner.Spinner{
			Frames: frames,
			FPS:    interval,
		}),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(accent))),
	)

	return m
}

// Update handles messages for the spinner.
func (m SpinnerModel) Update(msg tea.Msg) (SpinnerModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	var cmd tea.Cmd

	m.sp, cmd = m.sp.Update(msg)

	// Color pulse: alternate between Accent and AccentBright every 3 ticks
	if _, ok := msg.(spinner.TickMsg); ok {
		m.tickCount++
		accent := palette.DefaultTheme().Accent
		accentBright := palette.DefaultTheme().AccentBright

		if m.theme != nil {
			accent = m.theme.Accent
			accentBright = m.theme.AccentBright
		}

		if m.tickCount%6 < 3 {
			m.sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(accent))
		} else {
			m.sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(accentBright))
		}
	}

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
