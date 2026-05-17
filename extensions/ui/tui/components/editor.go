package components

import (
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/rivo/uniseg"
)

// SubmitMsg is emitted when the user submits the editor content.
type SubmitMsg struct {
	Text string
}

// EditorModel wraps a bubbles/v2 textarea with history and custom styling.
type EditorModel struct {
	ta      textarea.Model
	focused bool

	// BorderColor is the current border color (ANSI color code or name).
	BorderColor string

	// Pulse animation state
	PulsePos          int    // 0-7 position for pulse animation (0 = inactive)
	PulseActive       bool   // true when agent is actively working
	pulseAccent       string // accent color for pulse animation
	pulseAccentBright string // bright accent color for pulse animation

	// history
	history    []string
	histIdx    int
	savedLine  string
	navigating bool

	// completion
	completion    CompletionModel
	triggerOffset int

	// text selection state (mouse click-and-drag)
	selActive    bool
	selStartLine int
	selStartCol  int
	selEndLine   int
	selEndCol    int
	mouseDown    bool
}

const minEditorWidth = 20

func isEnterKey(code rune) bool {
	return code == tea.KeyEnter || code == tea.KeyKpEnter
}

// borderStyle creates a border style with the given foreground color.
func borderStyle(fg string) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(fg)).
		PaddingLeft(1)
}

// NewEditorModel creates a new editor model backed by bubbles/v2 textarea.
func NewEditorModel() EditorModel {
	ta := textarea.New()
	ta.DynamicHeight = true
	ta.MinHeight = 3
	ta.MaxHeight = 15
	ta.CharLimit = -1
	ta.ShowLineNumbers = false
	ta.SetVirtualCursor(true)
	ta.Prompt = ""
	ta.Placeholder = ""
	ta.SetHeight(3)
	ta.Focus()

	styles := textarea.DefaultStyles(false)
	styles.Focused.Base = borderStyle(palette.DefaultTheme().Accent)
	styles.Blurred.Base = borderStyle(palette.DefaultTheme().Border)
	styles.Focused.Text = lipgloss.NewStyle()
	styles.Blurred.Text = lipgloss.NewStyle()
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(palette.DefaultTheme().Muted))

	// Override light-mode defaults that cause white background on cursor line
	// and visible end-of-buffer characters.
	base := lipgloss.NewStyle()
	styles.Focused.CursorLine = base
	styles.Focused.CursorLineNumber = base
	styles.Focused.EndOfBuffer = base
	styles.Focused.LineNumber = base
	styles.Blurred.CursorLine = base
	styles.Blurred.CursorLineNumber = base
	styles.Blurred.EndOfBuffer = base
	styles.Blurred.LineNumber = base

	ta.SetStyles(styles)

	return EditorModel{
		ta:          ta,
		focused:     true,
		BorderColor: palette.DefaultTheme().Accent,
		completion:  NewCompletionModel(),
	}
}

// SetValue replaces the editor content.
func (m EditorModel) SetValue(s string) EditorModel {
	m.ta.SetValue(s)
	return m
}

// Value returns the current editor content.
func (m EditorModel) Value() string {
	return m.ta.Value()
}

// InsertNewline inserts a newline at the current cursor position without
// treating Enter as submit.
func (m EditorModel) InsertNewline() (EditorModel, tea.Cmd) {
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	return m, cmd
}

// SetSize updates the editor dimensions.
func (m EditorModel) SetSize(width, height int) EditorModel {
	m.ta.SetWidth(max(minEditorWidth, width))
	m.ta.SetHeight(max(1, height))

	return m
}

// Width returns the editor width.
func (m EditorModel) Width() int { return m.ta.Width() }

// Height returns the editor height (content lines, not including border).
func (m EditorModel) Height() int { return m.ta.Height() }

// MaxHeight returns the maximum height the editor can grow to.
func (m EditorModel) MaxHeight() int { return m.ta.MaxHeight }

// Focused returns whether the editor has focus.
func (m EditorModel) Focused() bool { return m.focused }

