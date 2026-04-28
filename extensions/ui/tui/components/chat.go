package components

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	uv "github.com/charmbracelet/ultraviolet"
)

// ChatItem is an interface for items rendered in the chat view.
type ChatItem interface {
	View(width int) string
}

// ChatItemIdentity is an optional interface for items that have a unique ID.
// Used for in-place updates of non-last items (e.g., tool panels).
type ChatItemIdentity interface {
	ChatItem
	ItemID() string
}

// cacheEntry stores the rendered output for a chat item at a given width.
type cacheEntry struct {
	width int
	lines []string
}

// ChatModel manages a scrollable list of chat items.
type ChatModel struct {
	items  []ChatItem
	width  int
	height int
	scroll int
	cache  *[]cacheEntry // pointer so value copies share state

	// autoScroll follows the stream when the user is near the bottom.
	autoScroll bool
	// newContent is set when content arrives while the user is scrolled up.
	newContent bool
	// turnEndPending is set externally when a turn ends while not at the bottom.
	turnEndPending bool
}

// NewChatModel creates a new chat model.
func NewChatModel() ChatModel {
	return ChatModel{autoScroll: true}
}

// SetSize updates the chat dimensions and invalidates the entire cache.
func (m ChatModel) SetSize(width, height int) ChatModel {
	if m.width != width {
		m.cache = nil
	} else if m.cache == nil {
		// Ensure cache pointer is initialized for value-copy sharing.
		c := make([]cacheEntry, len(m.items))
		m.cache = &c
	}

	m.width = width
	m.height = height

	return m
}

// Width returns the chat width.
func (m ChatModel) Width() int {
	return m.width
}

// Height returns the chat height.
func (m ChatModel) Height() int {
	return m.height
}

// Items returns the current chat items.
func (m ChatModel) Items() []ChatItem {
	return m.items
}

// ScrollOffset returns the current scroll offset.
func (m ChatModel) ScrollOffset() int {
	return m.scroll
}

// NearBottom returns true if the scroll position is within 3 lines of the maximum.
func (m ChatModel) NearBottom() bool {
	totalLines := m.totalLines()
	maxScroll := max(0, totalLines-m.height)

	return m.scroll >= maxScroll-3
}

// AtBottom returns true if scrolled to the very bottom.
func (m ChatModel) AtBottom() bool {
	totalLines := m.totalLines()
	maxScroll := max(0, totalLines-m.height)

	return m.scroll >= maxScroll
}

// NewContent returns whether new content arrived while scrolled up.
func (m ChatModel) NewContent() bool {
	return m.newContent
}

// TurnEndPending returns whether the turn-end scroll indicator is active.
func (m ChatModel) TurnEndPending() bool {
	return m.turnEndPending
}

// SetTurnEndPending sets the turn-end scroll indicator.
func (m ChatModel) SetTurnEndPending(pending bool) ChatModel {
	m.turnEndPending = pending
	return m
}

// AutoScroll returns whether auto-scroll is active.
func (m ChatModel) AutoScroll() bool {
	return m.autoScroll
}

// ScrollUp moves the viewport up by n lines.
func (m ChatModel) ScrollUp(n int) ChatModel {
	maxScroll := max(0, m.totalLines()-m.height)
	if maxScroll > 0 && m.scroll > 0 {
		m.autoScroll = false
	}

	m.scroll = max(0, m.scroll-n)

	return m
}

// ScrollDown moves the viewport down by n lines.
func (m ChatModel) ScrollDown(n int) ChatModel {
	totalLines := m.totalLines()
	maxScroll := max(0, totalLines-m.height)
	newScroll := min(maxScroll, m.scroll+n)
	m.scroll = newScroll

	// Re-enable auto-scroll if user scrolled back to the bottom
	if newScroll >= maxScroll {
		m.autoScroll = true
		m.newContent = false
	}

	return m
}

// JumpToBottom scrolls to the very bottom and clears all indicators.
func (m ChatModel) JumpToBottom() ChatModel {
	m.scrollToBottom()
	m.autoScroll = true
	m.newContent = false
	m.turnEndPending = false

	return m
}

// AddItem appends a chat item and auto-scrolls if near the bottom.
func (m ChatModel) AddItem(item ChatItem) ChatModel {
	nearBottom := m.NearBottom()

	m.items = append(m.items, item)

	if m.cache != nil {
		*m.cache = append(*m.cache, cacheEntry{})
	}

	if m.autoScroll || nearBottom {
		m.scrollToBottom()
		m.autoScroll = true
	} else {
		m.newContent = true
	}

	return m
}

