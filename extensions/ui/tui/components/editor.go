package components

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
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

	// history
	history    []string
	histIdx    int
	savedLine  string
	navigating bool
}

const minEditorWidth = 20

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
	ta.SetVirtualCursor(false)
	ta.SetHeight(3)
	ta.Focus()

	styles := textarea.DefaultStyles(false)
	styles.Focused.Base = borderStyle("63")
	styles.Blurred.Base = borderStyle("240")
	styles.Focused.Text = lipgloss.NewStyle()
	styles.Blurred.Text = lipgloss.NewStyle()
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ta.SetStyles(styles)

	return EditorModel{
		ta:          ta,
		focused:     true,
		BorderColor: "63",
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

// SetBorderColor updates the editor border color.
func (m EditorModel) SetBorderColor(color string) EditorModel {
	m.BorderColor = color

	styles := m.ta.Styles()
	styles.Focused.Base = borderStyle(color)
	styles.Blurred.Base = borderStyle(color)
	m.ta.SetStyles(styles)

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
// enter (submit), up/down (history), and alt+enter (newline).
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

// handleKey processes key-specific shortcuts (enter, up/down history).
// Returns true if the key was fully handled and should not be forwarded.
func (m EditorModel) handleKey(msg tea.KeyPressMsg) (bool, EditorModel, tea.Cmd) {
	// Enter submits (unless alt+enter for newline)
	if msg.Code == tea.KeyEnter && msg.Mod&tea.ModAlt == 0 {
		model, cmd := m.handleEnter()

		return true, model, cmd
	}

	// History navigation on up/down when textarea is single-line
	if msg.Code == tea.KeyUp {
		if m.navigating || m.ta.Line() == 0 {
			return true, m.historyUp(), nil
		}
	}

	if msg.Code == tea.KeyDown {
		if m.navigating && m.histIdx > 0 {
			return true, m.historyDown(), nil
		}

		if m.navigating && m.histIdx == 0 {
			m.navigating = false
			m.ta.SetValue(m.savedLine)
			m.savedLine = ""

			return true, m, nil
		}
	}

	return false, m, nil
}

func (m EditorModel) handleEnter() (EditorModel, tea.Cmd) {
	text := strings.TrimSpace(m.ta.Value())
	if text == "" {
		return m, nil
	}

	m = m.PushHistory(text)
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
}
