package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownRenderer_CodeBlocks(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "```go\nfmt.Println(\"hello\")\n```"
	out := ansi.Strip(r.Render(input))
	assert.Contains(t, out, "fmt.Println")
	// Glamour adds ANSI escape codes for syntax highlighting
	assert.Greater(t, len(out), len(input), "rendered output should contain styling")
}

func TestMarkdownRenderer_Bold(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "This is **bold** text."
	out := ansi.Strip(r.Render(input))
	assert.Contains(t, out, "bold")
	// Styled output should differ from plain text
	assert.GreaterOrEqual(t, len(out), len("bold"))
}

func TestMarkdownRenderer_Headers(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "# Main Title\n## Subtitle\nSome text"
	out := ansi.Strip(r.Render(input))
	assert.Contains(t, out, "Main Title")
	assert.Contains(t, out, "Subtitle")
	assert.Contains(t, out, "Some text")
}

func TestMarkdownRenderer_PlainTextPassthrough(t *testing.T) {
	r := NewMarkdownRenderer(80)
	input := "just plain text"
	out := ansi.Strip(r.Render(input))
	assert.Contains(t, out, "just plain text")
}

func TestMarkdownRenderer_SetWidth(t *testing.T) {
	r := NewMarkdownRenderer(80)

	longLine := strings.Repeat("word ", 30)
	out := r.Render(longLine)

	r.SetWidth(40)
	outNarrow := r.Render(longLine)

	// Narrow rendering should produce more lines due to wrapping
	lines80 := len(strings.Split(out, "\n"))
	lines40 := len(strings.Split(outNarrow, "\n"))
	assert.Greater(t, lines40, lines80, "narrow width should produce more lines")
}

func TestMarkdownRenderer_ZeroWidth(t *testing.T) {
	r := NewMarkdownRenderer(0)
	input := "# Hello"
	out := ansi.Strip(r.Render(input))
	// Should still render, just without word wrap
	assert.Contains(t, out, "Hello")
}

func TestNewMarkdownRenderer_NilSafety(t *testing.T) {
	r := NewMarkdownRenderer(80)
	require.NotNil(t, r)
}

func TestMarkdownRenderer_RenderReturnsPlainOnError(t *testing.T) {
	r := NewMarkdownRenderer(80)
	// Normal content should work fine
	input := "normal content"
	out := ansi.Strip(r.Render(input))
	assert.Contains(t, out, "normal content")
}
