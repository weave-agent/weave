package components

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// SubmitMsg is emitted when the user submits the editor content.
type SubmitMsg struct {
	Text string
}

// slashCommands is the default list of slash commands for autocomplete.
// Override via SetSlashCommands.
var slashCommands = []string{"/clear", "/compact", "/help", "/model", "/name", "/new", "/quit", "/resume"}

// EditorModel is a multi-line input with history and autocomplete.
type EditorModel struct {
	value       []rune
	cursor      int // rune position in value
	width       int
	height      int
	focused     bool
	BorderColor string

	// history
	history   []string
	histIdx   int // 0 = newest, len = no selection
	dirty     bool
	savedLine []rune // preserves current input during history navigation

	// autocomplete
	showAC    bool
	acIndex   int
	acItems   []string
	slashCmds []string

	// undo
	undoStack   [][]rune
	undoCursors []int
}

const minEditorWidth = 20

// NewEditorModel creates a new editor model.
func NewEditorModel() EditorModel {
	return EditorModel{
		height:      3,
		dirty:       true,
		slashCmds:   slashCommands,
		BorderColor: "63",
	}
}

// SetSlashCommands updates the list of slash commands for autocomplete.
func (m EditorModel) SetSlashCommands(cmds []string) EditorModel {
	m.slashCmds = cmds
	return m
}

// SetValue replaces the editor content.
func (m EditorModel) SetValue(s string) EditorModel {
	m.value = []rune(s)
	m.cursor = len(m.value)
	m.dirty = true
	m.showAC = false

	return m
}

// Value returns the current editor content.
func (m EditorModel) Value() string {
	return string(m.value)
}

// SetSize updates the editor dimensions.
func (m EditorModel) SetSize(width, height int) EditorModel {
	m.width = max(minEditorWidth, width)
	m.height = max(1, height)

	return m
}

// Width returns the editor width.
func (m EditorModel) Width() int { return m.width }

// Height returns the editor height.
func (m EditorModel) Height() int { return m.height }

// Focused returns whether the editor has focus.
func (m EditorModel) Focused() bool { return m.focused }

// AutocompleteVisible returns whether the autocomplete dropdown is showing.
func (m EditorModel) AutocompleteVisible() bool { return m.showAC }

// Focus gives the editor focus.
func (m EditorModel) Focus() EditorModel {
	m.focused = true
	return m
}

// Blur removes focus from the editor.
func (m EditorModel) Blur() EditorModel {
	m.focused = false
	return m
}

// SetBorderColor updates the editor border color.
func (m EditorModel) SetBorderColor(color string) EditorModel {
	m.BorderColor = color
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

// Update handles messages.
func (m EditorModel) Update(msg tea.Msg) (EditorModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleKey(msg)
	}

	return m, nil
}

//nolint:gocyclo // key dispatch naturally has many branches
func (m EditorModel) handleKey(msg tea.KeyPressMsg) (EditorModel, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEnter:
		if msg.Mod&tea.ModAlt != 0 {
			// alt+enter inserts newline
			return m.insertRune('\n'), nil
		}
		// autocomplete is visible: accept selection
		if m.showAC && len(m.acItems) > 0 {
			return m.acceptAutocomplete(), nil
		}

		return m.submit()

	case tea.KeyBackspace:
		return m.backspace(), nil

	case tea.KeyDelete:
		return m.deleteForward(), nil

	case tea.KeyTab:
		if m.showAC && len(m.acItems) > 0 {
			return m.acceptAutocomplete(), nil
		}

		return m, nil

	case tea.KeyEsc:
		m.showAC = false
		return m, nil

	case tea.KeyUp:
		if m.showAC {
			m.acIndex = max(0, m.acIndex-1)
			return m, nil
		}

		return m.historyUp(), nil

	case tea.KeyDown:
		if m.showAC {
			m.acIndex = min(len(m.acItems)-1, m.acIndex+1)
			return m, nil
		}

		return m.historyDown(), nil

	case tea.KeyLeft:
		if m.cursor > 0 {
			m.cursor--
		}

		m.showAC = false

		return m, nil

	case tea.KeyRight:
		if m.cursor < len(m.value) {
			m.cursor++
		}

		m.showAC = false

		return m, nil

	case tea.KeyHome:
		return m.CursorLineStart(), nil

	case tea.KeyEnd:
		return m.CursorLineEnd(), nil

	default:
		if msg.Text != "" {
			m = m.insertRune([]rune(msg.Text)...)
			m = m.updateAutocomplete()
			return m, nil
		}
		m.showAC = false
		return m, nil
	}
}