// UpdateItem replaces the last item if it matches the given type, otherwise appends.
// This is used for updating the current assistant message in-place.
func (m ChatModel) UpdateItem(item ChatItem) ChatModel {
	if len(m.items) > 0 {
		m.items[len(m.items)-1] = item
		m.invalidate(len(m.items) - 1)
	} else {
		m.items = append(m.items, item)

		if m.cache != nil {
			*m.cache = append(*m.cache, cacheEntry{})
		}
	}

	// Auto-scroll only if following the stream
	if m.autoScroll {
		m.scrollToBottom()
	}

	return m
}

// UpdateItemByID finds an item by ChatItemIdentity interface and replaces it.
// Falls back to appending if not found.
func (m ChatModel) UpdateItemByID(item ChatItem) ChatModel {
	id, ok := item.(ChatItemIdentity)
	if !ok {
		return m.AddItem(item)
	}

	targetID := id.ItemID()
	for i, existing := range m.items {
		if eid, ok := existing.(ChatItemIdentity); ok && eid.ItemID() == targetID {
			m.items[i] = item
			m.invalidate(i)

			return m
		}
	}

	return m.AddItem(item)
}

// UpdateItemAt replaces the item at the given index.
func (m ChatModel) UpdateItemAt(index int, item ChatItem) ChatModel {
	if index >= 0 && index < len(m.items) {
		m.items[index] = item
		m.invalidate(index)
	}

	return m
}

// invalidate marks a single cache entry as stale.
func (m *ChatModel) invalidate(index int) {
	if m.cache != nil && index >= 0 && index < len(*m.cache) {
		(*m.cache)[index] = cacheEntry{}
	}
}

// scrollToBottom adjusts scroll to show the last line.
func (m *ChatModel) scrollToBottom() {
	totalLines := m.totalLines()
	m.scroll = max(0, totalLines-m.height)
}

// totalLines counts the total rendered lines across all items, using cache where possible.
func (m *ChatModel) totalLines() int {
	m.ensureCache()

	total := 0
	for i := range m.items {
		total += len((*m.cache)[i].lines)
	}

	return total
}

// ensureCache guarantees the cache slice is aligned with items and renders any missing entries.
func (m *ChatModel) ensureCache() {
	if m.cache == nil {
		c := make([]cacheEntry, len(m.items))
		m.cache = &c
	} else if len(*m.cache) != len(m.items) {
		c := make([]cacheEntry, len(m.items))
		m.cache = &c
	}

	for i, item := range m.items {
		if (*m.cache)[i].width != m.width || (*m.cache)[i].lines == nil {
			text := item.View(m.width)
			(*m.cache)[i] = cacheEntry{
				width: m.width,
				lines: strings.Split(text, "\n"),
			}
		}
	}
}

// View renders the visible portion of the chat as a string.
func (m ChatModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	m.ensureCache()

	var allLines []string

	for i := range m.items {
		allLines = append(allLines, (*m.cache)[i].lines...)
	}

	total := len(allLines)
	maxScroll := max(0, total-m.height)
	m.scroll = min(m.scroll, maxScroll)

	end := min(m.scroll+m.height, total)

	visible := allLines[m.scroll:end]

	// Pad to fill height
	for len(visible) < m.height {
		visible = append(visible, "")
	}

	return strings.Join(visible, "\n")
}

// Draw renders the visible portion of the chat into a screen buffer region.
// Uses the width set via SetSize for item rendering and derives the viewport
// height from the area rectangle.
func (m ChatModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if m.width <= 0 || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	m.ensureCache()

	var allLines []string

	for i := range m.items {
		allLines = append(allLines, (*m.cache)[i].lines...)
	}

	viewportHeight := area.Dy()
	total := len(allLines)
	maxScroll := max(0, total-viewportHeight)
	m.scroll = min(m.scroll, maxScroll)

	end := min(m.scroll+viewportHeight, total)
	visible := allLines[m.scroll:end]

	for i, line := range visible {
		lineRect := uv.Rect(area.Min.X, area.Min.Y+i, area.Dx(), 1)
		uv.NewStyledString(line).Draw(scr, lineRect)
	}

	// Render scroll indicators on the last visible line
	if m.newContent || m.turnEndPending {
		indicator := "↓ new content"
		if m.turnEndPending && !m.newContent {
			indicator = "↓ scroll to bottom"
		}

		indStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
		lastRow := area.Min.Y + viewportHeight - 1
		indRect := uv.Rect(area.Min.X, lastRow, area.Dx(), 1)
		uv.NewStyledString(fmt.Sprintf("%s%s", strings.Repeat(" ", max(0, area.Dx()-utf8.RuneCountInString(indicator)-2)), indStyle.Render(indicator))).Draw(scr, indRect)
	}
}

// FormatUserMessage creates a formatted string for a user message.
func FormatUserMessage(content string) string {
	return "> " + content
}
