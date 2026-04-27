package styles

import (
	"image/color"

	"weave/sdk"

	lipgloss "charm.land/lipgloss/v2"
)

// Colors holds all ANSI 256-color values used by the TUI theme.
type Colors struct {
	// Brand
	Purple      color.Color
	LightPurple color.Color

	// Semantic
	Success color.Color
	Warning color.Color
	Error   color.Color

	// Neutral
	DimGray     color.Color
	MediumGray  color.Color
	Gray        color.Color
	LightGray   color.Color
	NearWhite   color.Color
	BrightWhite color.Color

	// Diff
	DiffAdded   color.Color
	DiffRemoved color.Color
	DiffHeader  color.Color
	DiffHunk    color.Color

	// Context percentage
	PctHealthy  color.Color
	PctWarning  color.Color
	PctCritical color.Color
}

// DefaultColors returns the standard TUI color palette.
func DefaultColors() Colors {
	return Colors{
		Purple:      lipgloss.Color("63"),
		LightPurple: lipgloss.Color("99"),
		Success:     lipgloss.Color("2"),
		Warning:     lipgloss.Color("220"),
		Error:       lipgloss.Color("1"),
		DimGray:     lipgloss.Color("8"),
		MediumGray:  lipgloss.Color("243"),
		Gray:        lipgloss.Color("245"),
		LightGray:   lipgloss.Color("246"),
		NearWhite:   lipgloss.Color("252"),
		BrightWhite: lipgloss.Color("15"),
		DiffAdded:   lipgloss.Color("2"),
		DiffRemoved: lipgloss.Color("1"),
		DiffHeader:  lipgloss.Color("6"),
		DiffHunk:    lipgloss.Color("5"),
		PctHealthy:  lipgloss.Color("82"),
		PctWarning:  lipgloss.Color("220"),
		PctCritical: lipgloss.Color("196"),
	}
}

// ThinkingBorderColor returns the border color for a thinking level.
func ThinkingBorderColor(level sdk.ThinkingLevel) color.Color {
	switch level {
	case sdk.ThinkingMinimal:
		return lipgloss.Color("246")
	case sdk.ThinkingLow:
		return lipgloss.Color("67")
	case sdk.ThinkingMedium:
		return lipgloss.Color("99")
	case sdk.ThinkingHigh:
		return lipgloss.Color("139")
	case sdk.ThinkingXHigh:
		return lipgloss.Color("177")
	default:
		return lipgloss.Color("240")
	}
}

// Theme holds all color and pre-built Lip Gloss v2 style definitions for the TUI.
// Components set Width/Height on these base styles at render time.
type Theme struct {
	Colors Colors

	// Footer styles
	Footer lipgloss.Style

	// Hint/status bar
	Hints         lipgloss.Style
	StatusMessage lipgloss.Style

	// Editor
	EditorBorder    lipgloss.Style
	Autocomplete    lipgloss.Style
	AutocompleteSel lipgloss.Style

	// Spinner
	Spinner lipgloss.Style

	// Thinking block
	ThinkingDim lipgloss.Style

	// Tool panel
	ToolPending   lipgloss.Style
	ToolSuccess   lipgloss.Style
	ToolError     lipgloss.Style
	ToolDim       lipgloss.Style
	ToolErrorBody lipgloss.Style

	// Diff rendering
	DiffAdded   lipgloss.Style
	DiffRemoved lipgloss.Style
	DiffContext lipgloss.Style
	DiffHeader  lipgloss.Style
	DiffHunk    lipgloss.Style

	// Overlay dialogs (selector, confirm, input)
	OverlayBorder      lipgloss.Style
	OverlayTitle       lipgloss.Style
	OverlayFilter      lipgloss.Style
	OverlaySelected    lipgloss.Style
	OverlayNormal      lipgloss.Style
	OverlaySubtitle    lipgloss.Style
	OverlayActiveBtn   lipgloss.Style
	OverlayInactiveBtn lipgloss.Style
	OverlayMessage     lipgloss.Style
	OverlayPrompt      lipgloss.Style
	OverlayInput       lipgloss.Style
	OverlayHint        lipgloss.Style
}

// DefaultTheme returns the standard TUI theme with all pre-built styles.
func DefaultTheme() Theme {
	c := DefaultColors()

	return Theme{
		Colors: c,

		Footer: lipgloss.NewStyle().Foreground(c.Gray),

		Hints:         lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		StatusMessage: lipgloss.NewStyle().Foreground(c.Gray),

		EditorBorder: lipgloss.NewStyle().
			BorderForeground(c.Purple).
			Border(lipgloss.NormalBorder()).
			PaddingLeft(1),
		Autocomplete: lipgloss.NewStyle().
			Foreground(c.BrightWhite).
			Background(c.Purple),
		AutocompleteSel: lipgloss.NewStyle().
			Foreground(c.BrightWhite).
			Background(c.LightPurple),

		Spinner: lipgloss.NewStyle().Foreground(c.LightPurple),

		ThinkingDim: lipgloss.NewStyle().Faint(true),

		ToolPending: lipgloss.NewStyle().
			BorderForeground(c.DimGray).
			Border(lipgloss.RoundedBorder()),
		ToolSuccess: lipgloss.NewStyle().
			BorderForeground(c.Success).
			Border(lipgloss.RoundedBorder()),
		ToolError: lipgloss.NewStyle().
			BorderForeground(c.Error).
			Border(lipgloss.RoundedBorder()),
		ToolDim:       lipgloss.NewStyle().Faint(true),
		ToolErrorBody: lipgloss.NewStyle().Foreground(c.Error),

		DiffAdded:   lipgloss.NewStyle().Foreground(c.DiffAdded),
		DiffRemoved: lipgloss.NewStyle().Foreground(c.DiffRemoved),
		DiffContext: lipgloss.NewStyle().Faint(true),
		DiffHeader:  lipgloss.NewStyle().Foreground(c.DiffHeader),
		DiffHunk:    lipgloss.NewStyle().Foreground(c.DiffHunk),

		OverlayBorder: lipgloss.NewStyle().
			BorderForeground(c.Purple).
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1),
		OverlayTitle: lipgloss.NewStyle().
			Foreground(c.BrightWhite).
			Bold(true),
		OverlayFilter: lipgloss.NewStyle().
			Foreground(c.LightPurple),
		OverlaySelected: lipgloss.NewStyle().
			Foreground(c.BrightWhite).
			Background(c.Purple),
		OverlayNormal: lipgloss.NewStyle().
			Foreground(c.NearWhite),
		OverlaySubtitle: lipgloss.NewStyle().
			Foreground(c.MediumGray),
		OverlayActiveBtn: lipgloss.NewStyle().
			Foreground(c.BrightWhite).
			Background(c.Purple).
			Padding(0, 2),
		OverlayInactiveBtn: lipgloss.NewStyle().
			Foreground(c.MediumGray).
			Padding(0, 2),
		OverlayMessage: lipgloss.NewStyle().
			Foreground(c.BrightWhite),
		OverlayPrompt: lipgloss.NewStyle().
			Foreground(c.BrightWhite),
		OverlayInput: lipgloss.NewStyle().
			Foreground(c.NearWhite),
		OverlayHint: lipgloss.NewStyle().
			Foreground(c.MediumGray),
	}
}
