package messages

import (
	"strings"

	"github.com/weave-agent/weave-tui/palette"
	"github.com/weave-agent/weave/sdk"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// NotificationMessage renders a typed notification in the chat area.
type NotificationMessage struct {
	content string
	level   sdk.NotifyLevel
}

// NewNotificationMessage creates a new notification message with the given level.
func NewNotificationMessage(content string, level sdk.NotifyLevel) *NotificationMessage {
	return &NotificationMessage{content: content, level: level}
}

// Content returns the notification text.
func (m *NotificationMessage) Content() string {
	return m.content
}

// Level returns the notification level.
func (m *NotificationMessage) Level() sdk.NotifyLevel {
	return m.level
}

// View renders the notification with a colored left border based on level.
func (m *NotificationMessage) View(width int) string {
	if width <= 0 {
		width = 80
	}

	theme := palette.DefaultTheme()
	borderColor, textColor := colorsForLevel(m.level, theme)

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(textColor))

	borderBar := borderStyle.Render("│")
	prefix := borderStyle.Render("◆ ")

	contentWidth := max(1, width-4)

	lines := strings.Split(m.content, "\n")

	var bldr strings.Builder

	for i, line := range lines {
		styledLine := contentStyle.Width(contentWidth).Render(line)
		bldr.WriteString(borderBar + prefix + styledLine)

		if i < len(lines)-1 {
			bldr.WriteString("\n")
		}
	}

	return bldr.String()
}

// Draw renders the notification into a screen buffer region.
func (m *NotificationMessage) Draw(scr uv.Screen, area uv.Rectangle) {
	drawView(scr, area, m.View(area.Dx()))
}

func colorsForLevel(level sdk.NotifyLevel, theme *palette.Theme) (borderColor, textColor string) {
	switch level {
	case sdk.NotifyWarning:
		return theme.Warning, theme.Warning
	case sdk.NotifyError:
		return theme.Error, theme.Error
	case sdk.NotifySuccess:
		return theme.Success, theme.Success
	default:
		return theme.Accent, theme.Foreground
	}
}
