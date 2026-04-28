package xchroma

import (
	"fmt"
	"io"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
)

// NewFormatter returns a Chroma formatter that uses Lip Gloss v2 styles for
// syntax highlighting.
func NewFormatter() chroma.Formatter {
	return chroma.FormatterFunc(func(w io.Writer, style *chroma.Style, it chroma.Iterator) error {
		for token := it(); token != chroma.EOF; token = it() {
			entry := style.Get(token.Type)
			value := token.Value

			if entry.IsZero() {
				if _, err := fmt.Fprint(w, value); err != nil {
					return fmt.Errorf("xchroma write: %w", err)
				}

				continue
			}

			s := lipgloss.NewStyle()

			if entry.Bold == chroma.Yes {
				s = s.Bold(true)
			}

			if entry.Underline == chroma.Yes {
				s = s.Underline(true)
			}

			if entry.Italic == chroma.Yes {
				s = s.Italic(true)
			}

			if entry.Colour.IsSet() { //nolint:misspell // Chroma uses British English
				s = s.Foreground(lipgloss.Color(entry.Colour.String())) //nolint:misspell // Chroma uses British English
			}

			if _, err := fmt.Fprint(w, s.Render(value)); err != nil {
				return fmt.Errorf("xchroma write: %w", err)
			}
		}

		return nil
	})
}
