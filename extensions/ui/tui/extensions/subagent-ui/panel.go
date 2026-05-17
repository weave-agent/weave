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
	agentID      string
	tracker      *AgentTracker
	theme        sdk.ThemeInfo
	bus          sdk.Bus
	scrollOffset int
}

// newAgentPanelDrawer creates a panel drawer for the given agent.
// The bus is used to publish cancel events; may be nil in tests.
func newAgentPanelDrawer(agentID string, tracker *AgentTracker, theme sdk.ThemeInfo, bus sdk.Bus) *agentPanelDrawer {
	return &agentPanelDrawer{
		agentID: agentID,
		tracker: tracker,
		theme:   theme,
		bus:     bus,
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

	// Line 1: Header row — status icon + name + mode + elapsed + cancel button
	statusIcon, statusColor := d.statusIndicator(agent.Status)
	elapsed := d.formatElapsed(agent)

	cancelBtn := ""
	if agent.Status == AgentRunning {
		cancelBtn = "  " + lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Error)).Render("[✕ cancel]")
	}

	header := fmt.Sprintf("%s %s  %s  %s%s",
		lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(statusIcon),
		lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.ForegroundBright)).Bold(true).Render(agent.Name),
		lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render(agent.Mode),
		lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.MutedBright)).Render(elapsed),
		cancelBtn,
	)

	if line < area.Dy() {
		lineRect := uv.Rect(area.Min.X, area.Min.Y+line, area.Dx(), 1)
		uv.NewStyledString(header).Draw(scr, lineRect)
		line++
	}

	// Line 2: Prompt row (if available)
	if agent.Prompt != "" && line < area.Dy() {
		prompt := d.truncate(agent.Prompt, area.Dx()-12)
		promptLine := lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render("  Prompt: " + prompt)
		lineRect := uv.Rect(area.Min.X, area.Min.Y+line, area.Dx(), 1)
		uv.NewStyledString(promptLine).Draw(scr, lineRect)
		line++
	}

	// Line 3: Separator
	if line < area.Dy() {
		sep := strings.Repeat("─", area.Dx())
		sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Border))
		lineRect := uv.Rect(area.Min.X, area.Min.Y+line, area.Dx(), 1)
		uv.NewStyledString(sepStyle.Render(sep)).Draw(scr, lineRect)
		line++
	}

	// Remaining lines: Scrollable tool log from ring buffer snapshot
	if agent.Output == nil {
		return
	}

	entries := agent.Output.Snapshot()
	if len(entries) == 0 {
		return
	}

	visibleLines := area.Dy() - line
	if visibleLines <= 0 {
		return
	}

	// Clamp scroll offset
	maxScroll := max(len(entries)-visibleLines, 0)
	d.scrollOffset = max(min(d.scrollOffset, maxScroll), 0)

	start := d.scrollOffset
	end := min(start+visibleLines, len(entries))

	for i := start; i < end && line < area.Dy(); i++ {
		entry := entries[i]
		entryStr := d.formatEntry(entry, area.Dx()-4)
		if entryStr == "" {
			continue
		}

		lineRect := uv.Rect(area.Min.X, area.Min.Y+line, area.Dx(), 1)
		uv.NewStyledString("  " + entryStr).Draw(scr, lineRect)
		line++
	}
}

// Update handles messages for the panel drawer.
func (d *agentPanelDrawer) Update(msg tea.Msg) (tui.PanelDrawer, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}

	ks := keyMsg.Keystroke()

	// Cancel on Ctrl+X or Enter
	if ks == "ctrl+x" || ks == "enter" {
		if d.bus != nil {
			d.bus.Publish(sdk.NewEvent("subagent.cancel", map[string]string{
				"id": d.agentID,
			}))
		}

		return d, nil
	}

	// Scroll up
	if ks == "up" {
		if d.scrollOffset > 0 {
			d.scrollOffset--
		}

		return d, nil
	}

	// Scroll down
	if ks == "down" {
		d.scrollOffset++
		return d, nil
	}

	return d, nil
}

// Handles returns true for key press messages.
func (d *agentPanelDrawer) Handles(msg tea.Msg) bool {
	_, ok := msg.(tea.KeyPressMsg)
	return ok
}

// formatEntry renders a single output entry as a styled string.
func (d *agentPanelDrawer) formatEntry(e outputEntry, maxW int) string {
	maxW = max(maxW, 10)

	switch e.Type {
	case "tool_start":
		tool := d.truncate(e.Tool, 10)
		content := d.truncate(e.Content, maxW-14)
		return lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Accent)).Render("⚙") + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Foreground)).Render(tool) +
			"  " + lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render(content)
	case "tool_end":
		tool := d.truncate(e.Tool, 10)
		content := d.truncate(e.Content, maxW-14)
		return lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Success)).Render("✓") + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Foreground)).Render(tool) +
			"  " + lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render(content)
	case "message_update", "message_start":
		content := d.truncate(e.Content, maxW-4)
		return lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.AccentBright)).Render("→") + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Foreground)).Render(content)
	case "message_end":
		return ""
	default:
		content := d.truncate(e.Content, maxW-4)
		return lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render("·") + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render(content)
	}
}

// truncate shortens a string to maxRunes, appending "..." if truncated.
func (d *agentPanelDrawer) truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}

	if maxRunes <= 3 {
		return strings.Repeat(".", maxRunes)
	}

	return string(runes[:maxRunes-3]) + "..."
}

// statusIndicator returns the icon and color for the agent's current status.
func (d *agentPanelDrawer) statusIndicator(status AgentStatus) (string, string) {
	switch status {
	case AgentRunning:
		return "●", d.theme.Accent
	case AgentCompleted:
		return "✓", d.theme.Success
	case AgentFailed:
		return "✗", d.theme.Error
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
