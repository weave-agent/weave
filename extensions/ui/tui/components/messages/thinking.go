package messages

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// ThinkingBlock renders a collapsible thinking section.
// Collapsed by default, showing a [thinking] label.
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

	dimStyle := lipgloss.NewStyle().Faint(true).Width(width - 2)

	if !b.expanded {
		return dimStyle.Render(FormatThinkingLabel(b.LineCount()))
	}

	lines := strings.Split(b.content, "\n")

	var bldr strings.Builder
	bldr.WriteString(dimStyle.Render("  [thinking] ▼"))
	bldr.WriteString("\n")

	for _, line := range lines {
		bldr.WriteString(dimStyle.Render("  " + line))
		bldr.WriteString("\n")
	}

	return strings.TrimRight(bldr.String(), "\n")
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

// FormatThinkingLabel creates the collapsed label text.
func FormatThinkingLabel(lineCount int) string {
	if lineCount <= 0 {
		return "  [thinking]"
	}

	return fmt.Sprintf("  [thinking] (%d lines)", lineCount)
}
