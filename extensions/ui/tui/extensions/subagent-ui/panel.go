package subagent

import (
	"fmt"
	"strings"
	"time"

	tui "github.com/weave-agent/weave-tui"
	"github.com/weave-agent/weave/sdk"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	panelKeyID       = "id"
	evtTypeToolStart = "tool_call"
	evtTypeToolEnd   = "tool_result"
	evtTypeMsgUpdate = "message_update"
	evtTypeMsgStart  = "message_start"
	evtTypeMsgEnd    = "message_end"
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
		cancelBtn = "  " + lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Error)).Render("[✕ cancel Ctrl+X]")
	}

	header := fmt.Sprintf(
		"%s %s  %s  %s%s",
		lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(statusIcon),
		lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.ForegroundBright)).Bold(true).Render(agent.Name),
		lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render(agent.Mode),
		lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.MutedBright)).Render(elapsed),
		cancelBtn,
	)

	line = d.drawLine(scr, area, line, header)

	// Separator
	if line < area.Dy() {
		sep := strings.Repeat("─", area.Dx())

		sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Border))

		line = d.drawLine(scr, area, line, sepStyle.Render(sep))
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

		line = d.drawLine(scr, area, line, "  "+entryStr)
	}
}

func (d *agentPanelDrawer) drawLine(scr uv.Screen, area uv.Rectangle, line int, content string) int {
	if line >= area.Dy() {
		return line
	}

	lineRect := uv.Rect(area.Min.X, area.Min.Y+line, area.Dx(), 1)
	uv.NewStyledString(content).Draw(scr, lineRect)

	return line + 1
}

// Update handles messages for the panel drawer.
func (d *agentPanelDrawer) Update(msg tea.Msg) (tui.PanelDrawer, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}

	ks := keyMsg.Keystroke()

	// Cancel on Ctrl+X (only for running agents)
	if ks == "ctrl+x" {
		if d.tracker != nil {
			agent := d.tracker.Get(d.agentID)

			if agent != nil && agent.Status == AgentRunning && d.bus != nil {
				d.bus.Publish(sdk.NewEvent("subagent.cancel", map[string]string{
					panelKeyID: d.agentID,
				}))
			}
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
	case evtTypeToolStart:
		tool := d.truncate(e.Tool, 10)

		content := d.truncate(e.Content, maxW-14)

		return lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Accent)).Render("⚙") + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Foreground)).Render(tool) +
			"  " + lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render(content)

	case evtTypeToolEnd:
		tool := d.truncate(e.Tool, 10)

		content := d.truncate(e.Content, maxW-14)

		return lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Success)).Render("✓") + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Foreground)).Render(tool) +
			"  " + lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Muted)).Render(content)

	case evtTypeMsgUpdate, evtTypeMsgStart:
		content := d.truncate(e.Content, maxW-4)

		return lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.AccentBright)).Render("→") + " " +
			lipgloss.NewStyle().Foreground(lipgloss.Color(d.theme.Foreground)).Render(content)

	case evtTypeMsgEnd:
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
	case AgentCancelled:
		return "⊘", d.theme.Warning
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
