package overlays

import (
	"fmt"
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// ConfirmResultMsg is emitted when the user responds to a confirm dialog.
type ConfirmResultMsg struct {
	Confirmed bool
}

// ConfirmModel is a yes/no dialog overlay.
type ConfirmModel struct {
	message string
	cursor  int // 0 = yes, 1 = no
	width   int
	height  int
	visible bool
}

// NewConfirmModel creates a new confirm model.
func NewConfirmModel(message string) ConfirmModel {
	return ConfirmModel{
		message: message,
		cursor:  0,
	}
}

// Visible returns whether the confirm dialog is shown.
func (m ConfirmModel) Visible() bool { return m.visible }

// Show makes the confirm dialog visible.
func (m ConfirmModel) Show() ConfirmModel {
	m.visible = true
	m.cursor = 0

	return m
}

// Hide hides the confirm dialog.
func (m ConfirmModel) Hide() ConfirmModel {
	m.visible = false
	return m
}

// SetSize updates the confirm dialog dimensions.
func (m ConfirmModel) SetSize(width, height int) ConfirmModel {
	m.width = width
	m.height = height

	return m
}

// Width returns the confirm dialog width.
func (m ConfirmModel) Width() int { return m.width }

// Height returns the confirm dialog height.
func (m ConfirmModel) Height() int { return m.height }

// Cursor returns the current cursor position (0=yes, 1=no).
func (m ConfirmModel) Cursor() int { return m.cursor }

// Update handles messages for the confirm dialog.
func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleKey(key)
	}

	return m, nil
}

func (m ConfirmModel) handleKey(msg tea.KeyPressMsg) (ConfirmModel, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEsc:
		m.visible = false
		return m, func() tea.Msg { return ConfirmResultMsg{Confirmed: false} }

	case tea.KeyLeft:
		m.cursor = 0
		return m, nil

	case tea.KeyRight:
		m.cursor = 1
		return m, nil

	case tea.KeyEnter:
		m.visible = false
		return m, func() tea.Msg { return ConfirmResultMsg{Confirmed: m.cursor == 0} }

	default:
		if msg.Text != "" {
			switch strings.ToLower(msg.Text) {
			case "y":
				m.visible = false
				return m, func() tea.Msg { return ConfirmResultMsg{Confirmed: true} }
			case "n":
				m.visible = false
				return m, func() tea.Msg { return ConfirmResultMsg{Confirmed: false} }
			}
		}
	}

	return m, nil
}

// View renders the confirm dialog overlay.
func (m ConfirmModel) View() string {
	if !m.visible || m.width < 4 {
		return ""
	}

	theme := palette.DefaultTheme()
	boxWidth := min(50, m.width-4)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Warning)).
		Width(boxWidth-2).
		Padding(0, 1)

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground))

	activeBtnStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground)).
		Background(lipgloss.Color(theme.Warning)).
		Padding(0, 2)

	inactiveBtnStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Muted)).
		Padding(0, 2)

	yesBtn := inactiveBtnStyle.Render("Yes")

	noBtn := inactiveBtnStyle.Render("No")
	if m.cursor == 0 {
		yesBtn = activeBtnStyle.Render("Yes")
	} else {
		noBtn = activeBtnStyle.Render("No")
	}

	buttons := fmt.Sprintf("%s  %s", yesBtn, noBtn)
	content := messageStyle.Render(m.message) + "\n\n" + buttons
	box := borderStyle.Render(content)

	lines := strings.Split(box, "\n")

	return lipgloss.NewStyle().
		MarginTop(max(0, (m.height-len(lines))/2)).
		MarginLeft(max(0, (m.width-boxWidth)/2)).
		Render(strings.Join(lines, "\n"))
}

// Draw renders the confirm dialog overlay into a screen buffer region.
func (m ConfirmModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if !m.visible || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
}
