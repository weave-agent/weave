package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
}

// NewChatModel creates a new chat model.
func NewChatModel() ChatModel {
	return ChatModel{}
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

// ScrollUp moves the viewport up by n lines.
func (m ChatModel) ScrollUp(n int) ChatModel {
	m.scroll = max(0, m.scroll-n)
	return m
}

// ScrollDown moves the viewport down by n lines.
func (m ChatModel) ScrollDown(n int) ChatModel {
	totalLines := m.totalLines()
	maxScroll := max(0, totalLines-m.height)
	m.scroll = min(maxScroll, m.scroll+n)

	return m
}

// AddItem appends a chat item and auto-scrolls to bottom.
func (m ChatModel) AddItem(item ChatItem) ChatModel {
	m.items = append(m.items, item)

	if m.cache != nil {
		*m.cache = append(*m.cache, cacheEntry{})
	}

	m.scrollToBottom()

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

// Update handles messages for the chat model.
func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	_ = msg

	return m, nil
}

// View renders the visible portion of the chat.
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

// FormatUserMessage creates a formatted string for a user message.
func FormatUserMessage(content string) string {
	return "> " + content
}
