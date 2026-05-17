package messages

import (
	"fmt"
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/lipgloss/v2"
)

// DiffLineKind classifies a unified diff line.
type DiffLineKind int

const (
	DiffContext DiffLineKind = iota
	DiffAdded
	DiffRemoved
	DiffHeader
	DiffHunk
)

// DiffLine represents a single line in a unified diff.
type DiffLine struct {
	Kind    DiffLineKind
	Content string
}

// ParseDiff parses unified diff text into classified lines.
// Returns nil if the input does not look like a diff.
func ParseDiff(text string) []DiffLine {
	lines := strings.Split(text, "\n")
	if !isUnifiedDiff(lines) {
		return nil
	}

	var result []DiffLine
	for _, line := range lines {
		result = append(result, classifyLine(line))
	}

	return result
}

// isUnifiedDiff checks if the text starts with ---/+++ diff markers
// and contains at least one @@ hunk marker.
func isUnifiedDiff(lines []string) bool {
	foundHeader := false
	hasHunk := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "--- ") {
			foundHeader = true
			continue
		}

		if foundHeader && strings.HasPrefix(trimmed, "+++ ") {
			// Require at least one hunk marker for confidence.
			for _, l := range lines {
				if strings.HasPrefix(strings.TrimSpace(l), "@@") {
					hasHunk = true
					break
				}
			}

			return hasHunk
		}

		break
	}

	return false
}

// classifyLine classifies a single diff line.
func classifyLine(line string) DiffLine {
	if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
		return DiffLine{Kind: DiffHeader, Content: line}
	}

	if strings.HasPrefix(line, "@@") {
		return DiffLine{Kind: DiffHunk, Content: line}
	}

	if strings.HasPrefix(line, "+") {
		return DiffLine{Kind: DiffAdded, Content: line}
	}

	if strings.HasPrefix(line, "-") {
		return DiffLine{Kind: DiffRemoved, Content: line}
	}

	return DiffLine{Kind: DiffContext, Content: line}
}

// DiffRenderer renders unified diff content with color coding.
type DiffRenderer struct {
	addedStyle   lipgloss.Style
	removedStyle lipgloss.Style
	contextStyle lipgloss.Style
	headerStyle  lipgloss.Style
	hunkStyle    lipgloss.Style
}

// NewDiffRenderer creates a new diff renderer with theme colors.
func NewDiffRenderer() *DiffRenderer {
	theme := palette.DefaultTheme()

	return &DiffRenderer{
		addedStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success)),
		removedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Error)),
		contextStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted)),
		headerStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent)),
		hunkStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.AccentBright)),
	}
}

// Render formats diff text with color coding.
// Returns the original text if it's not a valid diff.
func (r *DiffRenderer) Render(text string, width int) string {
	lines := ParseDiff(text)
	if lines == nil {
		return text
	}

	var bldr strings.Builder

	for i, line := range lines {
		if i > 0 {
			bldr.WriteString("\n")
		}

		var rendered string

		switch line.Kind {
		case DiffAdded:
			rendered = r.addedStyle.Render(line.Content)
		case DiffRemoved:
			rendered = r.removedStyle.Render(line.Content)
		case DiffHeader:
			rendered = r.headerStyle.Render(line.Content)
		case DiffHunk:
			rendered = r.hunkStyle.Render(line.Content)
		default:
			rendered = r.contextStyle.Render(line.Content)
		}

		bldr.WriteString(rendered)
	}

	return bldr.String()
}

// IsDiffContent checks if text looks like unified diff output.
func IsDiffContent(text string) bool {
	return isUnifiedDiff(strings.Split(text, "\n"))
}

// DiffStats returns the number of added and removed lines in a diff.
func DiffStats(text string) (added, removed int) {
	lines := ParseDiff(text)
	if lines == nil {
		return 0, 0
	}

	for _, line := range lines {
		switch line.Kind {
		case DiffAdded:
			added++
		case DiffRemoved:
			removed++
		default:
		}
	}

	return added, removed
}

// FormatDiffStats returns a human-readable summary of diff changes.
func FormatDiffStats(added, removed int) string {
	if added == 0 && removed == 0 {
		return ""
	}

	return fmt.Sprintf("+%d/-%d", added, removed)
}
