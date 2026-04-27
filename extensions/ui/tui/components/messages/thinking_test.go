package messages

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
)

func TestThinkingBlock_New(t *testing.T) {
	b := NewThinkingBlock("some thinking content")
	assert.Equal(t, "some thinking content", b.Content())
	assert.False(t, b.Expanded())
}

func TestThinkingBlock_ToggleExpanded(t *testing.T) {
	b := NewThinkingBlock("thinking")
	assert.False(t, b.Expanded())

	b.ToggleExpanded()
	assert.True(t, b.Expanded())

	b.ToggleExpanded()
	assert.False(t, b.Expanded())
}

func TestThinkingBlock_SetExpanded(t *testing.T) {
	b := NewThinkingBlock("thinking")
	b.SetExpanded(true)
	assert.True(t, b.Expanded())

	b.SetExpanded(false)
	assert.False(t, b.Expanded())
}

func TestThinkingBlock_View_Collapsed(t *testing.T) {
	b := NewThinkingBlock("deep thoughts about the problem")
	view := b.View(80)
	assert.Contains(t, view, "[thinking]")
	// Collapsed should NOT show content
	assert.NotContains(t, view, "deep thoughts")
}

func TestThinkingBlock_View_Expanded(t *testing.T) {
	b := NewThinkingBlock("deep thoughts about the problem")
	b.ToggleExpanded()
	view := b.View(80)
	assert.Contains(t, view, "[thinking]")
	assert.Contains(t, view, "deep thoughts about the problem")
}

func TestThinkingBlock_View_Collapsed_ShowsLineCount(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5"
	b := NewThinkingBlock(content)
	view := b.View(80)
	assert.Contains(t, view, "[thinking]")
	assert.Contains(t, view, "5 lines")
}

func TestThinkingBlock_View_MultilineExpanded(t *testing.T) {
	content := "first thought\nsecond thought\nthird thought"
	b := NewThinkingBlock(content)
	b.ToggleExpanded()
	view := b.View(80)
	assert.Contains(t, view, "first thought")
	assert.Contains(t, view, "second thought")
	assert.Contains(t, view, "third thought")
}

func TestThinkingBlock_View_EmptyContent(t *testing.T) {
	b := NewThinkingBlock("")
	view := b.View(80)
	assert.Contains(t, view, "[thinking]")
}

func TestThinkingBlock_View_ZeroWidth(t *testing.T) {
	b := NewThinkingBlock("thinking content")
	// Should not panic with zero width
	view := b.View(0)
	assert.Contains(t, view, "[thinking]")
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

func TestFormatThinkingLabel(t *testing.T) {
	tests := []struct {
		name      string
		lineCount int
		expect    string
	}{
		{"zero lines", 0, "  [thinking]"},
		{"one line", 1, "  [thinking] (1 lines)"},
		{"many lines", 42, "  [thinking] (42 lines)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, FormatThinkingLabel(tt.lineCount))
		})
	}
}

func TestThinkingBlock_Draw_Collapsed(t *testing.T) {
	b := NewThinkingBlock("secret thoughts")
	canvas := uv.NewScreenBuffer(80, 5)
	b.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "[thinking]")
	assert.NotContains(t, output, "secret thoughts")
}

func TestThinkingBlock_Draw_Expanded(t *testing.T) {
	b := NewThinkingBlock("deep thoughts\nmore thoughts")
	b.ToggleExpanded()

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
