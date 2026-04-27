package messages

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

// drawView writes a pre-rendered text string into a screen buffer region,
// splitting on newlines and clipping to the rectangle bounds.
func drawView(scr uv.Screen, area uv.Rectangle, text string) {
	if area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	for i, line := range strings.Split(text, "\n") {
		if i >= area.Dy() {
			break
		}

		lineRect := uv.Rect(area.Min.X, area.Min.Y+i, area.Dx(), 1)
		uv.NewStyledString(line).Draw(scr, lineRect)
	}
}
