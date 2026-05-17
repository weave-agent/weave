package components

import (
	"strings"
	"unicode/utf8"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// CompletionKind indicates the type of completion being shown.
type CompletionKind int

const (
	CompletionNone CompletionKind = iota
	CompletionSlash
	CompletionFile
	CompletionCustom
)

// CompletionItem is a single item in the completion popup.
type CompletionItem struct {
	Label       string
	Description string
	Value       string
}

// CompletionModel is a popup that shows a filtered list of completion items.
type CompletionModel struct {
	visible      bool
	items        []CompletionItem
	filtered     []CompletionItem
	cursor       int
	scrollOffset int
	filter       string
	width        int
	maxVisible   int
	kind         CompletionKind
}

// NewCompletionModel creates a new completion model with sensible defaults.
func NewCompletionModel() CompletionModel {
	return CompletionModel{
		width:      50,
		maxVisible: 8,
	}
}

// Show makes the completion visible with the given items and kind.
func (m CompletionModel) Show(kind CompletionKind, items []CompletionItem) CompletionModel {
	m.visible = true
	m.kind = kind

	m.items = append([]CompletionItem(nil), items...)
	m.filtered = append([]CompletionItem(nil), items...)
	m.cursor = 0
	m.scrollOffset = 0
	m.filter = ""

	return m
}

// Hide hides the completion popup.
func (m CompletionModel) Hide() CompletionModel {
	m.visible = false
	return m
}

// Visible returns whether the completion popup is currently shown.
func (m CompletionModel) Visible() bool {
	return m.visible
}

// Kind returns the current completion kind.
func (m CompletionModel) Kind() CompletionKind {
	return m.kind
}

// SetFilter filters items by case-insensitive prefix match on Label.
func (m CompletionModel) SetFilter(filter string) CompletionModel {
	m.filter = filter
	filterLower := strings.ToLower(filter)

	m.filtered = m.filtered[:0]
	for _, item := range m.items {
		labelLower := strings.ToLower(item.Label)
		if strings.HasPrefix(labelLower, filterLower) {
			m.filtered = append(m.filtered, item)
		}
	}

	m.cursor = 0

	return m
}

// CursorUp moves the selection up, wrapping to the end.
func (m CompletionModel) CursorUp() CompletionModel {
	if len(m.filtered) == 0 {
		return m
	}

	m.cursor--
	if m.cursor < 0 {
		m.cursor = len(m.filtered) - 1
	}

	m.adjustScroll()

	return m
}

// CursorDown moves the selection down, wrapping to the start.
func (m CompletionModel) CursorDown() CompletionModel {
	if len(m.filtered) == 0 {
		return m
	}

	m.cursor++
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}

	m.adjustScroll()

	return m
}

// adjustScroll ensures the cursor is within the visible window.
func (m *CompletionModel) adjustScroll() {
	if m.cursor >= m.scrollOffset+m.maxVisible {
		m.scrollOffset = m.cursor - m.maxVisible + 1
	} else if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
}

// SelectedItem returns the currently selected item, if any.
func (m CompletionModel) SelectedItem() (CompletionItem, bool) {
	if !m.visible || len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return CompletionItem{}, false
	}

	return m.filtered[m.cursor], true
}

// FilteredCount returns the number of visible (filtered) items.
func (m CompletionModel) FilteredCount() int {
	return len(m.filtered)
}

// Cursor returns the current cursor index.
func (m CompletionModel) Cursor() int {
	return m.cursor
}

// SetWidth sets the popup width for rendering.
func (m CompletionModel) SetWidth(w int) CompletionModel {
	m.width = w
	return m
}

// View renders the completion popup as a string.
func (m CompletionModel) View() string {
	if !m.visible || len(m.filtered) == 0 {
		return ""
	}

	theme := palette.DefaultTheme()

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.Border))

	innerWidth := m.width - 2 // account for left/right border
	visibleCount := min(len(m.filtered)-m.scrollOffset, m.maxVisible)

	var lines []string

	for i := range visibleCount {
		item := m.filtered[m.scrollOffset+i]
		line := m.renderLine(item, m.scrollOffset+i == m.cursor, innerWidth, theme)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")

	return borderStyle.Width(m.width).Render(content)
}

// Draw renders the completion popup into a screen buffer region.
func (m CompletionModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if !m.visible || len(m.filtered) == 0 || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
}

func (m CompletionModel) renderLine(item CompletionItem, selected bool, width int, theme *palette.Theme) string {
	if selected {
		text := item.Label
		if item.Description != "" {
			text += "  " + item.Description
		}

		runes := []rune(text)
		if len(runes) > width {
			text = string(runes[:width-1]) + "…"
		}

		style := lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color(theme.Accent)).
			Foreground(lipgloss.Color(theme.Foreground)).
			Width(width)

		return style.Render(text)
	}

	label := item.Label
	desc := item.Description

	// Truncate description if needed
	if desc != "" {
		maxDesc := max(0, width-utf8.RuneCountInString(label)-2)
		if maxDesc <= 0 {
			desc = ""
		} else {
			descRunes := []rune(desc)
			if len(descRunes) > maxDesc {
				desc = string(descRunes[:maxDesc-1]) + "…"
			}
		}
	}

	// Truncate label if it alone exceeds width
	if utf8.RuneCountInString(label) > width {
		label = string([]rune(label)[:width-1]) + "…"
		desc = ""
	}

	// Build styled segments: label (normal) + desc (dimmed) + padding
	labelStyle := lipgloss.NewStyle().Width(utf8.RuneCountInString(label))
	result := labelStyle.Render(label)

	descWidth := 0
	if desc != "" {
		descWidth = 2 + utf8.RuneCountInString(desc)
		descStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Width(descWidth)
		result += descStyle.Render("  " + desc)
	}

	pad := width - utf8.RuneCountInString(label) - descWidth
	if pad > 0 {
		padStyle := lipgloss.NewStyle().Width(pad)
		result += padStyle.Render("")
	}

	return result
}
