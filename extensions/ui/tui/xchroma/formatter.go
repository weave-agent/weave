package xchroma

import (
	"fmt"
	"image/color"
	"io"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
)

// NewFormatter returns a Chroma formatter that uses Lip Gloss v2 styles for
// syntax highlighting. If bgColor is non-nil, every token's background is
// forced to that color so code blocks blend with the surrounding UI.
func NewFormatter(bgColor color.Color) chroma.Formatter {
	return chroma.FormatterFunc(func(w io.Writer, style *chroma.Style, it chroma.Iterator) error {
		for token := it(); token != chroma.EOF; token = it() {
			entry := style.Get(token.Type)
			value := token.Value

			if entry.IsZero() {
				if _, err := fmt.Fprint(w, value); err != nil {
					return err
				}
				continue
			}

			s := lipgloss.NewStyle()
			if bgColor != nil {
				s = s.Background(bgColor)
			}

			if entry.Bold == chroma.Yes {
				s = s.Bold(true)
			}
			if entry.Underline == chroma.Yes {
				s = s.Underline(true)
			}
			if entry.Italic == chroma.Yes {
				s = s.Italic(true)
			}
			if entry.Colour.IsSet() {
				s = s.Foreground(lipgloss.Color(entry.Colour.String()))
			}

			if _, err := fmt.Fprint(w, s.Render(value)); err != nil {
				return err
			}
		}
		return nil
	})
}
