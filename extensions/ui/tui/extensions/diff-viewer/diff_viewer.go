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

		wrapped := wrapLine(line, width)
		for j, wl := range wrapped {
			if j > 0 {
				bldr.WriteString("\n")
			}

			var rendered string

			switch {
			case strings.HasPrefix(wl, "---") || strings.HasPrefix(wl, "+++"):
				rendered = headerStyle.Render(wl)
			case strings.HasPrefix(wl, "@@"):
				rendered = hunkStyle.Render(wl)
			case strings.HasPrefix(wl, "+"):
				rendered = addStyle.Render(wl)
			case strings.HasPrefix(wl, "-"):
				rendered = removeStyle.Render(wl)
			default:
				rendered = contextStyle.Render(wl)
			}

			bldr.WriteString(rendered)
		}
	}

	return bldr.String()
}

// wrapLine splits a line into chunks of at most width runes.
// A width of zero or less disables wrapping.
func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}

	runes := []rune(line)
	if len(runes) <= width {
		return []string{line}
	}

	var result []string
	for len(runes) > width {
		result = append(result, string(runes[:width]))
		runes = runes[width:]
	}
	result = append(result, string(runes))
	return result
}
