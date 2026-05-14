package diffviewer

import (
	"strings"

	"charm.land/lipgloss/v2"

	"weave/sdk"
)

func init() {
	sdk.RegisterUIExtension("diff-viewer", func(_ sdk.Config, _ struct{}) (sdk.UIExtension, error) {
		return &DiffViewer{}, nil
	})
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

var (
	diffHeaderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	diffHunkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	diffAddStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	diffRemoveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	diffContextStyle = lipgloss.NewStyle().Faint(true)
)

// diffRenderer renders unified diff output with color coding.
type diffRenderer struct{}

// Render applies diff color coding to the content. The width parameter is
// intentionally ignored; lipgloss handles terminal width automatically.
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
			rendered = diffHeaderStyle.Render(line)
		case strings.HasPrefix(line, "@@"):
			rendered = diffHunkStyle.Render(line)
		case strings.HasPrefix(line, "+"):
			rendered = diffAddStyle.Render(line)
		case strings.HasPrefix(line, "-"):
			rendered = diffRemoveStyle.Render(line)
		default:
			rendered = diffContextStyle.Render(line)
		}

		bldr.WriteString(rendered)
	}

	return bldr.String()
}