// Focus gives the editor focus.
func (m EditorModel) Focus() EditorModel {
	m.focused = true
	m.ta.Focus()

	return m
}

// Blur removes focus from the editor.
func (m EditorModel) Blur() EditorModel {
	m.focused = false
	m.ta.Blur()

	return m
}

// SetMaxHeight sets the maximum height for the dynamic textarea. Values <= 0 are ignored.
func (m EditorModel) SetMaxHeight(n int) EditorModel {
	if n > 0 {
		m.ta.MaxHeight = n
	}

	return m
}

// SetBorderColor updates the editor focused border color.
// The blurred border always uses the theme's Border color for distinction.
func (m EditorModel) SetBorderColor(color string) EditorModel {
	m.BorderColor = color

	styles := m.ta.Styles()
	styles.Focused.Base = borderStyle(color)
	styles.Blurred.Base = borderStyle(palette.DefaultTheme().Border)
	m.ta.SetStyles(styles)

	return m
}

// SetPulseActive enables or disables the pulse animation on the editor border.
func (m EditorModel) SetPulseActive(active bool) EditorModel {
	m.PulseActive = active
	if !active {
		m.PulsePos = 0
	}

	return m
}

// SetPulsePos updates the pulse animation position (0-7).
func (m EditorModel) SetPulsePos(pos int) EditorModel {
	m.PulsePos = pos % 8

	return m
}

// SetPulseColors updates the accent colors used by the pulse animation.
func (m EditorModel) SetPulseColors(accent, accentBright string) EditorModel {
	m.pulseAccent = accent
	m.pulseAccentBright = accentBright

	return m
}

// PushHistory appends a submitted value to history.
func (m EditorModel) PushHistory(s string) EditorModel {
	if s == "" {
		return m
	}

	if len(m.history) > 0 && m.history[0] == s {
		return m
	}

	m.history = append([]string{s}, m.history...)
	m.histIdx = 0

	return m
}

// History returns the history slice.
func (m EditorModel) History() []string {
	return m.history
}

// Update handles messages by forwarding to the textarea and intercepting
// enter (submit), up/down (history), and alt/shift+enter (newline).
func (m EditorModel) Update(msg tea.Msg) (EditorModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if handled, model, cmd := m.handleKey(keyMsg); handled {
			return model, cmd
		}
	}

	// Forward to textarea
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(msg)

	return m, cmd
}

// handleCompletionKey processes keys when the completion popup is visible.
// Returns true if the key was handled.
func (m EditorModel) handleCompletionKey(msg tea.KeyPressMsg) (bool, EditorModel, tea.Cmd) {
	switch msg.Code {
	case tea.KeyTab:
		m.completion = m.completion.CursorDown()

		return true, m, nil
	case tea.KeyUp:
		// If actively navigating history, hide completion and continue history nav
		if m.navigating {
			return true, m.historyUp().HideCompletion(), nil
		}

		m.completion = m.completion.CursorUp()

		return true, m, nil
	case tea.KeyDown:
		// If actively navigating history, hide completion and continue history nav
		if m.navigating {
			if m.histIdx > 0 {
				return true, m.historyDown().HideCompletion(), nil
			}

			if m.histIdx == 0 {
				m.navigating = false
				m.ta.SetValue(m.savedLine)
				m.savedLine = ""

				return true, m.HideCompletion(), nil
			}
		}

		m.completion = m.completion.CursorDown()

		return true, m, nil
	case tea.KeyEnter, tea.KeyKpEnter:
		// Alt+Enter or Shift+Enter inserts a newline, not apply completion
		if msg.Mod&(tea.ModAlt|tea.ModShift) != 0 {
			return false, m, nil
		}

		m = m.applyCompletion()
		model, cmd := m.handleEnter()

		return true, model, cmd
	case tea.KeyEscape:
		return true, m.HideCompletion(), nil
	}

	return false, m, nil
}

