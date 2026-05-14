package tui

import (
	"weave/ext/ui/tui/palette"
	"weave/sdk"
)

// TUIExtAPI provides TUI-specific extension capabilities.
// This is a minimal definition that will be expanded in Task 8.
type TUIExtAPI interface {
	RegisterTheme(name string, theme ThemeDef) error
	Theme() sdk.ThemeInfo
}

// ThemeDef defines a custom theme for registration.
type ThemeDef struct {
	Primary               string
	PrimaryDim            string
	PrimaryBright         string
	Success               string
	Error                 string
	Warning               string
	Muted                 string
	MutedBright           string
	Border                string
	BorderFocused         string
	BackgroundTint        string
	BackgroundTintPending string
	BackgroundTintSuccess string
	BackgroundTintError   string
	Foreground            string
	ForegroundBright      string
}

// toPaletteTheme converts a ThemeDef to a palette.Theme.
func (td ThemeDef) toPaletteTheme() *palette.Theme {
	return &palette.Theme{
		Primary:               td.Primary,
		PrimaryDim:            td.PrimaryDim,
		PrimaryBright:         td.PrimaryBright,
		Success:               td.Success,
		Error:                 td.Error,
		Warning:               td.Warning,
		Muted:                 td.Muted,
		MutedBright:           td.MutedBright,
		Border:                td.Border,
		BorderFocused:         td.BorderFocused,
		BackgroundTint:        td.BackgroundTint,
		BackgroundTintPending: td.BackgroundTintPending,
		BackgroundTintSuccess: td.BackgroundTintSuccess,
		BackgroundTintError:   td.BackgroundTintError,
		Foreground:            td.Foreground,
		ForegroundBright:      td.ForegroundBright,
	}
}
