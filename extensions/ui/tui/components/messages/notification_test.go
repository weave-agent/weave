package messages

import (
	"testing"

	"github.com/weave-agent/weave-tui/palette"
	"github.com/weave-agent/weave/sdk"

	"github.com/stretchr/testify/assert"
)

func TestNotificationMessage_Content(t *testing.T) {
	m := NewNotificationMessage("hello world", sdk.NotifyInfo)
	assert.Equal(t, "hello world", m.Content())
	assert.Equal(t, sdk.NotifyInfo, m.Level())
}

func TestNotificationMessage_View_InfoLevel(t *testing.T) {
	m := NewNotificationMessage("info message", sdk.NotifyInfo)
	view := m.View(80)

	theme := palette.DefaultTheme()

	assert.Contains(t, view, "info message")
	assert.Contains(t, view, "│")
	assert.Contains(t, view, "◆ ")
	// Info uses accent color for border
	assert.Contains(t, view, theme.Accent)
}

func TestNotificationMessage_View_WarningLevel(t *testing.T) {
	m := NewNotificationMessage("warning message", sdk.NotifyWarning)
	view := m.View(80)

	theme := palette.DefaultTheme()

	assert.Contains(t, view, "warning message")
	// Warning uses warning color
	assert.Contains(t, view, theme.Warning)
}

func TestNotificationMessage_View_ErrorLevel(t *testing.T) {
	m := NewNotificationMessage("error message", sdk.NotifyError)
	view := m.View(80)

	theme := palette.DefaultTheme()

	assert.Contains(t, view, "error message")
	// Error uses error color
	assert.Contains(t, view, theme.Error)
}

func TestNotificationMessage_View_SuccessLevel(t *testing.T) {
	m := NewNotificationMessage("success message", sdk.NotifySuccess)
	view := m.View(80)

	theme := palette.DefaultTheme()

	assert.Contains(t, view, "success message")
	// Success uses success color
	assert.Contains(t, view, theme.Success)
}

func TestNotificationMessage_View_Multiline(t *testing.T) {
	m := NewNotificationMessage("line one\nline two", sdk.NotifyInfo)
	view := m.View(80)

	assert.Contains(t, view, "line one")
	assert.Contains(t, view, "line two")
}

func TestNotificationMessage_View_ZeroWidth(t *testing.T) {
	m := NewNotificationMessage("test", sdk.NotifyInfo)
	view := m.View(0)

	assert.Contains(t, view, "test")
}

func TestColorsForLevel(t *testing.T) {
	theme := palette.DefaultTheme()

	border, text := colorsForLevel(sdk.NotifyInfo, theme)
	assert.Equal(t, theme.Accent, border)
	assert.Equal(t, theme.Foreground, text)

	border, text = colorsForLevel(sdk.NotifyWarning, theme)
	assert.Equal(t, theme.Warning, border)
	assert.Equal(t, theme.Warning, text)

	border, text = colorsForLevel(sdk.NotifyError, theme)
	assert.Equal(t, theme.Error, border)
	assert.Equal(t, theme.Error, text)

	border, text = colorsForLevel(sdk.NotifySuccess, theme)
	assert.Equal(t, theme.Success, border)
	assert.Equal(t, theme.Success, text)
}