// handleKey processes key-specific shortcuts (enter, up/down history).
// Returns true if the key was fully handled and should not be forwarded.
func (m EditorModel) handleKey(msg tea.KeyPressMsg) (bool, EditorModel, tea.Cmd) {
	// Completion key interception (when popup is visible)
	if m.completion.Visible() {
		if handled, model, cmd := m.handleCompletionKey(msg); handled {
			return true, model, cmd
		}
	}

	// Alt+Enter or Shift+Enter inserts a newline (plain Enter is bound to submit)
	if isEnterKey(msg.Code) && msg.Mod&(tea.ModAlt|tea.ModShift) != 0 {
		plain := msg
		plain.Mod &^= tea.ModAlt | tea.ModShift

		var cmd tea.Cmd

		m.ta, cmd = m.ta.Update(plain)

		return true, m, cmd
	}

	// Enter submits
	if isEnterKey(msg.Code) {
		model, cmd := m.handleEnter()

		return true, model, cmd
	}

	// History navigation on up/down when textarea is single-line
	if msg.Code == tea.KeyUp {
		if (m.navigating || m.ta.Line() == 0) && len(m.history) > 0 {
			return true, m.historyUp().HideCompletion(), nil
		}
	}

	if msg.Code == tea.KeyDown {
		if m.navigating && m.histIdx > 0 {
			return true, m.historyDown().HideCompletion(), nil
		}

		if m.navigating && m.histIdx == 0 {
			m.navigating = false
			m.ta.SetValue(m.savedLine)
			m.savedLine = ""

			return true, m.HideCompletion(), nil
		}
	}

	return false, m, nil
}

func (m EditorModel) handleEnter() (EditorModel, tea.Cmd) {
	text := strings.TrimSpace(m.ta.Value())

	// Always emit SubmitMsg — the model decides whether to act on empty text
	// (it checks for attachments before rejecting).
	if text != "" {
		m = m.PushHistory(text)
	}

	m.ta.Reset()
	m.navigating = false
	m.savedLine = ""

	return m, func() tea.Msg {
		return SubmitMsg{Text: text}
	}
}

func (m EditorModel) historyUp() EditorModel {
	if len(m.history) == 0 {
		return m
	}

	if !m.navigating {
		m.savedLine = m.ta.Value()
		m.navigating = true
	}

	if m.histIdx < len(m.history) {
		m.histIdx++
		m.ta.SetValue(m.history[m.histIdx-1])
	}

	return m
}

func (m EditorModel) historyDown() EditorModel {
	if m.histIdx > 1 {
		m.histIdx--
		m.ta.SetValue(m.history[m.histIdx-1])
	} else if m.histIdx == 1 {
		m.histIdx = 0
		m.ta.SetValue(m.savedLine)
		m.savedLine = ""
		m.navigating = false
	}

	return m
}

// CursorLineStart moves the cursor to the beginning of the current line.
func (m EditorModel) CursorLineStart() EditorModel {
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	_ = cmd

	return m
}

// CursorLineEnd moves the cursor to the end of the current line.
func (m EditorModel) CursorLineEnd() EditorModel {
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	_ = cmd

	return m
}

// CursorWordLeft moves the cursor one word backward.
func (m EditorModel) CursorWordLeft() EditorModel {
	// textarea handles this via key bindings, but for explicit dispatch:
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	_ = cmd

	return m
}

// CursorWordRight moves the cursor one word forward.
func (m EditorModel) CursorWordRight() EditorModel {
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	_ = cmd

	return m
}

// DeleteWordBackward deletes the word before the cursor.
func (m EditorModel) DeleteWordBackward() EditorModel {
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt})
	_ = cmd

	return m
}

// DeleteWordForward deletes the word after the cursor.
func (m EditorModel) DeleteWordForward() EditorModel {
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyDelete, Mod: tea.ModAlt})
	_ = cmd

	return m
}

// DeleteToLineStart deletes from cursor to the start of the current line.
func (m EditorModel) DeleteToLineStart() EditorModel {
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	_ = cmd

	return m
}

// DeleteToLineEnd deletes from cursor to the end of the current line.
func (m EditorModel) DeleteToLineEnd() EditorModel {
	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	_ = cmd

	return m
}

