package overlays

import (
	"fmt"
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// MultiSelectResultMsg is emitted when the user confirms or cancels multi-selection.
type MultiSelectResultMsg struct {
	Selected []int
	Ok       bool
}

// MultiSelectModel is a checkbox list overlay for selecting multiple items.
type MultiSelectModel struct {
	title    string
	items    []string
	selected map[int]bool
	cursor   int
	width    int
	height   int
	visible  bool
}

// NewMultiSelectModel creates a new multi-select model.
func NewMultiSelectModel(title string, items []string, defaults []bool) MultiSelectModel {
	selected := make(map[int]bool)

	for i, v := range defaults {
		if i < len(items) && v {
			selected[i] = true
		}
	}

	return MultiSelectModel{
		title:    title,
		items:    items,
		selected: selected,
		cursor:   0,
	}
}

// Visible returns whether the multi-select is shown.
func (m MultiSelectModel) Visible() bool { return m.visible }

// Show makes the multi-select visible.
func (m MultiSelectModel) Show() MultiSelectModel {
	m.visible = true
	return m
}

// Hide hides the multi-select.
func (m MultiSelectModel) Hide() MultiSelectModel {
	m.visible = false
	return m
}

// SetSize updates the multi-select dimensions.
func (m MultiSelectModel) SetSize(width, height int) MultiSelectModel {
	m.width = width
	m.height = height

	return m
}

// Width returns the multi-select width.
func (m MultiSelectModel) Width() int { return m.width }

// Height returns the multi-select height.
func (m MultiSelectModel) Height() int { return m.height }

// Cursor returns the current cursor position.
func (m MultiSelectModel) Cursor() int { return m.cursor }

// Selected returns a copy of the selected indices.
func (m MultiSelectModel) Selected() []int {
	var result []int

	for i := range len(m.items) {
		if m.selected[i] {
			result = append(result, i)
		}
	}

	return result
}

// IsSelected returns whether the item at the given index is selected.
func (m MultiSelectModel) IsSelected(index int) bool {
	return m.selected[index]
}

// Update handles messages for the multi-select.
func (m MultiSelectModel) Update(msg tea.Msg) (MultiSelectModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleKey(key)
	}

	return m, nil
}

func (m MultiSelectModel) handleKey(msg tea.KeyPressMsg) (MultiSelectModel, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEsc:
		m.visible = false
		return m, func() tea.Msg { return MultiSelectResultMsg{Ok: false} }

	case tea.KeyUp:
		if len(m.items) > 0 {
			m.cursor = max(0, m.cursor-1)
		}

		return m, nil

	case tea.KeyDown:
		if len(m.items) > 0 {
			m.cursor = min(len(m.items)-1, m.cursor+1)
		}

		return m, nil

	case tea.KeyEnter:
		if msg.Mod&tea.ModCtrl != 0 {
			// Ctrl+Enter confirms selection
			m.visible = false

			return m, func() tea.Msg {
				return MultiSelectResultMsg{Selected: m.Selected(), Ok: true}
			}
		}
		// Plain Enter toggles selection
		if len(m.items) > 0 && m.cursor >= 0 && m.cursor < len(m.items) {
			m.selected[m.cursor] = !m.selected[m.cursor]
		}

		return m, nil

	case tea.KeySpace:
		if len(m.items) > 0 && m.cursor >= 0 && m.cursor < len(m.items) {
			m.selected[m.cursor] = !m.selected[m.cursor]
		}

		return m, nil

	default:
		return m, nil
	}
}

// View renders the multi-select overlay.
func (m MultiSelectModel) View() string {
	if !m.visible || m.width < 4 {
		return ""
	}

	theme := palette.DefaultTheme()

	boxWidth := min(60, m.width-4)
	boxHeight := min(15, m.height-2)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Accent)).
		Width(boxWidth-2).
		Padding(0, 1)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground)).
		Bold(true)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground)).
		Background(lipgloss.Color(theme.Accent))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.MutedBright))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Muted))

	// Item list
	maxItems := boxHeight - 5 // room for header + hint + borders + padding

	var listLines []string

	for i, item := range m.items {
		if i >= maxItems {
			listLines = append(listLines, mutedStyle.Render("  ..."))
			break
		}

		check := "☐"
		if m.selected[i] {
			check = "☑"
		}

		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}

		line := fmt.Sprintf("%s%s %s", prefix, check, item)

		if i == m.cursor {
			listLines = append(listLines, selectedStyle.Render(line))
		} else {
			listLines = append(listLines, normalStyle.Render(line))
		}
	}

	if len(m.items) == 0 {
		listLines = append(listLines, mutedStyle.Render("  No items"))
	}

	hint := mutedStyle.Render("Enter to toggle · Ctrl+Enter to confirm · Esc to cancel")
	content := titleStyle.Render(m.title) + "\n" + strings.Join(listLines, "\n") + "\n" + hint
	box := borderStyle.Render(content)

	// Center the box
	lines := strings.Split(box, "\n")

	return lipgloss.NewStyle().
		MarginTop(max(0, (m.height-len(lines))/2)).
		MarginLeft(max(0, (m.width-boxWidth)/2)).
		Render(strings.Join(lines, "\n"))
}

// Draw renders the multi-select overlay into a screen buffer region.
func (m MultiSelectModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if !m.visible || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
}
