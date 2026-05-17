package overlays

import (
	"fmt"
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// SelectorItem is an item in the selector list.
type SelectorItem struct {
	Title    string
	Subtitle string
}

// SelectorSelectedMsg is emitted when the user selects an item.
type SelectorSelectedMsg struct {
	Index int
	Item  SelectorItem
}

// SelectorCancelledMsg is emitted when the user cancels selection.
type SelectorCancelledMsg struct{}

// filteredEntry pairs an item with its original index.
type filteredEntry struct {
	Item  SelectorItem
	Index int
}

// SelectorModel is a fuzzy-searchable list overlay.
type SelectorModel struct {
	title   string
	items   []SelectorItem
	filter  string
	cursor  int
	width   int
	height  int
	visible bool
}

// NewSelectorModel creates a new selector model.
func NewSelectorModel(title string, items []SelectorItem) SelectorModel {
	return SelectorModel{
		title:  title,
		items:  items,
		cursor: 0,
	}
}

// Visible returns whether the selector is shown.
func (m SelectorModel) Visible() bool { return m.visible }

// Show makes the selector visible and resets filter/cursor.
func (m SelectorModel) Show() SelectorModel {
	m.visible = true
	m.filter = ""
	m.cursor = 0

	return m
}

// Hide hides the selector.
func (m SelectorModel) Hide() SelectorModel {
	m.visible = false
	return m
}

// SetSize updates the selector dimensions.
func (m SelectorModel) SetSize(width, height int) SelectorModel {
	m.width = width
	m.height = height

	return m
}

// Width returns the selector width.
func (m SelectorModel) Width() int { return m.width }

// Height returns the selector height.
func (m SelectorModel) Height() int { return m.height }

// Cursor returns the current cursor position among filtered items.
func (m SelectorModel) Cursor() int { return m.cursor }

// Filter returns the current filter text.
func (m SelectorModel) Filter() string { return m.filter }

// filteredItems returns items matching the current filter with original indices.
func (m SelectorModel) filteredItems() []filteredEntry {
	if m.filter == "" {
		result := make([]filteredEntry, len(m.items))
		for i, item := range m.items {
			result[i] = filteredEntry{Item: item, Index: i}
		}

		return result
	}

	var matched []filteredEntry

	for i, item := range m.items {
		if fuzzyMatch(item.Title, m.filter) || fuzzyMatch(item.Subtitle, m.filter) {
			matched = append(matched, filteredEntry{Item: item, Index: i})
		}
	}

	return matched
}

// fuzzyMatch returns true if all characters in query appear in target in order.
func fuzzyMatch(target, query string) bool {
	runes := []rune(strings.ToLower(target))
	query = strings.ToLower(query)

	ti := 0

	for _, qc := range query {
		found := false

		for ti < len(runes) {
			if runes[ti] == qc {
				found = true
				ti++

				break
			}

			ti++
		}

		if !found {
			return false
		}
	}

	return true
}

// Update handles messages for the selector.
func (m SelectorModel) Update(msg tea.Msg) (SelectorModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleKey(key)
	}

	return m, nil
}

func (m SelectorModel) handleKey(msg tea.KeyPressMsg) (SelectorModel, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEsc:
		m.visible = false
		return m, func() tea.Msg { return SelectorCancelledMsg{} }

	case tea.KeyUp:
		m.cursor = max(0, m.cursor-1)
		return m, nil

	case tea.KeyDown:
		filtered := m.filteredItems()
		if len(filtered) == 0 {
			return m, nil
		}

		m.cursor = min(len(filtered)-1, m.cursor+1)

		return m, nil

	case tea.KeyEnter:
		filtered := m.filteredItems()
		if len(filtered) == 0 {
			return m, nil
		}

		m.cursor = max(0, min(m.cursor, len(filtered)-1))
		entry := filtered[m.cursor]
		m.visible = false

		return m, func() tea.Msg {
			return SelectorSelectedMsg{Index: entry.Index, Item: entry.Item}
		}

	case tea.KeyBackspace:
		if m.filter != "" {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.cursor = 0
		}

		return m, nil

	default:
		if msg.Text != "" {
			m.filter += msg.Text
			m.cursor = 0

			return m, nil
		}
	}

	return m, nil
}

// View renders the selector overlay.
func (m SelectorModel) View() string {
	if !m.visible || m.width < 4 {
		return ""
	}

	theme := palette.DefaultTheme()
	filtered := m.filteredItems()

	boxWidth := min(60, m.width-4)
	boxHeight := min(15, m.height-2)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Accent)).
		Width(boxWidth-2).
		Padding(0, 1)

	// Header with title and filter
	headerText := m.title
	if m.filter != "" {
		headerText = fmt.Sprintf("%s  / %s", m.title, m.filter)
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground)).
		Bold(true)

	filterStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.AccentBright))

	var headerRendered string

	if m.filter != "" {
		parts := strings.SplitN(headerText, "  / ", 2)
		headerRendered = titleStyle.Render(parts[0]) + "  / " + filterStyle.Render(parts[1])
	} else {
		headerRendered = titleStyle.Render(headerText)
	}

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground)).
		Background(lipgloss.Color(theme.Accent))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.MutedBright))

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Muted))

	// Item list
	maxItems := boxHeight - 4 // room for header + borders + padding

	var listLines []string

	for i, entry := range filtered {
		if i >= maxItems {
			listLines = append(listLines, subtitleStyle.Render("  ..."))
			break
		}

		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}

		line := prefix + entry.Item.Title
		if entry.Item.Subtitle != "" {
			line += "  " + entry.Item.Subtitle
		}

		if i == m.cursor {
			listLines = append(listLines, selectedStyle.Render(line))
		} else {
			listLines = append(listLines, normalStyle.Render(line))
		}
	}

	if len(filtered) == 0 {
		listLines = append(listLines, subtitleStyle.Render("  No matches"))
	}

	content := headerRendered + "\n" + strings.Join(listLines, "\n")
	box := borderStyle.Render(content)

	// Center the box
	lines := strings.Split(box, "\n")

	return lipgloss.NewStyle().
		MarginTop(max(0, (m.height-len(lines))/2)).
		MarginLeft(max(0, (m.width-boxWidth)/2)).
		Render(strings.Join(lines, "\n"))
}

// Draw renders the selector overlay into a screen buffer region.
func (m SelectorModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if !m.visible || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
}
