package overlays

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewLoginModel(t *testing.T) {
	m := NewLoginModel("OpenAI", "https://example.com/auth")
	assert.Equal(t, "OpenAI", m.provider)
	assert.Equal(t, "https://example.com/auth", m.authURL)
	assert.Equal(t, "Waiting for authorization...", m.status)
	assert.False(t, m.visible)
}

func TestLoginModel_ShowHide(t *testing.T) {
	m := NewLoginModel("Test", "https://example.com/auth")
	m = m.Show()
	assert.True(t, m.Visible())

	m = m.Hide()
	assert.False(t, m.Visible())
}

func TestLoginModel_SetSize(t *testing.T) {
	m := NewLoginModel("Test", "https://example.com/auth")
	m = m.SetSize(80, 24)
	assert.Equal(t, 80, m.width)
	assert.Equal(t, 24, m.height)
}

func TestLoginModel_SetStatus(t *testing.T) {
	m := NewLoginModel("Test", "https://example.com/auth")
	m = m.SetStatus("Authentication successful")
	assert.Equal(t, "Authentication successful", m.status)
}

func TestLoginModel_Update_Escape(t *testing.T) {
	m := NewLoginModel("Test", "https://example.com/auth")
	m = m.Show()

	newM, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	assert.False(t, newM.visible)
	assert.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(LoginCancelledMsg)
	assert.True(t, ok, "expected LoginCancelledMsg")
}

func TestLoginModel_Update_OtherKey(t *testing.T) {
	m := NewLoginModel("Test", "https://example.com/auth")
	m = m.Show()

	newM, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, newM.visible)
	assert.Nil(t, cmd)
}

func TestLoginModel_View_NotVisible(t *testing.T) {
	m := NewLoginModel("Test", "https://example.com/auth")
	assert.Empty(t, m.View())
}

func TestLoginModel_View_Visible(t *testing.T) {
	m := NewLoginModel("OpenAI", "https://platform.openai.com/authorize")
	m = m.Show().SetSize(80, 24)

	view := m.View()
	assert.Contains(t, view, "Authenticate with OpenAI")
	assert.Contains(t, view, "Waiting for authorization...")
	assert.Contains(t, view, "Esc to cancel")
}

func TestLoginModel_View_LongURL(t *testing.T) {
	m := NewLoginModel("Test", "https://example.com/very/long/url/that/needs/truncation")
	m = m.Show().SetSize(30, 10)

	view := m.View()
	assert.Contains(t, view, "...")
}

func TestLoginDialog_Adapter(t *testing.T) {
	model := NewLoginModel("Test", "https://example.com/auth").Show()
	dialog := NewLoginDialog("login-test", model)

	assert.Equal(t, "login-test", dialog.ID())
	assert.False(t, dialog.Done())

	// Test cancellation
	newD, _ := dialog.Update(LoginCancelledMsg{})
	updated := newD.(*LoginDialog)
	assert.True(t, updated.Done())
	assert.Error(t, updated.Result().Err)
}

func TestLoginDialog_Handles(t *testing.T) {
	model := NewLoginModel("Test", "https://example.com/auth")
	dialog := NewLoginDialog("login-test", model)

	assert.True(t, dialog.Handles(tea.KeyPressMsg{Code: tea.KeyEsc}))
	assert.True(t, dialog.Handles(LoginCancelledMsg{}))
	assert.False(t, dialog.Handles("some other msg"))
}

func TestLoginDialog_SetSize(t *testing.T) {
	model := NewLoginModel("Test", "https://example.com/auth")
	dialog := NewLoginDialog("login-test", model)

	newD := dialog.SetSize(100, 30)
	assert.Equal(t, 100, newD.(*LoginDialog).model.width)
	assert.Equal(t, 30, newD.(*LoginDialog).model.height)
}
