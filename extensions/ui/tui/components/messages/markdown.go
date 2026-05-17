package messages

import (
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/charmbracelet/glamour"

	"github.com/weave-agent/weave-tui/xchroma"
)

const chromaFormatterName = "weave"

// init registers the custom Chroma formatter so glamour can use it by name.
func init() {
	formatters.Register(chromaFormatterName, xchroma.NewFormatter())
}

// MarkdownRenderer wraps a glamour renderer for terminal-aware markdown rendering.
type MarkdownRenderer struct {
	mu       sync.Mutex
	renderer *glamour.TermRenderer
	width    int
}

// NewMarkdownRenderer creates a new markdown renderer with the given width.
func NewMarkdownRenderer(width int) *MarkdownRenderer {
	r := &MarkdownRenderer{width: width}
	r.rebuild()

	return r
}

// SetWidth updates the word-wrap width and rebuilds the renderer.
func (r *MarkdownRenderer) SetWidth(width int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.width != width {
		r.width = width
		r.rebuild()
	}
}

// Render converts markdown text to styled terminal output.
// If rendering fails, returns the plain text.
func (r *MarkdownRenderer) Render(text string) string {
	r.mu.Lock()
	renderer := r.renderer
	r.mu.Unlock()

	if renderer == nil {
		return text
	}

	out, err := renderer.Render(text)
	if err != nil {
		return text
	}

	return strings.TrimSpace(out)
}

// rebuild recreates the glamour renderer with the current width.
func (r *MarkdownRenderer) rebuild() {
	opts := []glamour.TermRendererOption{
		glamour.WithStandardStyle("dark"),
		glamour.WithChromaFormatter(chromaFormatterName),
	}
	if r.width > 0 {
		opts = append(opts, glamour.WithWordWrap(r.width))
	}

	renderer, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		r.renderer = nil
		return
	}

	r.renderer = renderer
}
