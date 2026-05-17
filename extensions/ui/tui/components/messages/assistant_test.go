package messages

import (
	"strings"
	"testing"
	"time"

	"github.com/weave-agent/weave-tui/palette"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

// stripANSI removes ANSI escape sequences from a string for test assertions.
func stripANSI(s string) string {
	return ansi.Strip(s)
}

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

func TestAssistantMessage_AppendSetsDirty(t *testing.T) {
	m := NewAssistantMessage()
	assert.False(t, m.dirty, "new message should not be dirty")

	m.Append("text")
	assert.True(t, m.dirty, "Append should set dirty flag")
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

func TestAssistantMessage_FinalizeClearsRenderState(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("# Hello")
	_ = m.View(80) // trigger a render
	m.Finalize("final")

	assert.False(t, m.dirty)
	assert.Empty(t, m.cachedRender)
}

func TestAssistantMessage_View_Streaming_FirstRenderIsMarkdown(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("# Hello World\n\nSome **bold** text.")

	// First View() call should immediately render through Glamour
	view := stripANSI(m.View(80))
	assert.Contains(t, view, "Hello World")
	assert.Contains(t, view, "bold")
	// Markdown output should be longer than plain text due to ANSI codes
	assert.Greater(t, len(view), len("# Hello World\n\nSome **bold** text."))
}

func TestAssistantMessage_View_Streaming_DebouncedWithinInterval(t *testing.T) {
	m := NewAssistantMessage()

	// First render
	m.Append("line one")
	view1 := m.View(80)
	assert.Contains(t, stripANSI(view1), "line one")

	// Rapid Append within debounce interval
	m.Append(" line two")
	// Without sleeping, the debounce should prevent re-render
	// but return the cached render from the first call
	view2 := m.View(80)
	assert.Equal(t, view1, view2, "should return cached render within debounce interval")
}

func TestAssistantMessage_View_Streaming_DebounceExpired(t *testing.T) {
	m := NewAssistantMessage()

	m.Append("first")
	view1 := m.View(80)
	assert.Contains(t, stripANSI(view1), "first")

	// Wait for debounce to expire
	time.Sleep(renderDebounce + 10*time.Millisecond)

	m.Append(" second")
	view2 := m.View(80)
	assert.Contains(t, stripANSI(view2), "second", "after debounce expires, new content should be rendered")
	assert.NotEqual(t, view1, view2, "rendered output should change after debounce")
}

func TestAssistantMessage_View_Streaming_MultipleAppendsWithinDebounce(t *testing.T) {
	m := NewAssistantMessage()

	m.Append("a")
	_ = m.View(80)

	// Multiple appends within debounce window
	m.Append(" b")
	m.Append(" c")
	m.Append(" d")

	view := m.View(80)
	// Should still be the cached render, not including b, c, d
	assert.Contains(t, stripANSI(view), "a")
	// The cached render doesn't include the later appends
	assert.NotContains(t, stripANSI(view), "b c d")

	// After debounce expires, all content should appear
	time.Sleep(renderDebounce + 10*time.Millisecond)

	view = m.View(80)
	assert.Contains(t, stripANSI(view), "a b c d")
}

func TestAssistantMessage_View_Finalized_Markdown(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("# Hello World\n\nSome **bold** text.")
	view := stripANSI(m.View(80))
	assert.Contains(t, view, "Hello World")
	assert.Contains(t, view, "bold")
	assert.Greater(t, len(view), len("Hello World"))
}

func TestAssistantMessage_View_Finalized_CodeBlock(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("```go\nfmt.Println(\"hi\")\n```")
	view := stripANSI(m.View(80))
	assert.Contains(t, view, "fmt.Println")
}

func TestAssistantMessage_SetWidth(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("# Title")
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

func TestAssistantMessage_Interrupt_ClearsRenderState(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("partial")
	_ = m.View(80) // trigger render
	m.Interrupt()

	assert.False(t, m.dirty)
	assert.Empty(t, m.cachedRender)
}

func TestAssistantMessage_Interrupt_Idempotent(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("partial")
	m.Interrupt()
	content1 := m.Content()
	m.Interrupt()
	assert.Equal(t, content1, m.Content())
}

func TestAssistantMessage_Interrupt_NotStreaming(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("done")
	m.Interrupt()
	assert.False(t, m.Interrupted())
	assert.Equal(t, "done", m.Content())
}

func TestAssistantMessage_ProgressiveRender_PartialMarkdown(t *testing.T) {
	m := NewAssistantMessage()

	// Simulate streaming partial markdown (unclosed code fence)
	m.Append("```go\nfmt.Println(")
	view := stripANSI(m.View(80))
	// Glamour handles unclosed fences gracefully — should still render visible text
	assert.Contains(t, view, "fmt.Println")
}

func TestAssistantMessage_ProgressiveRender_UnclosedBold(t *testing.T) {
	m := NewAssistantMessage()

	// Unclosed bold markup
	m.Append("This is **bold")
	view := stripANSI(m.View(80))
	assert.Contains(t, view, "bold")
}

func TestAssistantMessage_ProgressiveRender_FencedCodeComplete(t *testing.T) {
	m := NewAssistantMessage()

	// Complete code fence
	m.Append("```go\nfmt.Println(\"hello\")\n```")
	view := stripANSI(m.View(80))
	assert.Contains(t, view, "fmt.Println")
}

func TestAssistantMessage_Draw_Streaming(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("hello world")

	canvas := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas, canvas.Bounds())
	output := stripANSI(uv.TrimSpace(canvas.Render()))
	assert.Contains(t, output, "hello world")
}

func TestAssistantMessage_Draw_Finalized(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("# Hello\n\nSome **bold** text.")

	canvas := uv.NewScreenBuffer(80, 10)
	m.Draw(canvas, canvas.Bounds())
	output := stripANSI(uv.TrimSpace(canvas.Render()))
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

func TestAssistantMessage_Draw_StreamingProgressive(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("# Hello")

	canvas1 := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas1, canvas1.Bounds())
	out1 := uv.TrimSpace(canvas1.Render())
	assert.Contains(t, stripANSI(out1), "Hello")

	// Append more within debounce — same cached render
	m.Append(" World")

	canvas2 := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas2, canvas2.Bounds())
	out2 := uv.TrimSpace(canvas2.Render())
	assert.Equal(t, out1, out2, "within debounce, Draw should use cached render")

	// Wait for debounce to expire
	time.Sleep(renderDebounce + 10*time.Millisecond)

	canvas3 := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas3, canvas3.Bounds())
	out3 := uv.TrimSpace(canvas3.Render())
	assert.Contains(t, stripANSI(out3), "World", "after debounce, Draw should include new content")
}

