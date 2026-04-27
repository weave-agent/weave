package messages

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
)

func TestAssistantMessage_Streaming(t *testing.T) {
	m := NewAssistantMessage()
	assert.True(t, m.IsStreaming())
	assert.Empty(t, m.Content())
}

func TestAssistantMessage_Append(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("hello ")
	m.Append("world")
	assert.Equal(t, "hello world", m.Content())
	assert.True(t, m.IsStreaming())
}

func TestAssistantMessage_Finalize(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("streaming text")
	m.Finalize("final content")
	assert.False(t, m.IsStreaming())
	assert.Equal(t, "final content", m.Content())
}

func TestAssistantMessage_FinalizeOverwritesStreamed(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("partial")
	m.Finalize("complete response")
	assert.Equal(t, "complete response", m.Content())
	assert.False(t, m.IsStreaming())
}

func TestAssistantMessage_View_Streaming_PlainText(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("hello")
	// Streaming messages return plain text without markdown processing
	assert.Equal(t, "hello", m.View(80))
}

func TestAssistantMessage_View_Finalized_Markdown(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("# Hello World\n\nSome **bold** text.")
	view := m.View(80)
	// Finalized messages go through markdown renderer
	// The output should contain the text but with styling applied
	assert.Contains(t, view, "Hello World")
	assert.Contains(t, view, "bold")
	// Markdown output is typically longer due to ANSI codes
	assert.Greater(t, len(view), len("Hello World"))
}

func TestAssistantMessage_View_Finalized_CodeBlock(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("```go\nfmt.Println(\"hi\")\n```")
	view := m.View(80)
	assert.Contains(t, view, "fmt.Println")
}

func TestAssistantMessage_SetWidth(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("# Title")
	// Should not panic when setting width
	m.SetWidth(120)
	view := m.View(80)
	assert.Contains(t, view, "Title")
}

func TestAssistantMessage_Interrupt(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("partial response")
	m.Interrupt()
	assert.False(t, m.IsStreaming())
	assert.True(t, m.Interrupted())
	assert.Contains(t, m.Content(), "partial response")
	assert.Contains(t, m.Content(), "[interrupted]")
}

func TestAssistantMessage_Interrupt_Idempotent(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("partial")
	m.Interrupt()
	content1 := m.Content()
	m.Interrupt() // second call should be no-op
	assert.Equal(t, content1, m.Content())
}

func TestAssistantMessage_Interrupt_NotStreaming(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("done")
	m.Interrupt() // no-op on finalized message
	assert.False(t, m.Interrupted())
	assert.Equal(t, "done", m.Content())
}

func TestAssistantMessage_Draw_Streaming(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("hello world")

	canvas := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "hello world")
}

func TestAssistantMessage_Draw_Finalized(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("# Hello\n\nSome **bold** text.")

	canvas := uv.NewScreenBuffer(80, 10)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "Hello")
}

func TestAssistantMessage_Draw_Multiline(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("line1\nline2\nline3")

	canvas := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	lines := strings.Split(output, "\n")
	assert.GreaterOrEqual(t, len(lines), 3)
}

func TestAssistantMessage_Draw_ClipsToArea(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("line1\nline2\nline3\nline4\nline5")

	canvas := uv.NewScreenBuffer(80, 2)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	lines := strings.Split(output, "\n")
	assert.LessOrEqual(t, len(lines), 2)
}

func TestAssistantMessage_Draw_ZeroArea(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("hello")

	canvas := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas, uv.Rect(0, 0, 0, 0))
}