// CursorLine returns the current cursor line (0-indexed from content start).
func (m EditorModel) CursorLine() int {
	return m.ta.Line()
}

// VisualCursorLine returns the cursor's visual line position within the
// visible textarea area, accounting for scroll offset.
func (m EditorModel) VisualCursorLine() int {
	return m.ta.Line() - m.ta.ScrollYOffset()
}

// CursorColumn returns the current cursor column (0-indexed from line start).
func (m EditorModel) CursorColumn() int {
	return m.ta.Column()
}

// Completion returns the editor's completion model.
func (m EditorModel) Completion() CompletionModel {
	return m.completion
}

// ShowCompletion shows the completion popup with the given items and filter.
// triggerOffset is the byte position of the trigger character in the full value.
func (m EditorModel) ShowCompletion(kind CompletionKind, items []CompletionItem, filter string, triggerOffset int) EditorModel {
	m.completion = m.completion.Show(kind, items)
	m.completion = m.completion.SetFilter(filter)
	m.triggerOffset = triggerOffset

	return m
}

// HideCompletion hides the completion popup.
func (m EditorModel) HideCompletion() EditorModel {
	m.completion = m.completion.Hide()
	m.triggerOffset = 0

	return m
}

// CompletionActive returns whether the completion popup is currently shown.
func (m EditorModel) CompletionActive() bool {
	return m.completion.Visible()
}

// applyCompletion replaces the trigger portion of the textarea value with the
// selected completion item and hides the popup. Replaces from triggerOffset to
// the end of the token being completed (next whitespace), so cursor position
// within the token does not affect the result.
func (m EditorModel) applyCompletion() EditorModel {
	item, ok := m.completion.SelectedItem()
	if !ok {
		return m.HideCompletion()
	}

	value := m.ta.Value()

	offset := m.triggerOffset
	if offset >= 0 && offset < len(value) && value[offset] == ' ' {
		offset++ // keep the space as a separator
	}

	// Find end of token (next whitespace after trigger), not cursor position
	endOffset := offset
	if offset >= 0 {
		for endOffset < len(value) && value[endOffset] != ' ' && value[endOffset] != '\t' && value[endOffset] != '\n' {
			endOffset++
		}
	}

	if offset >= 0 && offset <= len(value) {
		m.ta.SetValue(value[:offset] + item.Value + value[endOffset:])
	}

	return m.HideCompletion()
}

// --- Selection methods ---

// StartSelection begins a new text selection at the given logical line and rune column.
func (m EditorModel) StartSelection(line, col int) EditorModel {
	m.selActive = true
	m.mouseDown = true
	m.selStartLine = line
	m.selStartCol = col
	m.selEndLine = line
	m.selEndCol = col

	return m
}

// ExtendSelection updates the end point of the current selection.
func (m EditorModel) ExtendSelection(line, col int) EditorModel {
	if !m.selActive {
		return m
	}

	m.selEndLine = line
	m.selEndCol = col

	return m
}

// EndSelection finalizes the current selection and normalizes start <= end.
func (m EditorModel) EndSelection() EditorModel {
	m.mouseDown = false

	if m.selStartLine > m.selEndLine {
		m.selStartLine, m.selEndLine = m.selEndLine, m.selStartLine
		m.selStartCol, m.selEndCol = m.selEndCol, m.selStartCol
	} else if m.selStartLine == m.selEndLine && m.selStartCol > m.selEndCol {
		m.selStartCol, m.selEndCol = m.selEndCol, m.selStartCol
	}

	return m
}

// ClearSelection removes the active selection.
func (m EditorModel) ClearSelection() EditorModel {
	m.selActive = false
	m.mouseDown = false
	m.selStartLine = 0
	m.selStartCol = 0
	m.selEndLine = 0
	m.selEndCol = 0

	return m
}

// HasSelection returns true if there is an active non-empty selection.
func (m EditorModel) HasSelection() bool {
	if !m.selActive {
		return false
	}

	if m.selStartLine == m.selEndLine {
		return m.selStartCol != m.selEndCol
	}

	return true
}

