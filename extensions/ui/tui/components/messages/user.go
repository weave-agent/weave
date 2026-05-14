package messages

import (
	"fmt"
	"regexp"
	"strings"

	"weave/ext/ui/tui/palette"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

var skillXMLRe = regexp.MustCompile(`(?s)<skill\s+name="([^"]+)"[^>]*>(.*?)</skill>(.*)`)

type skillBlock struct {
	name     string
	body     string
	trailing string
}

func parseSkillXML(content string) (*skillBlock, bool) {
	matches := skillXMLRe.FindStringSubmatch(strings.TrimSpace(content))
	if matches == nil {
		return nil, false
	}

	return &skillBlock{
		name:     matches[1],
		body:     strings.TrimSpace(matches[2]),
		trailing: strings.TrimSpace(matches[3]),
	}, true
}

// UserMessage renders a user-sent message.
type UserMessage struct {
	content  string
	expanded bool
}

// NewUserMessage creates a new user message.
func NewUserMessage(content string) *UserMessage {
	return &UserMessage{content: content}
}

// Content returns the message text.
func (m *UserMessage) Content() string {
	return m.content
}

// Expanded returns whether the message is expanded.
func (m *UserMessage) Expanded() bool {
	return m.expanded
}

// ToggleExpanded flips the expand/collapse state.
func (m *UserMessage) ToggleExpanded() {
	m.expanded = !m.expanded
}

// IsSkillInvocation reports whether this message contains a skill XML block.
func (m *UserMessage) IsSkillInvocation() bool {
	_, ok := parseSkillXML(m.content)
	return ok
}

// View renders the user message with styled border and prefix.
func (m *UserMessage) View(width int) string {
	if width <= 0 {
		width = 80
	}

	theme := palette.DefaultTheme()

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Primary))
	prefixStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Primary))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))

	borderBar := borderStyle.Render("│")
	prefix := prefixStyle.Render("❯ ")

	// Content width accounts for border + prefix
	contentWidth := max(1, width-4)

	block, ok := parseSkillXML(m.content)
	if !ok {
		return styleUserContent(m.content, borderBar, prefix, contentStyle, contentWidth)
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted)).Width(contentWidth)

	if !m.expanded {
		label := fmt.Sprintf("[skill %s]", block.name)
		if block.trailing != "" {
			label += " " + block.trailing
		}

		return borderBar + prefix + dimStyle.Render(label)
	}

	var bldr strings.Builder

	header := fmt.Sprintf("[skill %s] ▼", block.name)
	bldr.WriteString(borderBar + prefix + dimStyle.Render(header))
	bldr.WriteString("\n")

	if block.body != "" {
		for line := range strings.SplitSeq(block.body, "\n") {
			bldr.WriteString(borderBar + "  " + contentStyle.Render(line))
			bldr.WriteString("\n")
		}
	}

	if block.trailing != "" {
		bldr.WriteString("\n")

		for line := range strings.SplitSeq(block.trailing, "\n") {
			bldr.WriteString(borderBar + "  " + contentStyle.Render(line))
			bldr.WriteString("\n")
		}
	}

	return strings.TrimRight(bldr.String(), "\n")
}

// styleUserContent prefixes each line with the border bar and symbol,
// applying the content style to the text.
func styleUserContent(content, borderBar, prefix string, contentStyle lipgloss.Style, width int) string {
	lines := strings.Split(content, "\n")

	var bldr strings.Builder

	for i, line := range lines {
		styledLine := contentStyle.Width(width).Render(line)
		bldr.WriteString(borderBar + prefix + styledLine)

		if i < len(lines)-1 {
			bldr.WriteString("\n")
		}
	}

	return bldr.String()
}

// Draw renders the user message into a screen buffer region.
func (m *UserMessage) Draw(scr uv.Screen, area uv.Rectangle) {
	drawView(scr, area, m.View(area.Dx()))
}