// --- Task 2: Assistant message role indicator tests ---

func TestAssistantMessage_RoleIndicator_PresentInView(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("Hello world")

	view := m.View(80)
	assert.Contains(t, view, "◆")
}

func TestAssistantMessage_RoleIndicator_PresentInStreamingView(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("streaming content")

	view := m.View(80)
	assert.Contains(t, view, "◆")
	assert.Contains(t, stripANSI(view), "streaming content")
}

func TestAssistantMessage_RoleIndicator_PresentInDraw(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("Test content")

	canvas := uv.NewScreenBuffer(80, 5)
	m.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())

	assert.Contains(t, output, "◆")
}

func TestAssistantMessage_RoleIndicator_MutedColor(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("content")

	view := m.View(80)
	// The role indicator uses muted color (240)
	assert.Contains(t, view, palette.DefaultTheme().Muted)
}

func TestAssistantMessage_RoleIndicator_NotInContent(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("just content")

	// Content should not include the role indicator
	assert.Equal(t, "just content", m.Content())
	assert.NotContains(t, m.Content(), "assistant")
}

// --- Task 6: Fade-in animation tests ---

func TestAssistantMessage_CreatedAt_IsSet(t *testing.T) {
	m := NewAssistantMessage()
	assert.False(t, m.createdAt.IsZero(), "createdAt should be set on creation")
}

func TestAssistantMessage_FadeColor_NewMessage_IsDim(t *testing.T) {
	m := NewAssistantMessage()
	// A brand-new message should start with ForegroundDim (245)
	assert.Equal(t, palette.DefaultTheme().ForegroundDim, m.fadeColor())
}

func TestAssistantMessage_FadeColor_Progresses(t *testing.T) {
	m := NewAssistantMessage()
	m.createdAt = time.Now().Add(-60 * time.Millisecond)
	// After 60ms, should be at MutedBright (248)
	assert.Equal(t, palette.DefaultTheme().MutedBright, m.fadeColor())
}

func TestAssistantMessage_FadeColor_After150ms_IsFull(t *testing.T) {
	m := NewAssistantMessage()
	m.createdAt = time.Now().Add(-200 * time.Millisecond)
	// After 150ms, should be full brightness (theme foreground)
	assert.Equal(t, palette.DefaultTheme().Foreground, m.fadeColor())
}

func TestAssistantMessage_FadeColor_FinalizedMessage(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("final content")
	// Even for finalized messages, fade color should be full brightness after delay
	m.createdAt = time.Now().Add(-200 * time.Millisecond)
	assert.Equal(t, palette.DefaultTheme().Foreground, m.fadeColor())
}

func TestAssistantMessage_View_HasFadeColor(t *testing.T) {
	m := NewAssistantMessage()
	m.Append("hello")
	// Force createdAt to the future so fade color is always ForegroundDim
	m.createdAt = time.Now().Add(time.Hour)
	view := m.View(80)
	// The fade style should wrap the content with ForegroundDim color
	assert.Contains(t, view, palette.DefaultTheme().ForegroundDim)
}

func TestAssistantMessage_View_Finalized_NoFadeColor(t *testing.T) {
	m := NewAssistantMessage()
	m.Finalize("final content")
	// Force createdAt to the future so fade color would be ForegroundDim if applied
	m.createdAt = time.Now().Add(time.Hour)
	view := m.View(80)
	// Finalized messages should NOT have fade styling — they render at full brightness
	assert.NotContains(t, view, palette.DefaultTheme().ForegroundDim)
}
