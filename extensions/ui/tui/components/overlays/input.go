package overlays

import (
	"fmt"
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// InputResultMsg is emitted when the user submits or cancels the input.
type InputResultMsg struct {
	Value string
	Ok    bool
}

// InputModel is a single-line input modal overlay.
type InputModel struct {
	prompt  string
	value   []rune
	cursor  int
	mask    rune
	width   int
	height  int
	visible bool
}

// NewInputModel creates a new input model.
func NewInputModel(prompt string) InputModel {
	return InputModel{
		prompt: prompt,
	}
}

// Visible returns whether the input modal is shown.
func (m InputModel) Visible() bool { return m.visible }

// Show makes the input modal visible and resets value.
func (m InputModel) Show() InputModel {
	m.visible = true
	m.value = nil
	m.cursor = 0

	return m
}

// Hide hides the input modal.
func (m InputModel) Hide() InputModel {
	m.visible = false
	return m
}

// SetSize updates the input modal dimensions.
func (m InputModel) SetSize(width, height int) InputModel {
	m.width = width
	m.height = height

	return m
}

// Width returns the input modal width.
func (m InputModel) Width() int { return m.width }

// Height returns the input modal height.
func (m InputModel) Height() int { return m.height }

// Cursor returns the current cursor position.
func (m InputModel) Cursor() int { return m.cursor }

// SetMask sets the mask character for the input (0 means no masking).
func (m InputModel) SetMask(mask rune) InputModel {
	m.mask = mask
	return m
}

// Mask returns the current mask character (0 means no masking).
func (m InputModel) Mask() rune { return m.mask }

// Value returns the current input value.
func (m InputModel) Value() string { return string(m.value) }

// Update handles messages for the input modal.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleKey(key)
	}

	return m, nil
}

func (m InputModel) handleKey(msg tea.KeyPressMsg) (InputModel, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEsc:
		m.visible = false
		return m, func() tea.Msg { return InputResultMsg{Ok: false} }

	case tea.KeyEnter:
		val := string(m.value)
		m.visible = false

		return m, func() tea.Msg { return InputResultMsg{Value: val, Ok: true} }

	case tea.KeyBackspace:
		if m.cursor > 0 {
			m.value = append(m.value[:m.cursor-1], m.value[m.cursor:]...)
			m.cursor--
		}

		return m, nil

	case tea.KeyDelete:
		if m.cursor < len(m.value) {
			m.value = append(m.value[:m.cursor], m.value[m.cursor+1:]...)
		}

		return m, nil

	case tea.KeyLeft:
		if m.cursor > 0 {
			m.cursor--
		}

		return m, nil

	case tea.KeyRight:
		if m.cursor < len(m.value) {
			m.cursor++
		}

		return m, nil

	default:
		if msg.Text != "" {
			runes := []rune(msg.Text)
			tail := make([]rune, len(m.value[m.cursor:]))
			copy(tail, m.value[m.cursor:])
			m.value = append(m.value[:m.cursor], runes...)
			m.value = append(m.value, tail...)
			m.cursor += len(runes)

			return m, nil
		}
	}

	return m, nil
}

// renderCursor returns the cursor line with the block cursor character
// inserted at the current cursor position, applying masking if enabled.
func (m InputModel) renderCursor() string {
	if m.cursor > len(m.value) {
		var displayValue string
		if m.mask != 0 {
			displayValue = strings.Repeat(string(m.mask), len(m.value))
		} else {
			displayValue = string(m.value)
		}

		return displayValue + "▎"
	}

	var before, after string

	if m.cursor > 0 {
		if m.mask != 0 {
			before = strings.Repeat(string(m.mask), m.cursor)
		} else {
			before = string(m.value[:m.cursor])
		}
	}

	if m.cursor < len(m.value) {
		if m.mask != 0 {
			after = strings.Repeat(string(m.mask), len(m.value)-m.cursor)
		} else {
			after = string(m.value[m.cursor:])
		}
	}

	return fmt.Sprintf("%s▎%s", before, after)
}

// View renders the input modal overlay.
func (m InputModel) View() string {
	if !m.visible || m.width < 4 {
		return ""
	}

	theme := palette.DefaultTheme()
	boxWidth := min(50, m.width-4)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.BorderFocused)).
		Width(boxWidth-2).
		Padding(0, 1)

	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground))

	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.MutedBright))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Muted))

	cursor := m.renderCursor()

	content := promptStyle.Render(m.prompt) + "\n" + inputStyle.Render(cursor) + "\n" + hintStyle.Render("Enter to confirm · Esc to cancel")
	box := borderStyle.Render(content)

	lines := strings.Split(box, "\n")

	return lipgloss.NewStyle().
		MarginTop(max(0, (m.height-len(lines))/2)).
		MarginLeft(max(0, (m.width-boxWidth)/2)).
		Render(strings.Join(lines, "\n"))
}

// Draw renders the input modal overlay into a screen buffer region.
func (m InputModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if !m.visible || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
}
