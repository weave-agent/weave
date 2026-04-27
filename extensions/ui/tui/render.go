package tui

import (
	uv "github.com/charmbracelet/ultraviolet"
)

// Drawable is implemented by TUI components that render into a screen buffer region.
type Drawable interface {
	Draw(scr uv.Screen, area uv.Rectangle)
}

// DrawableFunc adapts a function to the Drawable interface.
type DrawableFunc func(scr uv.Screen, area uv.Rectangle)

func (f DrawableFunc) Draw(scr uv.Screen, area uv.Rectangle) { f(scr, area) }

// DrawOpts controls the layout computation for a Draw call.
type DrawOpts struct {
	EditorLines int // content lines for the editor (border adds 2)
	HeaderRows  int // 0-2: hints, landing header
	PillRows    int // 0-1: spinner, status message
}

// Composer renders the full TUI layout into a screen buffer.
// It computes the layout and delegates Draw to each component in its region.
type Composer struct {
	Engine LayoutEngine
}

// NewComposer creates a Composer with a default LayoutEngine.
func NewComposer() Composer {
	return Composer{Engine: NewLayoutEngine()}
}

// Draw computes layout from the screen bounds and delegates rendering
// to each component within its allocated rectangle.
func (c Composer) Draw(
	scr uv.Screen,
	header, main, pills, editor, footer Drawable,
	opts DrawOpts,
) {
	lt := c.Engine.ComputeFull(
		scr.Bounds().Dx(), scr.Bounds().Dy(),
		opts.EditorLines, opts.HeaderRows, opts.PillRows,
	)

	drawNonEmpty(header, lt.Header, scr)
	drawNonEmpty(main, lt.Main, scr)
	drawNonEmpty(pills, lt.Pills, scr)
	drawNonEmpty(editor, lt.Editor, scr)
	drawNonEmpty(footer, lt.Footer, scr)
}

// drawNonEmpty calls Draw only if the component and its area are both valid.
func drawNonEmpty(d Drawable, area uv.Rectangle, scr uv.Screen) {
	if d == nil || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	d.Draw(scr, area)
}
