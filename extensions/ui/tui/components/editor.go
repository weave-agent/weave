package components

import (
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SubmitMsg is emitted when the user submits the editor content.
type SubmitMsg struct {
	Text string
}

// SlashCommands lists available slash commands for autocomplete.
// This is set externally before the editor is used.
var SlashCommands = []string{"/clear", "/compact", "/help", "/model", "/name", "/new", "/quit", "/resume"}

// EditorModel is a multi-line input with history and autocomplete.
type EditorModel struct {
	value   []rune
	cursor  int // rune position in value
	width   int
	height  int
	focused bool

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
}

// NewEditorModel creates a new editor model.
func NewEditorModel() EditorModel {
	return EditorModel{
		height:    3,
		dirty:     true,
		slashCmds: SlashCommands,
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
	m.width = width
	m.height = height
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

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m EditorModel) handleKey(msg tea.KeyMsg) (EditorModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if msg.Alt {
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
		m.cursor = 0
		m.showAC = false
		return m, nil

	case tea.KeyEnd:
		m.cursor = len(m.value)
		m.showAC = false
		return m, nil

	case tea.KeyRunes:
		m = m.insertRune(msg.Runes...)
		m = m.updateAutocomplete()
		return m, nil

	default:
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

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("63")).
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

	var b strings.Builder
	for i, line := range lines {
		if i >= maxLines {
			break
		}
		if i == cursorLine && m.focused {
			if cursorCol < len(line) {
				b.WriteString(line[:cursorCol])
				b.WriteString("▎")
				b.WriteString(line[cursorCol:])
			} else {
				b.WriteString(line)
				b.WriteString("▎")
			}
		} else {
			b.WriteString(line)
		}
		if i < maxLines-1 {
			b.WriteString("\n")
		}
	}

	result := borderStyle.Render(b.String())

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
