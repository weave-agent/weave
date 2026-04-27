package messages

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

// AssistantMessage accumulates streaming text deltas into a single message.
type AssistantMessage struct {
	content     strings.Builder
	final       string
	streaming   bool
	interrupted bool
	renderer    *MarkdownRenderer
}

// NewAssistantMessage creates a new assistant message in streaming mode.
func NewAssistantMessage() *AssistantMessage {
	return &AssistantMessage{
		streaming: true,
		renderer:  NewMarkdownRenderer(80),
	}
}

// SetWidth updates the markdown renderer width.
func (m *AssistantMessage) SetWidth(width int) {
	m.renderer.SetWidth(width)
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

// Interrupt marks a streaming message as interrupted, finalizing it with
// the accumulated content plus an [interrupted] tag.
func (m *AssistantMessage) Interrupt() {
	if !m.streaming {
		return
	}

	m.final = m.content.String() + "\n[interrupted]"
	m.streaming = false
	m.interrupted = true
}

// Interrupted returns whether the message was interrupted.
func (m *AssistantMessage) Interrupted() bool {
	return m.interrupted
}

// View renders the assistant message. Finalized messages use markdown rendering;
// streaming messages render as plain text for performance.
func (m *AssistantMessage) View(width int) string {
	m.renderer.SetWidth(width)

	if m.streaming {
		return m.Content()
	}

	return m.renderer.Render(m.Content())
}

// Draw renders the assistant message into a screen buffer region.
func (m *AssistantMessage) Draw(scr uv.Screen, area uv.Rectangle) {
	if area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	text := m.View(area.Dx())

	for i, line := range strings.Split(text, "\n") {
		if i >= area.Dy() {
			break
		}

		lineRect := uv.Rect(area.Min.X, area.Min.Y+i, area.Dx(), 1)
		uv.NewStyledString(line).Draw(scr, lineRect)
	}
}
