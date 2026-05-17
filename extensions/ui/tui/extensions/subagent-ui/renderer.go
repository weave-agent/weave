package subagent

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/weave-agent/weave/sdk"

	lipgloss "charm.land/lipgloss/v2"
)

const (
	statusCompleted = "completed"
	statusFailed    = "failed"
	statusRunning   = "running"
	statusCancelled = "canceled"
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
	if json.Unmarshal([]byte(content), &bgResp) == nil && bgResp.ID != "" &&
		strings.HasPrefix(bgResp.ID, "subagent_") {
		// Only treat as background if status is a known background value.
		switch bgResp.Status {
		case statusRunning, statusCompleted, statusFailed, statusCancelled:
			return r.renderBackgroundResponse(bgResp.ID, bgResp.Status, theme)
		}
	}

	// Foreground agent output — truncate long results.
	return r.renderForegroundOutput(content, theme, width)
}

// renderBackgroundResponse renders a compact card for a background agent launch.
func (r *subagentRenderer) renderBackgroundResponse(id, status string, theme sdk.ThemeInfo) string {
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.MutedBright))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.AccentBright))

	icon := "↗"
	iconColor := theme.Accent

	if status == statusCancelled {
		icon = "⊘"
		iconColor = theme.Warning
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Warning))
	}

	iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(iconColor))

	return fmt.Sprintf("%s Agent %s %s",
		iconStyle.Render(icon),
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
		if width > 3 && utf8.RuneCountInString(line) > width {
			runes := []rune(line)

			truncateAt := max(width-3, 0)
			if len(runes) > truncateAt {
				runes = runes[:truncateAt]
			}

			line = string(runes) + "..."
		}

		b.WriteString(outputStyle.Render(line))
	}

	return b.String()
}
