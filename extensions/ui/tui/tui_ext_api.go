package tui

import (
	"time"

	"weave/ext/ui/tui/palette"
	"weave/sdk"

	uv "github.com/charmbracelet/ultraviolet"
)

// TUIExtAPI provides TUI-specific extension capabilities.
// Extensions that need deeper TUI integration implement TUIExtension and
// receive this API during wiring.
type TUIExtAPI interface {
	// Panels
	ShowPanel(config PanelConfig, drawer PanelDrawer)
	HidePanel(id string)
	RemovePanel(id string)
	PanelVisible(id string) bool
	PanelTray() PanelTrayAPI

	// Read-only
	Theme() sdk.ThemeInfo
	Size() (int, int)

	// Editor
	EditorText() string
	SetEditorText(text string)
	PasteToEditor(text string)

	// Rendering
	RegisterRichRenderer(tool string, renderer RichToolRenderer)
	RegisterMessageRenderer(msgType string, renderer sdk.MessageRenderer)

	// Footer/Header
	SetFooter(component TUIComponent)
	SetHeader(component TUIComponent)

	// Input
	OnTerminalInput(handler func(KeyEvent))
	AddAutocomplete(provider AutocompleteProvider)

	// Cosmetic
	SetWorkingFrames(frames []string, interval time.Duration)
	RegisterTheme(name string, theme ThemeDef) error

	// Redraw
	RequestRedraw()
}

// TUIExtension is a TUI-specific plugin that registers with the TUI's
// extension API. TUI extensions are discovered by the launcher and wired
// by the TUI at startup. They are silently skipped in headless mode.
type TUIExtension interface {
	Name() string
	RegisterTUI(api TUIExtAPI)
}

// PanelTrayAPI provides access to the panel tray for tab ordering.
type PanelTrayAPI interface {
	SetOrder(ids []string)
	GetOrder() []string
}

// RichToolRenderer renders tool output with theme access.
type RichToolRenderer interface {
	Render(content string, theme sdk.ThemeInfo, width int) string
}

// TUIComponent is a replaceable UI component (header/footer).
type TUIComponent interface {
	Draw(scr uv.Screen, area uv.Rectangle)
}

// KeyEvent represents a raw terminal key event.
type KeyEvent struct {
	Code   rune
	Mod    int
	String string
}

// AutocompleteProvider provides completion suggestions for the editor.
type AutocompleteProvider interface {
	Name() string
	Suggestions(ctx AutocompleteContext) []AutocompleteSuggestion
}

// AutocompleteContext provides context for autocomplete suggestions.
type AutocompleteContext struct {
	Text   string
	Cursor int
	Line   string
}

// AutocompleteSuggestion is a single autocomplete suggestion.
type AutocompleteSuggestion struct {
	Label       string
	Description string
	Value       string
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
