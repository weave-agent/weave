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

func TestAssistantMessage_View(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("hello")
	assert.Equal(t, "hello", m.View(80))

	m.Finalize("world")
	assert.Equal(t, "world", m.View(80))
}
