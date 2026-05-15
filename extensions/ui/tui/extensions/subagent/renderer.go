package subagent

import (
	"encoding/json"
	"fmt"
	"strings"

	"weave/sdk"

	lipgloss "charm.land/lipgloss/v2"
)

// subagentRenderer implements tui.RichToolRenderer for subagent tool output.
// It renders background agent responses as compact cards and foreground
// responses with truncated output.
type subagentRenderer struct{}

// Render produces a theme-styled card for subagent tool result content.
func (r *subagentRenderer) Render(content string, theme sdk.ThemeInfo, width int) string {
	if content == "" {
		return ""
	}

	// Try parsing as a background agent response: {"id":"...","status":"running"}
	var bgResp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.Unmarshal([]byte(content), &bgResp) == nil && bgResp.ID != "" {
		return r.renderBackgroundResponse(bgResp.ID, bgResp.Status, theme)
	}

	// Foreground agent output — truncate long results.
	return r.renderForegroundOutput(content, theme, width)
}

// renderBackgroundResponse renders a compact card for a background agent launch.
func (r *subagentRenderer) renderBackgroundResponse(id, status string, theme sdk.ThemeInfo) string {
	iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Primary))
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.MutedBright))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.PrimaryBright))

	return fmt.Sprintf("%s Agent %s %s",
		iconStyle.Render("↗"),
		idStyle.Render(id),
		statusStyle.Render("("+status+")"),
	)
}

// renderForegroundOutput renders foreground agent output with truncation.
func (r *subagentRenderer) renderForegroundOutput(content string, theme sdk.ThemeInfo, width int) string {
	lines := strings.Split(content, "\n")
	maxLines := 8
	if len(lines) > maxLines {
		truncated := make([]string, maxLines)
		copy(truncated, lines[:maxLines])
		remaining := len(lines) - maxLines
		truncated[maxLines-1] = fmt.Sprintf("... (%d more lines)", remaining)
		lines = truncated
	}

	outputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Foreground))

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}

		// Truncate wide lines if width is specified.
		if width > 0 && len(line) > width {
			line = line[:width-3] + "..."
		}

		b.WriteString(outputStyle.Render(line))
	}

	return b.String()
}