func (m EditorModel) submit() (EditorModel, tea.Cmd) {
	text := string(m.value)
	if text == "" {
		return m, nil
	}

	saved := m.value
	m.value = nil
	m.cursor = 0
	m.dirty = true
	m.showAC = false
	m = m.PushHistory(text)
	_ = saved

	return m, func() tea.Msg {
		return SubmitMsg{Text: text}
	}
}

func (m EditorModel) insertRune(runes ...rune) EditorModel {
	tail := make([]rune, len(m.value[m.cursor:]))
	copy(tail, m.value[m.cursor:])

	m.value = append(m.value[:m.cursor], runes...)
	m.value = append(m.value, tail...)
	m.cursor += len(runes)
	m.dirty = true

	return m
}

func (m EditorModel) backspace() EditorModel {
	if m.cursor == 0 {
		return m
	}

	m = m.saveUndo()
	m.value = append(m.value[:m.cursor-1], m.value[m.cursor:]...)
	m.cursor--
	m.dirty = true
	m.showAC = false

	return m
}

func (m EditorModel) deleteForward() EditorModel {
	if m.cursor >= len(m.value) {
		return m
	}

	m = m.saveUndo()
	m.value = append(m.value[:m.cursor], m.value[m.cursor+1:]...)
	m.dirty = true
	m.showAC = false

	return m
}

func (m EditorModel) historyUp() EditorModel {
	if len(m.history) == 0 {
		return m
	}

	if m.histIdx < len(m.history) {
		if m.histIdx == 0 {
			m.savedLine = m.value
		}

		m.histIdx++
		idx := m.histIdx - 1
		m.value = []rune(m.history[idx])
		m.cursor = len(m.value)
		m.dirty = true
	}

	return m
}

func (m EditorModel) historyDown() EditorModel {
	if m.histIdx > 1 {
		m.histIdx--
		idx := m.histIdx - 1
		m.value = []rune(m.history[idx])
		m.cursor = len(m.value)
		m.dirty = true
	} else if m.histIdx == 1 {
		m.histIdx = 0
		m.value = m.savedLine
		m.savedLine = nil
		m.cursor = len(m.value)
		m.dirty = true
	}

	return m
}

func (m EditorModel) updateAutocomplete() EditorModel {
	text := string(m.value[:m.cursor])

	// only trigger on / prefix at start of line or after newline
	lastNewline := strings.LastIndex(text, "\n")

	prefix := text
	if lastNewline >= 0 {
		prefix = text[lastNewline+1:]
	}

	// check for space before trimming
	if strings.Contains(prefix, " ") {
		m.showAC = false
		return m
	}

	prefix = strings.TrimSpace(prefix)

	if !strings.HasPrefix(prefix, "/") {
		m.showAC = false
		return m
	}

	m.acItems = nil
	for _, cmd := range m.slashCmds {
		if strings.HasPrefix(cmd, prefix) {
			m.acItems = append(m.acItems, cmd)
		}
	}

	if len(m.acItems) == 0 {
		m.showAC = false
		return m
	}

	m.showAC = true
	if m.acIndex >= len(m.acItems) {
		m.acIndex = 0
	}

	return m
}

func (m EditorModel) acceptAutocomplete() EditorModel {
	if len(m.acItems) == 0 {
		return m
	}

	selected := m.acItems[m.acIndex]

	// find the / prefix start in current value
	text := string(m.value[:m.cursor])
	lastNewline := strings.LastIndex(text, "\n")
	prefixStart := lastNewline + 1
	prefix := string(m.value[prefixStart:m.cursor])

	// replace the prefix portion with the full command
	trimmed := strings.TrimSpace(prefix)
	before, _, ok := strings.Cut(trimmed, " ")

	replaceLen := utf8.RuneCountInString(trimmed)
	if ok {
		replaceLen = utf8.RuneCountInString(before)
	}

	// calculate actual rune positions
	replaceStart := m.cursor - runeCountUpTo(m.value, prefixStart, m.cursor, replaceLen)

	newValue := append([]rune{}, m.value[:replaceStart]...)
	newValue = append(newValue, []rune(selected)...)
	newValue = append(newValue, m.value[m.cursor:]...)

	m.value = newValue
	m.cursor = replaceStart + len([]rune(selected))
	m.dirty = true
	m.showAC = false

	return m
}

