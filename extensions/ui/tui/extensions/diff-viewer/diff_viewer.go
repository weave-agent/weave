package diffviewer

import (
	"strings"

	"charm.land/lipgloss/v2"

	"weave/ext/ui/tui"
	"weave/sdk"
)

func init() {
	tui.RegisterTUIExtension("diff-viewer", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (tui.TUIExtension, error) {
		return &DiffViewer{}, nil
	})
}

// DiffViewer is a TUI extension that registers a theme-aware diff renderer
// for edit tool output.
type DiffViewer struct{}

// Name returns the extension name.
func (d *DiffViewer) Name() string { return "diff-viewer" }

// RegisterTUI wires the rich diff renderer into the TUI.
func (d *DiffViewer) RegisterTUI(api tui.TUIExtAPI) {
	api.RegisterRichRenderer("edit", &richDiffRenderer{})
}

// richDiffRenderer renders unified diff output with theme-aware color coding.
type richDiffRenderer struct{}

// Render applies diff color coding using theme-aligned colors.
func (r *richDiffRenderer) Render(content string, theme sdk.ThemeInfo, width int) string {
	if content == "" {
		return ""
	}

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Primary))
	hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.PrimaryBright))
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success))
	removeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Error))
	contextStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))

	lines := strings.Split(content, "\n")
	var bldr strings.Builder

	for i, line := range lines {
		if i > 0 {
			bldr.WriteString("\n")
		}

		var rendered string

		switch {
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			rendered = headerStyle.Render(line)
		case strings.HasPrefix(line, "@@"):
			rendered = hunkStyle.Render(line)
		case strings.HasPrefix(line, "+"):
			rendered = addStyle.Render(line)
		case strings.HasPrefix(line, "-"):
			rendered = removeStyle.Render(line)
		default:
			rendered = contextStyle.Render(line)
		}

		bldr.WriteString(rendered)
	}

	return bldr.String()
}
