package tui

import (
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/mattn/go-runewidth"
)

// PanelTab represents a single tab in the panel tray.
type PanelTab struct {
	ID    string
	Title string
}

// PanelTray renders a tab strip for visible panels.
type PanelTray struct {
	tabs      []PanelTab
	activeIdx int
	width     int
	focused   bool
}

// NewPanelTray creates a new PanelTray.
func NewPanelTray() PanelTray {
	return PanelTray{activeIdx: -1}
}

// SetTabs updates the tray tabs and active index.
func (pt PanelTray) SetTabs(tabs []PanelTab, activeIdx int) PanelTray {
	pt.tabs = tabs
	pt.activeIdx = activeIdx

	return pt
}

// SetSize updates the tray width.
func (pt PanelTray) SetSize(width int) PanelTray {
	pt.width = width
	return pt
}

// SetFocused sets whether the tray has keyboard focus.
func (pt PanelTray) SetFocused(focused bool) PanelTray {
	pt.focused = focused
	return pt
}

// ActiveID returns the ID of the active tab, or empty string if none.
func (pt PanelTray) ActiveID() string {
	if pt.activeIdx < 0 || pt.activeIdx >= len(pt.tabs) {
		return ""
	}

	return pt.tabs[pt.activeIdx].ID
}

// Next cycles to the next tab.
func (pt PanelTray) Next() PanelTray {
	if len(pt.tabs) == 0 {
		return pt
	}

	pt.activeIdx = (pt.activeIdx + 1) % len(pt.tabs)

	return pt
}

// Prev cycles to the previous tab.
func (pt PanelTray) Prev() PanelTray {
	if len(pt.tabs) == 0 {
		return pt
	}

	pt.activeIdx = (pt.activeIdx - 1 + len(pt.tabs)) % len(pt.tabs)

	return pt
}

// Len returns the number of tabs.
func (pt PanelTray) Len() int {
	return len(pt.tabs)
}

// IsFocused returns true if the tray has focus.
func (pt PanelTray) IsFocused() bool {
	return pt.focused
}

// Draw renders the tab strip into the given area.
func (pt PanelTray) Draw(scr uv.Screen, area uv.Rectangle, theme *palette.Theme) {
	if len(pt.tabs) == 0 {
		return
	}

	rowStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(theme.BackgroundTint))
	uv.NewStyledString(rowStyle.Render(strings.Repeat(" ", area.Dx()))).Draw(scr, area)

	var parts []string

	for i, tab := range pt.tabs {
		title := tab.Title
		if title == "" {
			title = tab.ID
		}

		if i == pt.activeIdx {
			if pt.focused {
				parts = append(parts, lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.AccentBright)).
					Background(lipgloss.Color(theme.BackgroundTint)).
					Padding(0, 1).
					Render("["+title+"]"))
			} else {
				parts = append(parts, lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.Accent)).
					Background(lipgloss.Color(theme.BackgroundTint)).
					Padding(0, 1).
					Render(title))
			}
		} else {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Muted)).
				Background(lipgloss.Color(theme.BackgroundTint)).
				Padding(0, 1).
				Render(title))
		}
	}

	line := " " + strings.Join(parts, " ")
	line = truncateDisplayWidth(line, area.Dx())

	uv.NewStyledString(line).Draw(scr, area)
}

// truncateDisplayWidth truncates a string to fit within maxWidth display cells.
// It is ANSI-escape aware (preserves lipgloss styling sequences) and
// runewidth-aware (handles wide CJK characters correctly).
func truncateDisplayWidth(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}

	var b strings.Builder

	w := 0
	inEsc := false

	for _, r := range s {
		if inEsc {
			b.WriteRune(r)

			if r == 'm' {
				inEsc = false
			}

			continue
		}

		if r == '\x1b' {
			inEsc = true

			b.WriteRune(r)

			continue
		}

		rw := runewidth.RuneWidth(r)
		if w+rw > maxWidth {
			break
		}

		w += rw

		b.WriteRune(r)
	}

	return b.String()
}
