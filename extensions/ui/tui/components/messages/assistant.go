package messages

import (
	"strings"
	"time"

	"github.com/weave-agent/weave-tui/palette"
	"github.com/weave-agent/weave/sdk"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// renderDebounce is the minimum interval between Glamour re-renders during streaming.
const renderDebounce = 100 * time.Millisecond

// AssistantMessage accumulates streaming text deltas into a single message.
type AssistantMessage struct {
	content       strings.Builder
	final         string
	streaming     bool
	interrupted   bool
	renderer      *MarkdownRenderer
	dirty         bool
	lastRender    time.Time
	cachedRender  string
	renderStarted bool
	createdAt     time.Time
	msgType       string
}

// NewAssistantMessage creates a new assistant message in streaming mode.
func NewAssistantMessage() *AssistantMessage {
	return &AssistantMessage{
		streaming: true,
		renderer:  NewMarkdownRenderer(80),
		createdAt: time.Now(),
	}
}

// SetWidth updates the markdown renderer width.
func (m *AssistantMessage) SetWidth(width int) {
	m.renderer.SetWidth(width)
}

// Append adds a content delta to the streaming message.
func (m *AssistantMessage) Append(delta string) {
	m.content.WriteString(delta)
	m.dirty = true
}

// Finalize marks the message as complete with the final content.
func (m *AssistantMessage) Finalize(content string) {
	m.final = content
	m.streaming = false
	m.dirty = false
	m.cachedRender = ""
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
	m.dirty = false
	m.cachedRender = ""
}

// Interrupted returns whether the message was interrupted.
func (m *AssistantMessage) Interrupted() bool {
	return m.interrupted
}

// fadeColor returns a progressively brighter foreground color based on elapsed
// time since creation. Uses the grayscale palette: ForegroundDim -> ForegroundBright.
func (m *AssistantMessage) fadeColor() string {
	theme := palette.DefaultTheme()

	elapsed := time.Since(m.createdAt)
	if elapsed >= 150*time.Millisecond {
		return theme.Foreground
	}

	if elapsed < 50*time.Millisecond {
		return theme.ForegroundDim
	}

	return theme.MutedBright
}

// SetMessageType sets the message type for custom renderer lookup.
func (m *AssistantMessage) SetMessageType(msgType string) {
	m.msgType = msgType
}

// renderCustom attempts to render using a registered custom renderer.
// Returns the rendered string and true if a renderer was found.
func (m *AssistantMessage) renderCustom(width int) (string, bool) {
	if m.msgType == "" {
		return "", false
	}

	renderer, ok := GetMessageRenderer(m.msgType)
	if !ok {
		return "", false
	}

	theme := sdk.ThemeInfo{}
	if GetThemeInfo != nil {
		theme = GetThemeInfo()
	}

	content := renderer.Render(m.Content(), theme, width)

	// Apply fade-in effect during first 150ms while streaming
	if m.streaming {
		fadeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.fadeColor()))
		content = fadeStyle.Render(content)
	}

	// Prepend subtle role indicator
	roleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.DefaultTheme().Muted))
	roleIndicator := roleStyle.Render("◆")

	return roleIndicator + content, true
}

// View renders the assistant message. Finalized messages use markdown rendering.
// Streaming messages progressively render through Glamour with ~100ms debounce
// to avoid re-rendering on every tiny delta while still providing formatted output.
func (m *AssistantMessage) View(width int) string {
	m.renderer.SetWidth(width)

	if rendered, ok := m.renderCustom(width); ok {
		return rendered
	}

	var content string

	if m.streaming {
		content = m.progressiveRender()
	} else {
		content = m.renderer.Render(m.Content())
	}

	// Apply fade-in effect during first 150ms while streaming
	if m.streaming {
		fadeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.fadeColor()))
		content = fadeStyle.Render(content)
	}

	// Prepend subtle role indicator
	roleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.DefaultTheme().Muted))
	roleIndicator := roleStyle.Render("◆")

	return roleIndicator + content
}

// progressiveRender returns the cached rendered output, re-rendering through
// Glamour only when the content is dirty and the debounce interval has elapsed.
// On the first Append after creation or finalization, it renders immediately
// so the user sees formatted content as soon as it arrives.
func (m *AssistantMessage) progressiveRender() string {
	if m.dirty {
		elapsed := time.Since(m.lastRender)
		if !m.renderStarted || elapsed >= renderDebounce {
			m.cachedRender = m.renderer.Render(m.Content())
			m.lastRender = time.Now()
			m.dirty = false
			m.renderStarted = true
		}
	}

	if m.cachedRender != "" {
		return m.cachedRender
	}

	return m.Content()
}

// NeedsRender returns true when the message is streaming with pending content
// that hasn't been re-rendered through Glamour yet. This signals to the chat
// cache that the item should be re-rendered even if width hasn't changed,
// allowing spinner ticks to trigger debounce-based re-renders.
func (m *AssistantMessage) NeedsRender() bool {
	return m.streaming && m.dirty
}

// Draw renders the assistant message into a screen buffer region.
func (m *AssistantMessage) Draw(scr uv.Screen, area uv.Rectangle) {
	drawView(scr, area, m.View(area.Dx()))
}
