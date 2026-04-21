package messages

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssistantMessage_Streaming(t *testing.T) {
	m := NewAssistantMessage()
	assert.True(t, m.IsStreaming())
	assert.Equal(t, "", m.Content())
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
	assert.True(t, len(view) > len("Hello World"))
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
