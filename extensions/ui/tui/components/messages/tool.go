package messages

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"weave/sdk"

	"github.com/charmbracelet/lipgloss"
)

const maxCollapsedLines = 20

// ToolState represents the execution state of a tool call.
type ToolState int

const (
	ToolPending ToolState = iota
	ToolSuccess
	ToolError
)

// ToolPanel renders a tool call with its output in a bordered panel.
type ToolPanel struct {
	toolID         string
	toolName       string
	args           string
	output         string
	state          ToolState
	expanded       bool
	diffRenderer   *DiffRenderer
	customRenderer sdk.ToolRenderer
}

// NewToolPanel creates a new tool panel in pending state.
func NewToolPanel(toolID, toolName, args string) *ToolPanel {
	return &ToolPanel{
		toolID:   toolID,
		toolName: toolName,
		args:     truncateArgs(args, 100),
		state:    ToolPending,
		expanded: false,
	}
}

// ToolID returns the tool call ID.
func (p *ToolPanel) ToolID() string {
	return p.toolID
}

// ItemID implements ChatItemIdentity for in-place updates.
func (p *ToolPanel) ItemID() string {
	return p.toolID
}

// State returns the current tool state.
func (p *ToolPanel) State() ToolState {
	return p.state
}

// Expanded returns whether the panel is expanded.
func (p *ToolPanel) Expanded() bool {
	return p.expanded
}

// SetResult updates the panel with a tool result.
func (p *ToolPanel) SetResult(output string, isError bool) {
	p.output = output
	if isError {
		p.state = ToolError
	} else {
		p.state = ToolSuccess
	}
}

// ToggleExpanded flips the expand/collapse state.
func (p *ToolPanel) ToggleExpanded() {
	p.expanded = !p.expanded
}

// SetDiffRenderer sets the diff renderer for auto-detecting diff output.
func (p *ToolPanel) SetDiffRenderer(r *DiffRenderer) {
	p.diffRenderer = r
}

// SetRenderer sets a custom tool renderer registered via sdk.UI.
func (p *ToolPanel) SetRenderer(r sdk.ToolRenderer) {
	p.customRenderer = r
}

// View renders the tool panel as a bordered box.
func (p *ToolPanel) View(width int) string {
	if width <= 0 {
		width = 80
	}

	borderStyle := borderStyleForState(p.state, width)
	header := p.renderHeader()
	body := p.renderBody()

	var b strings.Builder
	b.WriteString(borderStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(body)

	return b.String()
}

func (p *ToolPanel) renderHeader() string {
	stateLabel := stateLabelForState(p.state)
	if p.args != "" {
		return fmt.Sprintf(" %s %s(%s)", stateLabel, p.toolName, p.args)
	}
	return fmt.Sprintf(" %s %s", stateLabel, p.toolName)
}

func (p *ToolPanel) renderBody() string {
	if p.output == "" {
		dim := lipgloss.NewStyle().Faint(true)
		if p.state == ToolPending {
			return dim.Render("  running...")
		}
		return dim.Render("  (no output)")
	}

	// Use custom renderer if registered.
	if p.customRenderer != nil {
		return p.customRenderer.Render(p.output, 0)
	}

	// Auto-detect diff content and use diff renderer.
	if p.diffRenderer != nil && IsDiffContent(p.output) {
		return p.diffRenderer.Render(p.output, 0)
	}

	lines := strings.Split(p.output, "\n")

	if !p.expanded && len(lines) > maxCollapsedLines {
		visible := lines[:maxCollapsedLines]
		hidden := len(lines) - maxCollapsedLines
		body := strings.Join(visible, "\n")
		if p.state == ToolError {
			body = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(body)
		}
		return body + fmt.Sprintf("\n  ... %d more lines (collapsed)", hidden)
	}

	body := strings.Join(lines, "\n")
	if p.state == ToolError {
		body = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(body)
	}
	return body
}

func borderStyleForState(state ToolState, width int) lipgloss.Style {
	switch state {
	case ToolPending:
		return lipgloss.NewStyle().
			Width(width - 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")) // dim gray
	case ToolSuccess:
		return lipgloss.NewStyle().
			Width(width - 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("2")) // green
	case ToolError:
		return lipgloss.NewStyle().
			Width(width - 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("1")) // red
	default:
		return lipgloss.NewStyle().
			Width(width - 2).
			Border(lipgloss.RoundedBorder())
	}
}

func stateLabelForState(state ToolState) string {
	switch state {
	case ToolPending:
		return "⏳"
	case ToolSuccess:
		return "✓"
	case ToolError:
		return "✗"
	default:
		return "?"
	}
}

func truncateArgs(args string, maxLen int) string {
	args = strings.TrimSpace(args)
	// Try to keep it on one line.
	args = strings.ReplaceAll(args, "\n", " ")
	if utf8.RuneCountInString(args) > maxLen {
		runes := []rune(args)
		return string(runes[:maxLen-3]) + "..."
	}
	return args
}
