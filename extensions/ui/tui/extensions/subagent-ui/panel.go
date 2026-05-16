package subagent

import (
	"fmt"
	"strings"
	"time"

	"weave/ext/ui/tui"
	"weave/sdk"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// agentPanelDrawer implements tui.PanelDrawer for a single tracked subagent.
type agentPanelDrawer struct {
	agentID string
	tracker *AgentTracker
	theme   sdk.ThemeInfo
}

// newAgentPanelDrawer creates a panel drawer for the given agent.
func newAgentPanelDrawer(agentID string, tracker *AgentTracker, theme sdk.ThemeInfo) *agentPanelDrawer {
	return &agentPanelDrawer{
		agentID: agentID,
		tracker: tracker,
		theme:   theme,
	}
}

// Draw renders the agent panel content into the screen buffer.
func (d *agentPanelDrawer) Draw(scr uv.Screen, area uv.Rectangle) {
	if area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	agent := d.tracker.Get(d.agentID)
	if agent == nil {
		return
	}

	line := 0

	// Line 1: status indicator + name + mode + elapsed time
	statusIcon, statusColor := d.statusIndicator(agent.Status)
	elapsed := d.formatElapsed(agent)

	header := fmt.Sprintf("%s %s  %s  %s",
		lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(statusIcon),
		lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.ForegroundBright)).Bold(true).Render(agent.Name),
		lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render(agent.Mode),
		lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.MutedBright)).Render(elapsed),
	)

	if line < area.Dy() {
		lineRect := uv.Rect(area.Min.X, area.Min.Y+line, area.Dx(), 1)
		uv.NewStyledString(header).Draw(scr, lineRect)

		line++
	}

	// Remaining lines: result preview
	if agent.Result != "" && line < area.Dy() {
		result := d.formatResult(agent.Result, area.Dx(), area.Dy()-line)
		resultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted))

		for rLine := range strings.SplitSeq(result, "\n") {
			if line >= area.Dy() {
				break
			}

			lineRect := uv.Rect(area.Min.X, area.Min.Y+line, area.Dx(), 1)
			uv.NewStyledString(resultStyle.Render(rLine)).Draw(scr, lineRect)

			line++
		}
	}
}

// Update handles messages for the panel drawer.
func (d *agentPanelDrawer) Update(msg tea.Msg) (tui.PanelDrawer, tea.Cmd) {
	return d, nil
}

// Handles returns true for messages this drawer should process.
func (d *agentPanelDrawer) Handles(msg tea.Msg) bool {
	return false
}

// statusIndicator returns the icon and color for the agent's current status.
func (d *agentPanelDrawer) statusIndicator(status AgentStatus) (string, string) {
	switch status {
	case AgentRunning:
		return "●", d.theme.Accent // ●
	case AgentCompleted:
		return "✓", d.theme.Success // ✓
	case AgentFailed:
		return "✗", d.theme.Error // ✗
	default:
		return "●", d.theme.Muted
	}
}

// formatElapsed returns a human-readable elapsed time string.
func (d *agentPanelDrawer) formatElapsed(agent *TrackedAgent) string {
	var elapsed time.Duration
	if agent.Status == AgentRunning {
		elapsed = time.Since(agent.SpawnedAt)
	} else {
		elapsed = agent.DoneAt.Sub(agent.SpawnedAt)
	}

	if elapsed < 0 {
		elapsed = 0
	}

	if elapsed < time.Minute {
		return fmt.Sprintf("%ds", int(elapsed.Seconds()))
	}

	return fmt.Sprintf("%dm%ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
}

// formatResult truncates and formats the result for display in the panel.
func (d *agentPanelDrawer) formatResult(result string, maxWidth, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if maxLines <= 0 {
		return ""
	}

	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	maxRunes := max(maxWidth-4, 10)

	var b strings.Builder

	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}

		runes := []rune(l)
		if len(runes) > maxRunes {
			if maxRunes <= 3 {
				b.WriteString(strings.Repeat(".", maxRunes))
			} else {
				b.WriteString(string(runes[:maxRunes-3]) + "...")
			}
		} else {
			b.WriteString(l)
		}
	}

	return b.String()
}
