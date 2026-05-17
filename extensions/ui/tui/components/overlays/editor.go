package overlays

import (
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// EditorResultMsg is emitted when the user submits or cancels the editor.
type EditorResultMsg struct {
	Value string
	Ok    bool
}

// EditorModel is a multi-line text editor modal overlay.
type EditorModel struct {
	title   string
	ta      textarea.Model
	width   int
	height  int
	visible bool
}

// NewEditorModel creates a new editor overlay model.
func NewEditorModel(title, prefill string) EditorModel {
	ta := textarea.New()
	ta.DynamicHeight = false
	ta.ShowLineNumbers = false
	ta.SetVirtualCursor(true)
	ta.Prompt = ""
	ta.Placeholder = ""
	ta.Focus()

	styles := textarea.DefaultStyles(false)
	styles.Focused.Base = lipgloss.NewStyle()
	styles.Blurred.Base = lipgloss.NewStyle()
	styles.Focused.Text = lipgloss.NewStyle()
	styles.Blurred.Text = lipgloss.NewStyle()

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
	ta.SetValue(prefill)

	return EditorModel{
		title: title,
		ta:    ta,
	}
}

// Visible returns whether the editor modal is shown.
func (m EditorModel) Visible() bool { return m.visible }

// Show makes the editor modal visible.
func (m EditorModel) Show() EditorModel {
	m.visible = true
	m.ta.Focus()

	return m
}

// Hide hides the editor modal.
func (m EditorModel) Hide() EditorModel {
	m.visible = false
	m.ta.Blur()

	return m
}

// SetSize updates the editor modal dimensions.
func (m EditorModel) SetSize(width, height int) EditorModel {
	m.width = width
	m.height = height

	// Leave room for border, title, and hint
	contentWidth := max(10, width-6)
	contentHeight := max(3, height-6)

	m.ta.SetWidth(contentWidth)
	m.ta.SetHeight(contentHeight)

	return m
}

// Width returns the editor modal width.
func (m EditorModel) Width() int { return m.width }

// Height returns the editor modal height.
func (m EditorModel) Height() int { return m.height }

// Value returns the current editor content.
func (m EditorModel) Value() string { return m.ta.Value() }

// Update handles messages for the editor modal.
func (m EditorModel) Update(msg tea.Msg) (EditorModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleKey(key)
	}

	var cmd tea.Cmd

	m.ta, cmd = m.ta.Update(msg)

	return m, cmd
}

func (m EditorModel) handleKey(msg tea.KeyPressMsg) (EditorModel, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEsc:
		m.visible = false

		return m, func() tea.Msg { return EditorResultMsg{Ok: false} }

	case tea.KeyEnter:
		if msg.Mod&tea.ModCtrl != 0 {
			// Ctrl+Enter submits
			m.visible = false
			val := m.ta.Value()

			return m, func() tea.Msg { return EditorResultMsg{Value: val, Ok: true} }
		}

		// Plain Enter inserts newline — forward to textarea
		var cmd tea.Cmd

		m.ta, cmd = m.ta.Update(msg)

		return m, cmd

	default:
		var cmd tea.Cmd

		m.ta, cmd = m.ta.Update(msg)

		return m, cmd
	}
}

// View renders the editor modal overlay.
func (m EditorModel) View() string {
	if !m.visible || m.width < 4 {
		return ""
	}

	theme := palette.DefaultTheme()
	boxWidth := min(60, m.width-4)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.BorderFocused)).
		Width(boxWidth-2).
		Padding(0, 1)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground)).
		Bold(true)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Muted))

	content := titleStyle.Render(m.title) + "\n" + m.ta.View() + "\n" + hintStyle.Render("Ctrl+Enter to confirm · Esc to cancel")
	box := borderStyle.Render(content)

	lines := strings.Split(box, "\n")

	return lipgloss.NewStyle().
		MarginTop(max(0, (m.height-len(lines))/2)).
		MarginLeft(max(0, (m.width-boxWidth)/2)).
		Render(strings.Join(lines, "\n"))
}

// Draw renders the editor modal overlay into a screen buffer region.
func (m EditorModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if !m.visible || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
}
