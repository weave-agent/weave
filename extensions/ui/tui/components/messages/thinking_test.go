package messages

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThinkingBlock_New(t *testing.T) {
	b := NewThinkingBlock("some thinking content")
	assert.Equal(t, "some thinking content", b.Content())
}

func TestThinkingBlock_View(t *testing.T) {
	b := NewThinkingBlock("deep thoughts about the problem")
	view := b.View(80)
	assert.Contains(t, view, "∴")
	assert.Contains(t, view, "Thinking")
	assert.Contains(t, view, "deep thoughts about the problem")
}

func TestThinkingBlock_View_Multiline(t *testing.T) {
	content := "first thought\nsecond thought\nthird thought"
	b := NewThinkingBlock(content)
	view := b.View(80)
	assert.Contains(t, view, "∴")
	assert.Contains(t, view, "first thought")
	assert.Contains(t, view, "second thought")
	assert.Contains(t, view, "third thought")
}

func TestThinkingBlock_View_EmptyContent(t *testing.T) {
	b := NewThinkingBlock("")
	view := b.View(80)
	assert.Contains(t, view, "∴")
	assert.Contains(t, view, "Thinking")
}

func TestThinkingBlock_View_ZeroWidth(t *testing.T) {
	b := NewThinkingBlock("thinking content")
	// Should not panic with zero width
	view := b.View(0)
	assert.Contains(t, view, "∴")
	assert.Contains(t, view, "Thinking")
}

func TestThinkingBlock_LineCount(t *testing.T) {
	tests := []struct {
		name    string
		content string
		expect  int
	}{
		{"empty", "", 0},
		{"single line", "one line", 1},
		{"multi line", "a\nb\nc", 3},
		{"trailing newline", "a\nb\n", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewThinkingBlock(tt.content)
			assert.Equal(t, tt.expect, b.LineCount())
		})
	}
}

func TestThinkingBlock_Summary(t *testing.T) {
	tests := []struct {
		name    string
		content string
		maxLen  int
		expect  string
	}{
		{"short", "hello", 20, "hello"},
		{"truncated", strings.Repeat("x", 50), 20, strings.Repeat("x", 17) + "..."},
		{"empty", "", 20, "(empty)"},
		{"first line", "first line\nsecond line", 20, "first line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewThinkingBlock(tt.content)
			assert.Equal(t, tt.expect, b.Summary(tt.maxLen))
		})
	}
}

func TestThinkingBlock_View_HasThereforeSymbol(t *testing.T) {
	b := NewThinkingBlock("deep thoughts")
	view := b.View(80)
	assert.Contains(t, view, "∴")
	assert.Contains(t, view, "Thinking")
	assert.Contains(t, view, "░")
}

func TestThinkingBlock_View_HasBarIndent(t *testing.T) {
	b := NewThinkingBlock("first line\nsecond line")
	view := b.View(80)
	// Content should be indented with "  ░ " prefix
	lines := strings.Split(view, "\n")
	require.GreaterOrEqual(t, len(lines), 3)
	// First line is header with bar, subsequent lines have "  ░ " prefix
	assert.Contains(t, lines[1], "░", "content line should have bar")
	assert.Contains(t, lines[2], "░", "content line should have bar")
}

func TestThinkingBlock_Draw(t *testing.T) {
	b := NewThinkingBlock("deep thoughts\nmore thoughts")
	canvas := uv.NewScreenBuffer(80, 10)
	b.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "deep thoughts")
	assert.Contains(t, output, "more thoughts")
}

func TestThinkingBlock_Draw_ZeroArea(t *testing.T) {
	b := NewThinkingBlock("thinking content")
	canvas := uv.NewScreenBuffer(80, 5)
	b.Draw(canvas, uv.Rect(0, 0, 0, 0))
}
