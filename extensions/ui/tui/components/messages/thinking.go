package messages

import (
	"strings"
	"unicode/utf8"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// ThinkingBlock renders a thinking section, always shown.
type ThinkingBlock struct {
	content string
}

// NewThinkingBlock creates a new thinking block.
func NewThinkingBlock(content string) *ThinkingBlock {
	return &ThinkingBlock{content: content}
}

// Content returns the thinking content.
func (b *ThinkingBlock) Content() string {
	return b.content
}

// View renders the thinking block with left border bar.
func (b *ThinkingBlock) View(width int) string {
	if width <= 0 {
		width = 80
	}

	theme := palette.DefaultTheme()

	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ForegroundDim)).
		Width(width - 2)

	bar := barStyle.Render("░")

	lines := strings.Split(b.content, "\n")

	var bldr strings.Builder
	bldr.WriteString(bar + " " + headerStyle.Render("∴ Thinking…"))
	bldr.WriteString("\n")

	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ForegroundDim)).
		Width(width - 4)

	for _, line := range lines {
		styledLine := contentStyle.Render(line)
		bldr.WriteString("  " + bar + " " + styledLine)
		bldr.WriteString("\n")
	}

	return strings.TrimRight(bldr.String(), "\n")
}

// Draw renders the thinking block into a screen buffer region.
func (b *ThinkingBlock) Draw(scr uv.Screen, area uv.Rectangle) {
	drawView(scr, area, b.View(area.Dx()))
}

// Summary returns a short preview of the thinking content.
func (b *ThinkingBlock) Summary(maxLen int) string {
	first := strings.SplitN(b.content, "\n", 2)[0]
	if utf8.RuneCountInString(first) > maxLen {
		runes := []rune(first)
		return string(runes[:maxLen-3]) + "..."
	}

	if first == "" {
		return "(empty)"
	}

	return first
}

// LineCount returns the number of lines in the thinking content.
func (b *ThinkingBlock) LineCount() int {
	if b.content == "" {
		return 0
	}

	return len(strings.Split(b.content, "\n"))
}
