package overlays

import (
	"errors"
	"strings"

	"github.com/weave-agent/weave-tui/palette"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// LoginCancelledMsg is emitted when the user presses Escape to cancel the login dialog.
type LoginCancelledMsg struct{}

// LoginModel is a display dialog for OAuth login progress.
// It shows the provider name, authorization URL, and current status.
type LoginModel struct {
	provider string
	authURL  string
	status   string // e.g. "Waiting for authorization..."
	width    int
	height   int
	visible  bool
}

// NewLoginModel creates a new login dialog model.
func NewLoginModel(provider, authURL string) LoginModel {
	return LoginModel{
		provider: provider,
		authURL:  authURL,
		status:   "Waiting for authorization...",
	}
}

// Visible returns whether the login dialog is shown.
func (m LoginModel) Visible() bool { return m.visible }

// Show makes the login dialog visible.
func (m LoginModel) Show() LoginModel {
	m.visible = true
	return m
}

// Hide hides the login dialog.
func (m LoginModel) Hide() LoginModel {
	m.visible = false
	return m
}

// SetSize updates the login dialog dimensions.
func (m LoginModel) SetSize(width, height int) LoginModel {
	m.width = width
	m.height = height

	return m
}

// SetStatus updates the status message displayed in the dialog.
func (m LoginModel) SetStatus(status string) LoginModel {
	m.status = status
	return m
}

// SetAuthURL updates the authorization URL displayed in the dialog.
func (m LoginModel) SetAuthURL(authURL string) LoginModel {
	m.authURL = authURL
	return m
}

// Update handles messages for the login dialog.
func (m LoginModel) Update(msg tea.Msg) (LoginModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.Code == tea.KeyEsc {
		m.visible = false
		return m, func() tea.Msg { return LoginCancelledMsg{} }
	}

	return m, nil
}

// View renders the login dialog.
func (m LoginModel) View() string {
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

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.AccentBright))

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.MutedBright))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Muted))

	// Wrap URL if it's too long
	urlText := m.authURL

	maxURLLen := boxWidth - 9
	if maxURLLen > 0 && len(urlText) > boxWidth-6 {
		urlText = urlText[:maxURLLen] + "..."
	}

	content := titleStyle.Render("Authenticate with "+m.provider) + "\n" +
		statusStyle.Render(m.status) + "\n" +
		urlStyle.Render(urlText) + "\n" +
		hintStyle.Render("Esc to cancel")

	box := borderStyle.Render(content)
	lines := strings.Split(box, "\n")

	return lipgloss.NewStyle().
		MarginTop(max(0, (m.height-len(lines))/2)).
		MarginLeft(max(0, (m.width-boxWidth)/2)).
		Render(strings.Join(lines, "\n"))
}

// Draw renders the login dialog into a screen buffer region.
func (m LoginModel) Draw(scr uv.Screen, area uv.Rectangle) {
	if !m.visible || area.Dx() <= 0 || area.Dy() <= 0 {
		return
	}

	uv.NewStyledString(m.View()).Draw(scr, area)
}

// --- Login Dialog Adapter ---

// LoginDialog wraps a LoginModel as a Dialog.
type LoginDialog struct {
	id     string
	model  LoginModel
	done   bool
	result DialogResult
}

// NewLoginDialog creates a dialog wrapping a LoginModel.
func NewLoginDialog(id string, model LoginModel) *LoginDialog {
	return &LoginDialog{id: id, model: model}
}

func (d *LoginDialog) ID() string           { return d.id }
func (d *LoginDialog) Done() bool           { return d.done }
func (d *LoginDialog) Result() DialogResult { return d.result }
func (d *LoginDialog) Model() LoginModel    { return d.model }

func (d *LoginDialog) SetAuthURL(url string) {
	d.model = d.model.SetAuthURL(url)
}

func (d *LoginDialog) SetSize(width, height int) Dialog {
	d.model = d.model.SetSize(width, height)
	return d
}

func (d *LoginDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg.(type) {
	case LoginCancelledMsg:
		d.done = true
		d.result = DialogResult{Err: errLoginCancelled}

		return d, nil

	default:
		var cmd tea.Cmd

		d.model, cmd = d.model.Update(msg)

		return d, cmd
	}
}

func (d *LoginDialog) Draw(scr uv.Screen, area uv.Rectangle) {
	d.model.Draw(scr, area)
}

func (d *LoginDialog) Handles(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyPressMsg:
		return true
	case LoginCancelledMsg:
		return true
	}

	return false
}

var errLoginCancelled = errors.New("login canceled")
