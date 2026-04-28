package attachments

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	pasteNewlineThreshold = 10
	pasteCharThreshold    = 1000
)

// Attachment holds a file attached to the current prompt.
type Attachment struct {
	Path     string
	Content  string
	Lines    int
	IsPasted bool
}

// Model tracks attachments above the editor and handles paste detection.
type Model struct {
	items       []Attachment
	deleteIdx   int
	deleteMode  bool
}

// New creates an empty attachments model.
func New() Model {
	return Model{}
}

// Add appends an attachment.
func (m Model) Add(a Attachment) Model {
	m.items = append(m.items, a)
	return m
}

// AddFile reads a file and adds it as an attachment.
func (m Model) AddFile(path string) (Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("read %s: %w", path, err)
	}

	content := string(data)
	lines := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") && content != "" {
		lines++
	}

	return m.Add(Attachment{
		Path:    path,
		Content: content,
		Lines:   lines,
	}), nil
}

// AddPaste creates an attachment from pasted content.
func (m Model) AddPaste(content string) Model {
	lines := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") && content != "" {
		lines++
	}

	return m.Add(Attachment{
		Path:     "paste.txt",
		Content:  content,
		Lines:    lines,
		IsPasted: true,
	})
}

// Remove deletes the attachment at index.
func (m Model) Remove(idx int) Model {
	if idx < 0 || idx >= len(m.items) {
		return m
	}
	m.items = append(m.items[:idx], m.items[idx+1:]...)
	if m.deleteIdx >= len(m.items) {
		m.deleteIdx = max(0, len(m.items)-1)
	}
	if len(m.items) == 0 {
		m.deleteMode = false
		m.deleteIdx = 0
	}
	return m
}

// Items returns the current attachments.
func (m Model) Items() []Attachment {
	return m.items
}

// IsPastedContent returns true if the text should be auto-converted to an attachment.
func IsPastedContent(text string) bool {
	newlines := strings.Count(text, "\n")
	return newlines >= pasteNewlineThreshold || len(text) >= pasteCharThreshold
}

// ToggleDeleteMode toggles attachment delete mode.
func (m Model) ToggleDeleteMode() Model {
	if len(m.items) == 0 {
		m.deleteMode = false
		return m
	}
	m.deleteMode = !m.deleteMode
	m.deleteIdx = 0
	return m
}

// DeleteModeNext moves to the next attachment in delete mode.
func (m Model) DeleteModeNext() Model {
	if len(m.items) == 0 {
		return m
	}
	m.deleteIdx = (m.deleteIdx + 1) % len(m.items)
	return m
}

// DeleteModePrev moves to the previous attachment in delete mode.
func (m Model) DeleteModePrev() Model {
	if len(m.items) == 0 {
		return m
	}
	m.deleteIdx = (m.deleteIdx - 1 + len(m.items)) % len(m.items)
	return m
}

// InDeleteMode returns whether delete mode is active.
func (m Model) InDeleteMode() bool {
	return m.deleteMode
}

// DeleteIdx returns the currently highlighted attachment in delete mode.
func (m Model) DeleteIdx() int {
	return m.deleteIdx
}

// Clear removes all attachments.
func (m Model) Clear() Model {
	m.items = nil
	m.deleteMode = false
	m.deleteIdx = 0
	return m
}

// RenderPrompt returns the combined text including attachment contents.
func (m Model) RenderPrompt(editorText string) string {
	if len(m.items) == 0 {
		return editorText
	}

	var sb strings.Builder
	if editorText != "" {
		sb.WriteString(editorText)
		sb.WriteString("\n\n")
	}

	for i, a := range m.items {
		if i > 0 {
			sb.WriteString("\n")
		}
		name := filepath.Base(a.Path)
		fmt.Fprintf(&sb, "<file name=%q>\n%s\n</file>", name, a.Content)
	}

	return sb.String()
}

// Draw renders attachment indicators into the screen buffer.
func (m Model) Draw(scr uv.Screen, area uv.Rectangle) {
	if area.Dx() <= 0 || area.Dy() <= 0 || len(m.items) == 0 {
		return
	}

	y := area.Min.Y
	maxY := area.Min.Y + area.Dy()

	attachStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	bracketStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	for i, a := range m.items {
		if y >= maxY {
			break
		}

		name := filepath.Base(a.Path)
		label := fmt.Sprintf(" %s (%d lines) ", name, a.Lines)

		lineArea := uv.Rect(area.Min.X, y, area.Dx(), 1)

		if m.deleteMode && i == m.deleteIdx {
			text := deleteStyle.Render(label)
			uv.NewStyledString(text).Draw(scr, lineArea)
		} else {
			prefix := bracketStyle.Render("[")
			suffix := bracketStyle.Render("]")
			text := prefix + attachStyle.Render(label) + suffix
			uv.NewStyledString(text).Draw(scr, lineArea)
		}

		y++
	}
}

// Height returns the number of rows needed to display attachments.
func (m Model) Height() int {
	return len(m.items)
}
