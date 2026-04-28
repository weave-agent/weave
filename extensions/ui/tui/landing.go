package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// LandingModel renders a landing screen shown before the first prompt.
// It displays the weave logo, current model/provider info, keybinding hints,
// and a placeholder prompt.
type LandingModel struct {
	model    string
	provider string
	width    int
	height   int
}

// NewLandingModel creates a landing model with the given model and provider info.
func NewLandingModel(model, provider string) LandingModel {
	return LandingModel{
		model:    model,
		provider: provider,
	}
}

// SetSize updates the landing model's available dimensions.
func (m LandingModel) SetSize(width, height int) LandingModel {
	m.width = width
	m.height = height

	return m
}

// Draw renders the landing screen into the given screen buffer area.
func (m LandingModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	w := area.Dx()
	lines := m.buildLines()

	// Vertically center if there's room
	y := area.Min.Y
	if area.Dy() > len(lines) {
		y = area.Min.Y + (area.Dy()-len(lines))/2
	}

	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	placeholderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

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
		case strings.HasPrefix(line, "placeholder:"):
			rendered = placeholderStyle.Render(strings.TrimPrefix(line, "placeholder:"))
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

	lines = append(lines,
		"",
		"hint:  ctrl+p model  ·  ctrl+l select  ·  shift+tab thinking",
		"hint:  ctrl+n new  ·  ctrl+o expand  ·  ctrl+t toggle",
		"",
		"placeholder:  Type a message to get started...",
	)

	return lines
}

func (m LandingModel) logo() []string {
	return []string{
		"",
		"          ╭──╮",
		"          │╲ ╱│",
		"          │ ╳ │",
		"          │╱ ╲│",
		"          ╰──╯",
	}
}