// runeCountUpTo counts runes from start position that correspond to `count` chars of trimmed prefix.
func runeCountUpTo(_ []rune, start, end, count int) int {
	return min(count, end-start)
}

// View renders the editor.
func (m EditorModel) View() string {
	if m.width <= 0 {
		return ""
	}

	borderColor := m.BorderColor
	if borderColor == "" {
		borderColor = "63"
	}

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		PaddingLeft(1)

	contentWidth := max(1, m.width-borderStyle.GetHorizontalBorderSize()-1)

	text := string(m.value)
	lines := wrapText(text, contentWidth)

	// place cursor
	cursorLine, cursorCol := cursorPosition(m.value, m.cursor, contentWidth)

	maxLines := max(1, m.height)

	for len(lines) < maxLines {
		lines = append(lines, "")
	}

	var renderedLines []string

	for i, line := range lines {
		if i >= maxLines {
			break
		}

		var rendered string

		if i == cursorLine && m.focused {
			if cursorCol < len(line) {
				rendered = line[:cursorCol] + "▎" + line[cursorCol:]
			} else {
				rendered = line + "▎"
			}
		} else {
			rendered = line
		}

		// Pad to contentWidth so border spans full width
		if rw := lipgloss.Width(rendered); rw < contentWidth {
			rendered += strings.Repeat(" ", contentWidth-rw)
		}

		renderedLines = append(renderedLines, rendered)
	}

	result := borderStyle.Render(strings.Join(renderedLines, "\n"))
	// autocomplete overlay
	if m.showAC && len(m.acItems) > 0 {
		acStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("63"))

		selectedStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("99"))

		var acLines []string

		for i, item := range m.acItems {
			if i == m.acIndex {
				acLines = append(acLines, selectedStyle.Render(item))
			} else {
				acLines = append(acLines, acStyle.Render(item))
			}
		}

		result = lipgloss.JoinVertical(lipgloss.Left, strings.Join(acLines, "\n"), result)
	}

	return result
}

// Draw renders the editor into an ultraviolet screen buffer region.
func (m EditorModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if area.Dx() <= 0 || area.Dy() <= 0 || m.width <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
}

// wrapText splits text into lines, wrapping at width.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	inputLines := strings.Split(text, "\n")

	var result []string

	for _, line := range inputLines {
		if line == "" {
			result = append(result, "")
			continue
		}

		runes := []rune(line)
		for len(runes) > 0 {
			w := min(len(runes), width)
			result = append(result, string(runes[:w]))
			runes = runes[w:]
		}
	}

	if len(result) == 0 {
		result = []string{""}
	}

	return result
}

// cursorPosition returns (line, column) for a rune cursor in wrapped text.
func cursorPosition(value []rune, cursor, width int) (int, int) {
	if width <= 0 {
		return 0, 0
	}

	line := 0
	col := 0

	for i := 0; i < cursor && i < len(value); i++ {
		if value[i] == '\n' {
			line++
			col = 0
		} else {
			col++
			if col >= width {
				line++
				col = 0
			}
		}
	}

	return line, col
}

// CursorLineStart moves the cursor to the beginning of the current line.
func (m EditorModel) CursorLineStart() EditorModel {
	for i := m.cursor - 1; i >= 0; i-- {
		if m.value[i] == '\n' {
			m.cursor = i + 1
			m.dirty = true
			m.showAC = false

			return m
		}
	}

	m.cursor = 0
	m.dirty = true
	m.showAC = false

	return m
}

