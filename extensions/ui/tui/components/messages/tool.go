package messages

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"weave/ext/ui/tui/palette"
	"weave/sdk"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const maxCollapsedLines = 20

// ToolState represents the execution state of a tool call.
type ToolState int

const (
	ToolPending ToolState = iota
	ToolSuccess
	ToolError
)

// ToolPanel renders a tool call with its output in a panel.
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

// View renders the tool panel.
func (p *ToolPanel) View(width int) string {
	if width <= 0 {
		width = 80
	}

	borderStyle := borderStyleForState(p.state, width)
	header := p.renderHeader()
	body := p.renderBody(width)

	var b strings.Builder
	b.WriteString(borderStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(body)

	return b.String()
}

// Draw renders the tool panel into a screen buffer region.
func (p *ToolPanel) Draw(scr uv.Screen, area uv.Rectangle) {
	drawView(scr, area, p.View(area.Dx()))
}

func (p *ToolPanel) renderHeader() string {
	stateLabel := stateLabelForState(p.state)
	if p.args != "" {
		formatted := truncateArgs(formatArgs(p.args), 100)
		if formatted != "" {
			return fmt.Sprintf(" %s %s(%s)", stateLabel, p.toolName, formatted)
		}
	}

	return fmt.Sprintf(" %s %s", stateLabel, p.toolName)
}

func (p *ToolPanel) renderBody(width int) string {
	theme := palette.DefaultTheme()

	if p.output == "" {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))
		if p.state == ToolPending {
			return dim.Render("    running...")
		}

		return dim.Render("    (no output)")
	}

	// Use custom renderer if registered.
	if p.customRenderer != nil {
		return padLeft(p.customRenderer.Render(p.output, width), 4)
	}

	// Auto-detect diff content and use diff renderer.
	if p.diffRenderer != nil && IsDiffContent(p.output) {
		return padLeft(p.diffRenderer.Render(p.output, width), 4)
	}

	lines := strings.Split(p.output, "\n")

	if !p.expanded && len(lines) > maxCollapsedLines {
		visible := lines[:maxCollapsedLines]
		hidden := len(lines) - maxCollapsedLines

		body := padLeft(strings.Join(visible, "\n"), 4)
		if p.state == ToolError {
			body = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Error)).Render(body)
		}

		return body + fmt.Sprintf("\n    ... %d more lines (collapsed)", hidden)
	}

	body := padLeft(strings.Join(lines, "\n"), 4)
	if p.state == ToolError {
		body = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Error)).Render(body)
	}

	return body
}

func borderStyleForState(state ToolState, width int) lipgloss.Style {
	theme := palette.DefaultTheme()

	switch state {
	case ToolPending:
		return lipgloss.NewStyle().
			Width(width).
			Background(lipgloss.Color(theme.BackgroundTintPending))
	case ToolSuccess:
		return lipgloss.NewStyle().
			Width(width).
			Background(lipgloss.Color(theme.BackgroundTintSuccess))
	case ToolError:
		return lipgloss.NewStyle().
			Width(width).
			Background(lipgloss.Color(theme.BackgroundTintError))
	default:
		return lipgloss.NewStyle().
			Width(width)
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

// formatArgs converts a JSON object string into a compact key: value representation.
// {"command": "ls -la", "timeout": 60} → command: "ls -la", timeout: 60
func formatArgs(argsJSON string) string {
	argsJSON = strings.TrimSpace(argsJSON)
	if argsJSON == "" || argsJSON == "{}" {
		return ""
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return argsJSON
	}

	if len(m) == 0 {
		return ""
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		v := m[k]
		switch val := v.(type) {
		case string:
			parts = append(parts, fmt.Sprintf("%s: %q", k, val))
		case float64:
			if val == float64(int64(val)) {
				parts = append(parts, fmt.Sprintf("%s: %d", k, int64(val)))
			} else {
				parts = append(parts, fmt.Sprintf("%s: %g", k, val))
			}
		case bool:
			parts = append(parts, fmt.Sprintf("%s: %t", k, val))
		default:
			parts = append(parts, fmt.Sprintf("%s: %v", k, val))
		}
	}

	return strings.Join(parts, ", ")
}

func padLeft(text string, spaces int) string {
	if spaces <= 0 {
		return text
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
