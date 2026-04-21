package messages

import "strings"

// AssistantMessage accumulates streaming text deltas into a single message.
type AssistantMessage struct {
	content  strings.Builder
	final    string
	streaming bool
}

// NewAssistantMessage creates a new assistant message in streaming mode.
func NewAssistantMessage() *AssistantMessage {
	return &AssistantMessage{streaming: true}
}

// Append adds a content delta to the streaming message.
func (m *AssistantMessage) Append(delta string) {
	m.content.WriteString(delta)
}

// Finalize marks the message as complete with the final content.
func (m *AssistantMessage) Finalize(content string) {
	m.final = content
	m.streaming = false
}

// Content returns the accumulated content. If finalized, returns the final content.
func (m *AssistantMessage) Content() string {
	if !m.streaming {
		return m.final
	}
	return m.content.String()
}

// IsStreaming returns whether the message is still streaming.
func (m *AssistantMessage) IsStreaming() bool {
	return m.streaming
}

// View renders the assistant message as a string.
func (m *AssistantMessage) View(width int) string {
	return m.Content()
}