// CursorLineEnd moves the cursor to the end of the current line.
func (m EditorModel) CursorLineEnd() EditorModel {
	for i := m.cursor; i < len(m.value); i++ {
		if m.value[i] == '\n' {
			m.cursor = i
			m.dirty = true
			m.showAC = false

			return m
		}
	}

	m.cursor = len(m.value)
	m.dirty = true
	m.showAC = false

	return m
}

// CursorWordLeft moves the cursor one word backward.
func (m EditorModel) CursorWordLeft() EditorModel {
	if m.cursor == 0 {
		return m
	}

	pos := m.cursor - 1

	for pos > 0 && isWordBreak(m.value[pos]) {
		pos--
	}

	for pos > 0 && !isWordBreak(m.value[pos-1]) {
		pos--
	}

	m.cursor = pos
	m.dirty = true
	m.showAC = false

	return m
}

// CursorWordRight moves the cursor one word forward.
func (m EditorModel) CursorWordRight() EditorModel {
	if m.cursor >= len(m.value) {
		return m
	}

	pos := m.cursor

	for pos < len(m.value) && !isWordBreak(m.value[pos]) {
		pos++
	}

	for pos < len(m.value) && isWordBreak(m.value[pos]) {
		pos++
	}

	m.cursor = pos
	m.dirty = true
	m.showAC = false

	return m
}

// DeleteWordBackward deletes the word before the cursor.
func (m EditorModel) DeleteWordBackward() EditorModel {
	if m.cursor == 0 {
		return m
	}

	m = m.saveUndo()

	start := m.cursor - 1

	for start > 0 && isWordBreak(m.value[start]) {
		start--
	}

	for start > 0 && !isWordBreak(m.value[start-1]) {
		start--
	}

	if isWordBreak(m.value[start]) {
		start++
	}

	m.value = append(m.value[:start], m.value[m.cursor:]...)
	m.cursor = start
	m.dirty = true
	m.showAC = false

	return m
}

// DeleteWordForward deletes the word after the cursor.
func (m EditorModel) DeleteWordForward() EditorModel {
	if m.cursor >= len(m.value) {
		return m
	}

	m = m.saveUndo()

	end := m.cursor

	for end < len(m.value) && !isWordBreak(m.value[end]) {
		end++
	}

	for end < len(m.value) && isWordBreak(m.value[end]) {
		end++
	}

	m.value = append(m.value[:m.cursor], m.value[end:]...)
	m.dirty = true
	m.showAC = false

	return m
}

// DeleteToLineStart deletes from cursor to the start of the current line.
func (m EditorModel) DeleteToLineStart() EditorModel {
	if m.cursor == 0 {
		return m
	}

	m = m.saveUndo()

	start := m.cursor - 1

	for start > 0 && m.value[start-1] != '\n' {
		start--
	}

	m.value = append(m.value[:start], m.value[m.cursor:]...)
	m.cursor = start
	m.dirty = true
	m.showAC = false

	return m
}

// DeleteToLineEnd deletes from cursor to the end of the current line.
func (m EditorModel) DeleteToLineEnd() EditorModel {
	if m.cursor >= len(m.value) {
		return m
	}

	m = m.saveUndo()

	end := m.cursor

	for end < len(m.value) && m.value[end] != '\n' {
		end++
	}

	m.value = append(m.value[:m.cursor], m.value[end:]...)
	m.dirty = true
	m.showAC = false

	return m
}

// Undo restores the previous editor state from the undo stack.
func (m EditorModel) Undo() EditorModel {
	if len(m.undoStack) == 0 {
		return m
	}

	m.value = m.undoStack[len(m.undoStack)-1]
	m.cursor = m.undoCursors[len(m.undoCursors)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.undoCursors = m.undoCursors[:len(m.undoCursors)-1]
	m.dirty = true
	m.showAC = false

	return m
}

// saveUndo pushes the current state onto the undo stack.
func (m EditorModel) saveUndo() EditorModel {
	saved := make([]rune, len(m.value))
	copy(saved, m.value)
	m.undoStack = append(m.undoStack, saved)
	m.undoCursors = append(m.undoCursors, m.cursor)

	if len(m.undoStack) > 100 {
		m.undoStack = m.undoStack[1:]
		m.undoCursors = m.undoCursors[1:]
	}

	return m
}

func isWordBreak(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
