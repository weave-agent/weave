package diffviewer

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	tui "github.com/weave-agent/weave-tui"
	"github.com/weave-agent/weave/sdk"
)

func init() {
	tui.RegisterTUIExtension("diff-viewer", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (tui.TUIExtension, error) {
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

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent))
	hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.AccentBright))
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

// wrapLine splits a line into chunks of at most width display cells.
// A width of zero or less disables wrapping.
func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}

	if lipgloss.Width(line) <= width {
		return []string{line}
	}

	var result []string

	start := 0

	for start < len(line) {
		end := start
		w := 0

		for end < len(line) {
			r, size := utf8.DecodeRuneInString(line[end:])

			rw := runewidth.RuneWidth(r)

			if w+rw > width {
				break
			}

			w += rw
			end += size
		}

		if end == start {
			// Fallback for a single rune wider than width
			_, size := utf8.DecodeRuneInString(line[start:])
			end = start + size
		}

		result = append(result, line[start:end])
		start = end
	}

	return result
}
