package messages

import (
	"fmt"
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// CompactionEntry renders a compaction summary in the chat area.
type CompactionEntry struct {
	summarized   int
	tokensBefore int
	tokensAfter  int
}

// NewCompactionEntry creates a new compaction chat entry.
func NewCompactionEntry(summarized, tokensBefore, tokensAfter int) *CompactionEntry {
	return &CompactionEntry{
		summarized:   summarized,
		tokensBefore: tokensBefore,
		tokensAfter:  tokensAfter,
	}
}

// View renders the compaction entry.
func (e *CompactionEntry) View(width int) string {
	if width <= 0 {
		width = 80
	}

	theme := palette.DefaultTheme()

	if GetThemeInfo != nil {
		info := GetThemeInfo()
		theme = &palette.Theme{
			Muted: info.Muted,
		}
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Muted)).
		Width(width)

	saved := e.tokensBefore - e.tokensAfter
	saved = max(saved, 0)

	detail := fmt.Sprintf("%d messages summarized (%d → %d tokens, %d saved)",
		e.summarized, e.tokensBefore, e.tokensAfter, saved)

	var bldr strings.Builder
	bldr.WriteString(headerStyle.Render("◫ Compacted"))
	bldr.WriteString("\n")

	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Muted)).
		Width(width - 2)

	bldr.WriteString("  " + contentStyle.Render(detail))
	bldr.WriteString("\n")

	return strings.TrimRight(bldr.String(), "\n")
}

// Draw renders the compaction entry into a screen buffer region.
func (e *CompactionEntry) Draw(scr uv.Screen, area uv.Rectangle) {
	drawView(scr, area, e.View(area.Dx()))
}
