package diffviewer

import (
	"strings"

	"charm.land/lipgloss/v2"
	"weave/sdk"
)

func init() {
	sdk.RegisterUIExtension(&DiffViewer{})
}

// DiffViewer is a UI extension that registers a colorized diff renderer
// for edit tool output.
type DiffViewer struct{}

// Name returns the extension name.
func (d *DiffViewer) Name() string { return "diff-viewer" }

// Register wires the diff renderer into the TUI.
func (d *DiffViewer) Register(ui sdk.UI) {
	ui.RegisterRenderer("edit", &diffRenderer{})
}

// diffRenderer renders unified diff output with color coding.
type diffRenderer struct{}

func (r *diffRenderer) Render(content string, width int) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	var bldr strings.Builder

	for i, line := range lines {
		if i > 0 {
			bldr.WriteString("\n")
		}

		var rendered string

		switch {
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			rendered = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(line)
		case strings.HasPrefix(line, "@@"):
			rendered = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render(line)
		case strings.HasPrefix(line, "+"):
			rendered = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(line)
		case strings.HasPrefix(line, "-"):
			rendered = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(line)
		default:
			rendered = lipgloss.NewStyle().Faint(true).Render(line)
		}

		bldr.WriteString(rendered)
	}

	return bldr.String()
}
