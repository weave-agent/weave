package messages

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/weave-agent/weave-tui/palette"

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

// View renders the user message with left-edge bar.
func (m *UserMessage) View(width int) string {
	if width <= 0 {
		width = 80
	}

	theme := palette.DefaultTheme()

	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.AccentDim))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Foreground))

	bar := barStyle.Render("▐")

	contentWidth := max(1, width-2)

	block, ok := parseSkillXML(m.content)
	if !ok {
		return styleUserContent(m.content, bar, contentStyle, contentWidth)
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted)).Width(contentWidth)

	if !m.expanded {
		label := fmt.Sprintf("[skill %s]", block.name)
		if block.trailing != "" {
			label += " " + block.trailing
		}

		return bar + " " + dimStyle.Render(label)
	}

	var bldr strings.Builder

	header := fmt.Sprintf("[skill %s] ▼", block.name)
	bldr.WriteString(bar + " " + dimStyle.Render(header))
	bldr.WriteString("\n")

	if block.body != "" {
		for line := range strings.SplitSeq(block.body, "\n") {
			bldr.WriteString("  " + contentStyle.Render(line))
			bldr.WriteString("\n")
		}
	}

	if block.trailing != "" {
		bldr.WriteString("\n")

		for line := range strings.SplitSeq(block.trailing, "\n") {
			bldr.WriteString("  " + contentStyle.Render(line))
			bldr.WriteString("\n")
		}
	}

	return strings.TrimRight(bldr.String(), "\n")
}

// styleUserContent prefixes each line with the bar and a gap,
// applying the content style to the text.
func styleUserContent(content, bar string, contentStyle lipgloss.Style, width int) string {
	lines := strings.Split(content, "\n")

	var bldr strings.Builder

	for i, line := range lines {
		styledLine := contentStyle.Width(width).Render(line)
		bldr.WriteString(bar + " " + styledLine)

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
