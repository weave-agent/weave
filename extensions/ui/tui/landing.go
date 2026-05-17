package tui

import (
	"fmt"
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// LandingModel renders a landing screen shown before the first prompt.
// It displays the weave logo, current model/provider info, loaded extensions,
// and keybinding hints.
type LandingModel struct {
	model      string
	provider   string
	extensions []string
	width      int
	height     int
}

// NewLandingModel creates a landing model with the given model, provider, and extensions.
func NewLandingModel(model, provider string, extensions []string) LandingModel {
	return LandingModel{
		model:      model,
		provider:   provider,
		extensions: extensions,
	}
}

// SetSize updates the landing model's available dimensions.
func (m LandingModel) SetSize(width, height int) LandingModel {
	m.width = width
	m.height = height

	return m
}

// Draw renders the landing screen into the given screen buffer area.
func (m LandingModel) Draw(scr uv.Screen, area uv.Rectangle, theme *palette.Theme) {
	if area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	if theme == nil {
		theme = palette.DefaultTheme()
	}

	w := area.Dx()
	lines := m.buildLines()

	// Vertically center if there's room
	y := area.Min.Y
	if area.Dy() > len(lines) {
		y = area.Min.Y + (area.Dy()-len(lines))/2
	}

	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent)).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Border))
	ruleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Border))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.MutedBright))

	for i, line := range lines {
		if y+i >= area.Max.Y {
			break
		}

		var rendered string

		switch {
		case strings.HasPrefix(line, "name:"):
			rendered = nameStyle.Render(strings.TrimPrefix(line, "name:"))
		case strings.HasPrefix(line, "hint:"):
			rendered = hintStyle.Render(strings.TrimPrefix(line, "hint:"))
		case strings.HasPrefix(line, "rule:"):
			// Render a horizontal rule that spans most of the width
			ruleWidth := min(w-4, 40)
			if ruleWidth > 0 {
				rendered = ruleStyle.Render(strings.Repeat("─", ruleWidth))
			}
		case strings.HasPrefix(line, "label:"):
			rendered = labelStyle.Render(strings.TrimPrefix(line, "label:"))
		case strings.HasPrefix(line, "muted:"):
			rendered = mutedStyle.Render(strings.TrimPrefix(line, "muted:"))
		default:
			rendered = line
		}

		r := uv.Rect(area.Min.X, y+i, w, 1)
		uv.NewStyledString(rendered).Draw(scr, r)
	}
}

func (m LandingModel) buildLines() []string {
	lines := append([]string{}, m.logo()...)

	if m.model != "" {
		label := fmt.Sprintf("  %s (%s)", m.model, m.provider)
		lines = append(lines, "", "name:"+label)
	}

	if len(m.extensions) > 0 {
		lines = append(lines, "", "rule:", "label:  extensions")
		for _, extLine := range wrapList(m.extensions, m.width) {
			lines = append(lines, "muted:"+extLine)
		}
	}

	lines = append(
		lines,
		"",
		"hint:  ctrl+p model  ·  ctrl+l select  ·  shift+tab thinking",
		"hint:  ctrl+n new  ·  ctrl+o expand  ·  ctrl+t toggle",
	)

	return lines
}

// wrapList formats a list of items as comma-separated strings, wrapping when
// the line exceeds the available width. Each continuation line is prefixed with 4 spaces.
func wrapList(items []string, width int) []string {
	if len(items) == 0 {
		return nil
	}

	const prefix = "    "

	if width <= 0 {
		return []string{prefix + strings.Join(items, ", ")}
	}

	var (
		lines []string
		b     strings.Builder
	)

	for i, item := range items {
		if i > 0 {
			b.WriteString(", ")
		}

		b.WriteString(item)
	}

	text := b.String()
	maxLine := max(width-len(prefix), 10)

	for text != "" {
		if len(text) <= maxLine {
			lines = append(lines, prefix+text)
			break
		}

		cut := maxLine
		for cut > 0 && text[cut] != ' ' && text[cut] != ',' {
			cut--
		}

		if cut == 0 {
			cut = maxLine
		}

		lines = append(lines, prefix+text[:cut])
		text = strings.TrimLeft(text[cut:], " ")
	}

	return lines
}

func (m LandingModel) logo() []string {
	return []string{
		"",
		" █████ ███ █████  ██████   ██████   █████ █████  ██████ ",
		"░░███ ░███░░███  ███░░███ ░░░░░███ ░░███ ░░███  ███░░███",
		" ░███ ░███ ░███ ░███████   ███████  ░███  ░███ ░███████ ",
		" ░░███████████  ░███░░░   ███░░███  ░░███ ███  ░███░░░  ",
		"  ░░████░████   ░░██████ ░░████████  ░░█████   ░░██████ ",
		"   ░░░░ ░░░░     ░░░░░░   ░░░░░░░░    ░░░░░     ░░░░░░ ",
	}
}
