package messages

import (
	"fmt"
	"regexp"
	"strings"

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

// View renders the user message.
func (m *UserMessage) View(width int) string {
	block, ok := parseSkillXML(m.content)
	if !ok {
		return m.content
	}

	if width <= 0 {
		width = 80
	}

	dimStyle := lipgloss.NewStyle().Faint(true).Width(width - 2)

	if !m.expanded {
		label := fmt.Sprintf("  [skill %s]", block.name)
		if block.trailing != "" {
			label += " " + block.trailing
		}

		return dimStyle.Render(label)
	}

	var bldr strings.Builder

	header := fmt.Sprintf("  [skill %s] ▼", block.name)
	bldr.WriteString(dimStyle.Render(header))
	bldr.WriteString("\n")

	if block.body != "" {
		for line := range strings.SplitSeq(block.body, "\n") {
			bldr.WriteString(dimStyle.Render("  " + line))
			bldr.WriteString("\n")
		}
	}

	if block.trailing != "" {
		bldr.WriteString("\n")

		for line := range strings.SplitSeq(block.trailing, "\n") {
			bldr.WriteString(dimStyle.Render("  " + line))
			bldr.WriteString("\n")
		}
	}

	return strings.TrimRight(bldr.String(), "\n")
}

// Draw renders the user message into a screen buffer region.
func (m *UserMessage) Draw(scr uv.Screen, area uv.Rectangle) {
	drawView(scr, area, m.View(area.Dx()))
}