// MouseDown returns true if the mouse button is held down during a drag.
func (m EditorModel) MouseDown() bool {
	return m.mouseDown
}

// SelectionBounds returns normalized (startLine, startCol, endLine, endCol).
func (m EditorModel) SelectionBounds() (int, int, int, int) {
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

// ExtractSelection returns the plain text within the current selection.
func (m EditorModel) ExtractSelection() string {
	if !m.HasSelection() {
		return ""
	}

	sl, sc, el, ec := m.SelectionBounds()
	lines := strings.Split(m.Value(), "\n")

	if sl < 0 {
		sl = 0
	}

	if el >= len(lines) {
		el = len(lines) - 1
	}

	if sl >= len(lines) {
		return ""
	}

	var selected []string

	for line := sl; line <= el; line++ {
		runes := []rune(lines[line])
		startCol, endCol := 0, len(runes)

		if line == sl {
			startCol = min(sc, len(runes))
		}

		if line == el {
			endCol = min(ec, len(runes))
		}

		if startCol > endCol {
			startCol = endCol
		}

		selected = append(selected, string(runes[startCol:endCol]))
	}

	return strings.Join(selected, "\n")
}

// SelectWord selects the word at the given logical line and rune column.
func (m EditorModel) SelectWord(line, col int) EditorModel {
	lines := strings.Split(m.Value(), "\n")
	if line < 0 || line >= len(lines) {
		return m
	}

	runes := []rune(lines[line])
	if len(runes) == 0 {
		return m
	}

	start, end := findWordBounds(runes, col)
	if start < 0 {
		return m
	}

	m.selActive = true
	m.mouseDown = false
	m.selStartLine = line
	m.selStartCol = start
	m.selEndLine = line
	m.selEndCol = end

	return m
}

// --- Coordinate mapping ---

// ScrollYOffset returns the textarea's vertical scroll offset.
func (m EditorModel) ScrollYOffset() int {
	return m.ta.ScrollYOffset()
}

// ContentWidth returns the textarea's internal content (wrapping) width.
func (m EditorModel) ContentWidth() int {
	return m.ta.Width()
}

// VisualLineToLogical maps a global visual line index to a logical line index
// and row offset within that logical line's wrapped content.
func (m EditorModel) VisualLineToLogical(globalVLine int) (logicalLine, rowOffset int) {
	lines := strings.Split(m.Value(), "\n")
	w := m.ta.Width()

	if w <= 0 {
		w = 80
	}

	accumulated := 0

	for i, line := range lines {
		vlCount := max(1, (uniseg.StringWidth(line)+w-1)/w)

		if accumulated+vlCount > globalVLine {
			return i, globalVLine - accumulated
		}

		accumulated += vlCount
	}

	if len(lines) == 0 {
		return 0, 0
	}

	return len(lines) - 1, 0
}

// ColFromWrapped maps a row offset and screen column to a rune column
// within the logical line.
func (m EditorModel) ColFromWrapped(logicalLine, rowOffset, screenCol int) int {
	lines := strings.Split(m.Value(), "\n")

	if logicalLine < 0 || logicalLine >= len(lines) {
		return 0
	}

	w := m.ta.Width()

	if w <= 0 {
		w = 80
	}

	runeOffset := rowOffset*w + screenCol
	runes := []rune(lines[logicalLine])

	return min(runeOffset, len(runes))
}

// View renders the editor.
func (m EditorModel) View() string {
	return m.ta.View()
}

// Draw renders the editor into an ultraviolet screen buffer region.
func (m EditorModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
	m.drawSelectionHighlight(scr, area)

	if m.PulseActive {
		m.drawPulse(scr, area)
	}
}

// drawPulse overlays colored border cells for the pulse animation.
// Positions 0-3 are corners (TL, TR, BR, BL), positions 4-7 are edges (top, right, bottom, left).
func (m EditorModel) drawPulse(scr uv.Screen, area uv.Rectangle) {
	type segment struct {
		x, y int
	}

	w := area.Dx()
	h := area.Dy()

	// Build 8 segments: 4 corners + 4 edge midpoints
	segments := [8]segment{
		{area.Min.X, area.Min.Y},           // 0: TL
		{area.Max.X - 1, area.Min.Y},       // 1: TR
		{area.Max.X - 1, area.Max.Y - 1},   // 2: BR
		{area.Min.X, area.Max.Y - 1},       // 3: BL
		{area.Min.X + w/2, area.Min.Y},     // 4: top edge
		{area.Max.X - 1, area.Min.Y + h/2}, // 5: right edge
		{area.Min.X + w/2, area.Max.Y - 1}, // 6: bottom edge
		{area.Min.X, area.Min.Y + h/2},     // 7: left edge
	}

	// Compute colors: AccentBright at current position, Accent at trailing, BorderFocused for rest
	pos := m.PulsePos % 8
	trailing := (pos - 1 + 8) % 8

	for i, seg := range segments {
		var color string

		switch i {
		case pos:
			color = m.pulseAccentBright
			if color == "" {
				color = palette.DefaultTheme().AccentBright
			}
		case trailing:
			color = m.pulseAccent
			if color == "" {
				color = palette.DefaultTheme().Accent
			}
		default:
			continue // don't override other border cells
		}

		cell := scr.CellAt(seg.x, seg.y)
		if cell != nil && !cell.IsZero() {
			newCell := cell.Clone()
			newCell.Style.Fg = lipgloss.Color(color)
			scr.SetCell(seg.x, seg.y, newCell)
		}
	}
}

func (m EditorModel) drawSelectionHighlight(scr uv.Screen, area uv.Rectangle) {
	if !m.selActive {
		return
	}

	sl, sc, el, ec := m.SelectionBounds()
	if sl == el && sc == ec {
		return
	}

	contentX := area.Min.X + 2
	contentY := area.Min.Y + 1
	contentW := area.Dx() - 3
	contentH := area.Dy() - 2

	if contentW <= 0 || contentH <= 0 {
		return
	}

	scrollOffset := m.ta.ScrollYOffset()
	lines := strings.Split(m.Value(), "\n")
	wrapWidth := m.ta.Width()

	if wrapWidth <= 0 {
		wrapWidth = contentW
	}

	visualLine := 0

	for logLine := range lines {
		vlCount := max(1, (uniseg.StringWidth(lines[logLine])+wrapWidth-1)/wrapWidth)

		for vlRow := range vlCount {
			globalVLine := visualLine + vlRow

			if globalVLine < scrollOffset {
				continue
			}

			screenRow := globalVLine - scrollOffset
			if screenRow >= contentH {
				return
			}

			if logLine < sl || logLine > el {
				continue
			}

			startCol, endCol := m.selectionSpan(logLine, vlRow, wrapWidth, sl, sc, el, ec, contentW)
			if startCol >= endCol {
				continue
			}

			for x := contentX + startCol; x < contentX+endCol; x++ {
				if cell := scr.CellAt(x, contentY+screenRow); cell != nil {
					cell.Style.Attrs |= uv.AttrReverse
				}
			}
		}

		visualLine += vlCount
	}
}

// selectionSpan computes the screen column span for a visual line within the selection.
func (m EditorModel) selectionSpan(logLine, vlRow, wrapWidth, sl, sc, el, ec, contentW int) (startCol, endCol int) {
	startCol = 0
	endCol = contentW

	screenStart := sc - vlRow*wrapWidth
	if vlRow == 0 {
		screenStart = sc
	}

	if logLine == sl {
		if screenStart > endCol {
			return contentW, contentW
		}

		if screenStart > startCol {
			startCol = screenStart
		}
	}

	screenEnd := ec - vlRow*wrapWidth
	if vlRow == 0 {
		screenEnd = ec
	}

	if logLine == el {
		if screenEnd < startCol {
			return 0, 0
		}

		if screenEnd < endCol {
			endCol = screenEnd
		}
	}

	return startCol, endCol
}
