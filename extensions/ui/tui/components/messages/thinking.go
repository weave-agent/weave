package messages

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"weave/ext/ui/tui/palette"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// ThinkingBlock renders a collapsible thinking section.
// Collapsed by default, showing a styled header with lightbulb icon.
type ThinkingBlock struct {
	content  string
	expanded bool
}

// NewThinkingBlock creates a new collapsed thinking block.
func NewThinkingBlock(content string) *ThinkingBlock {
	return &ThinkingBlock{
		content:  content,
		expanded: false,
	}
}

// Content returns the thinking content.
func (b *ThinkingBlock) Content() string {
	return b.content
}

// Expanded returns whether the block is expanded.
func (b *ThinkingBlock) Expanded() bool {
	return b.expanded
}

// ToggleExpanded flips the expand/collapse state.
func (b *ThinkingBlock) ToggleExpanded() {
	b.expanded = !b.expanded
}

// View renders the thinking block.
func (b *ThinkingBlock) View(width int) string {
	if width <= 0 {
		width = 80
	}

	theme := palette.DefaultTheme()

	// Header with background tint and lightbulb icon
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Primary)).
		Background(lipgloss.Color(theme.BackgroundTint)).
		Width(width - 2)

	if !b.expanded {
		return headerStyle.Render(FormatThinkingLabel(b.LineCount()))
	}

	lines := strings.Split(b.content, "\n")

	var bldr strings.Builder
	bldr.WriteString(headerStyle.Render(fmt.Sprintf("💡 Thinking ▼  (%d lines)", b.LineCount())))
	bldr.WriteString("\n")

	// Expanded content uses indented left border in primary color
	borderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Primary))
	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Muted)).
		Width(width - 4)

	borderBar := borderStyle.Render("│")

	for _, line := range lines {
		styledLine := contentStyle.Render(line)
		bldr.WriteString("  " + borderBar + " " + styledLine)
		bldr.WriteString("\n")
	}

	return strings.TrimRight(bldr.String(), "\n")
}

// Draw renders the thinking block into a screen buffer region.
func (b *ThinkingBlock) Draw(scr uv.Screen, area uv.Rectangle) {
	drawView(scr, area, b.View(area.Dx()))
}

// SetExpanded sets the expanded state directly.
func (b *ThinkingBlock) SetExpanded(expanded bool) {
	b.expanded = expanded
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

// FormatThinkingLabel creates the collapsed label text with a lightbulb icon.
func FormatThinkingLabel(lineCount int) string {
	if lineCount <= 0 {
		return "💡 Thinking"
	}

	return fmt.Sprintf("💡 Thinking  (%d lines)", lineCount)
}
