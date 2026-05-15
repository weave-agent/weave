package components

import (
	"strings"

	"weave/ext/ui/tui/palette"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// ChatItem is an interface for items rendered in the chat view.
type ChatItem interface {
	View(width int) string
}

// NeedsRenderer is an optional interface for items that may need re-rendering
// even when width hasn't changed (e.g., streaming messages with debounce).
type NeedsRenderer interface {
	NeedsRender() bool
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

	// text selection state (mouse click-and-drag)
	selActive    bool
	selStartLine int
	selStartCol  int
	selEndLine   int
	selEndCol    int
	mouseDown    bool
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

	if m.autoScroll {
		m.scrollToBottom()
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
// Includes blank separator lines between items.
func (m *ChatModel) totalLines() int {
	m.ensureCache()

	total := 0
	for i := range m.items {
		total += len((*m.cache)[i].lines)
		// Blank separator line between items (not after the last one)
		if i < len(m.items)-1 {
			total++
		}
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
		stale := (*m.cache)[i].width != m.width || (*m.cache)[i].lines == nil

		if !stale {
			if nr, ok := item.(NeedsRenderer); ok && nr.NeedsRender() {
				stale = true
			}
		}

		if stale {
			text := item.View(m.width)
			(*m.cache)[i] = cacheEntry{
				width: m.width,
				lines: strings.Split(text, "\n"),
			}
		}
	}
}

// --- Selection methods ---

// StartSelection begins a new text selection at the given global line and column.
func (m ChatModel) StartSelection(line, col int) ChatModel {
	m.selActive = true
	m.selStartLine = line
	m.selStartCol = col
	m.selEndLine = line
	m.selEndCol = col
	m.mouseDown = true
	return m
}

// ExtendSelection updates the end point of the current selection.
func (m ChatModel) ExtendSelection(line, col int) ChatModel {
	if !m.mouseDown {
		return m
	}
	m.selEndLine = line
	m.selEndCol = col
	return m
}

// EndSelection finalizes the current selection and clears mouseDown.
func (m ChatModel) EndSelection() ChatModel {
	m.mouseDown = false
	// Normalize: ensure start <= end for line and column
	if m.selStartLine > m.selEndLine {
		m.selStartLine, m.selEndLine = m.selEndLine, m.selStartLine
		m.selStartCol, m.selEndCol = m.selEndCol, m.selStartCol
	} else if m.selStartLine == m.selEndLine && m.selStartCol > m.selEndCol {
		m.selStartCol, m.selEndCol = m.selEndCol, m.selStartCol
	}
	return m
}

// ClearSelection removes the active selection.
func (m ChatModel) ClearSelection() ChatModel {
	m.selActive = false
	m.selStartLine = 0
	m.selStartCol = 0
	m.selEndLine = 0
	m.selEndCol = 0
	m.mouseDown = false
	return m
}

// HasSelection returns true if there is an active non-empty selection.
func (m ChatModel) HasSelection() bool {
	if !m.selActive {
		return false
	}
	if m.selStartLine == m.selEndLine {
		return m.selStartCol != m.selEndCol
	}
	return true
}

// MouseDown returns true if the mouse button is currently held down.
func (m ChatModel) MouseDown() bool {
	return m.mouseDown
}

// SelectionBounds returns the normalized selection bounds as (startLine, startCol, endLine, endCol).
// Start is always <= end (by line, then by column).
func (m ChatModel) SelectionBounds() (int, int, int, int) {
	if !m.selActive {
		return 0, 0, 0, 0
	}
	sl, sc, el, ec := m.selStartLine, m.selStartCol, m.selEndLine, m.selEndCol
	if sl > el {
		sl, el = el, sl
		sc, ec = ec, sc
	} else if sl == el && sc > ec {
		sc, ec = ec, sc
	}
	return sl, sc, el, ec
}

// selectionSpan represents the start and end columns for a selected line.
type selectionSpan struct {
	startCol int
	endCol   int
}

// selectionForLine returns the column span for the given global content line
// if it falls within the current selection. Returns nil if the line is not selected.
func (m ChatModel) selectionForLine(globalLine int) *selectionSpan {
	if !m.selActive {
		return nil
	}
	sl, sc, el, ec := m.SelectionBounds()
	if globalLine < sl || globalLine > el {
		return nil
	}
	span := &selectionSpan{}
	if globalLine == sl {
		span.startCol = sc
	} else {
		span.startCol = 0
	}
	if globalLine == el {
		span.endCol = ec
	} else {
		// For intermediate lines, select to end of line (use a large value)
		span.endCol = m.width
	}
	// Empty span on a single line
	if globalLine == sl && globalLine == el && span.startCol == span.endCol {
		return nil
	}
	return span
}

// lineToItem maps a global content line to the item index and the line index within that item.
// Returns (-1, -1) if the line is out of bounds.
func (m ChatModel) lineToItem(contentLine int) (itemIdx, lineInItem int) {
	m.ensureCache()
	currentLine := 0
	for i := range m.items {
		itemLines := len((*m.cache)[i].lines)
		if contentLine >= currentLine && contentLine < currentLine+itemLines {
			return i, contentLine - currentLine
		}
		currentLine += itemLines
		// Blank separator line between items
		if i < len(m.items)-1 {
			if contentLine == currentLine {
				// This is a separator line, not part of any item
				return -1, -1
			}
			currentLine++
		}
	}
	return -1, -1
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
		// Add blank line between items (not after the last one)
		if i < len(m.items)-1 {
			allLines = append(allLines, "")
		}
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
		// Add blank line between items (not after the last one)
		if i < len(m.items)-1 {
			allLines = append(allLines, "")
		}
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

	// Render scroll indicators on the last visible line as a styled pill
	if m.newContent || m.turnEndPending {
		indicator := "↓ new content"
		if m.turnEndPending && !m.newContent {
			indicator = "↓ scroll to bottom"
		}

		indStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(palette.DefaultTheme().Warning)).
			Background(lipgloss.Color(palette.DefaultTheme().BackgroundTint)).
			Padding(0, 1)
		lastRow := area.Min.Y + viewportHeight - 1
		indRect := uv.Rect(area.Min.X, lastRow, area.Dx(), 1)
		paddedIndicator := " " + indStyle.Render(indicator) + " "
		spaces := max(0, area.Dx()-lipgloss.Width(paddedIndicator))
		uv.NewStyledString(strings.Repeat(" ", spaces)+paddedIndicator).Draw(scr, indRect)
	}
}

// FormatUserMessage creates a formatted string for a user message.
func FormatUserMessage(content string) string {
	return "> " + content
}
