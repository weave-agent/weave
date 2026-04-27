package components

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewSpinnerModel(t *testing.T) {
	s := NewSpinnerModel()
	assert.False(t, s.Visible())
}

func TestSpinnerModel_Show(t *testing.T) {
	s := NewSpinnerModel().Show()
	assert.True(t, s.Visible())
}

func TestSpinnerModel_Hide(t *testing.T) {
	s := NewSpinnerModel().Show().Hide()
	assert.False(t, s.Visible())
}

func TestSpinnerModel_ViewHidden(t *testing.T) {
	s := NewSpinnerModel()
	assert.Empty(t, s.View())
}

func TestSpinnerModel_ViewVisible(t *testing.T) {
	s := NewSpinnerModel().Show()
	view := s.View()
	assert.Contains(t, view, "Thinking...")
}

func TestSpinnerModel_SetLabel(t *testing.T) {
	s := NewSpinnerModel().Show().SetLabel("Loading...")
	view := s.View()
	assert.Contains(t, view, "Loading...")
}

func TestSpinnerModel_SetSize(t *testing.T) {
	s := NewSpinnerModel().SetSize(120)
	assert.Equal(t, 120, s.width)
}

func TestSpinnerModel_UpdateAdvancesFrame(t *testing.T) {
	s := NewSpinnerModel().Show()

	// Simulate a tick message
	tick := spinner.TickMsg{Time: time.Now()}
	s, cmd := s.Update(tick)
	assert.True(t, s.Visible())
	assert.NotNil(t, cmd) // should return next tick cmd
}

func TestSpinnerModel_UpdateIgnoredWhenHidden(t *testing.T) {
	s := NewSpinnerModel()
	tick := spinner.TickMsg{Time: time.Now()}
	_, cmd := s.Update(tick)
	assert.Nil(t, cmd)
}

func TestSpinnerModel_SpinnerUpdate_ShowMsg(t *testing.T) {
	s := NewSpinnerModel()
	s, cmd := s.SpinnerUpdate(SpinnerShowMsg{})
	assert.True(t, s.Visible())
	assert.NotNil(t, cmd) // starts ticking
}

func TestSpinnerModel_SpinnerUpdate_HideMsg(t *testing.T) {
	s := NewSpinnerModel().Show()
	s, cmd := s.SpinnerUpdate(SpinnerHideMsg{})
	assert.False(t, s.Visible())
	assert.Nil(t, cmd)
}

func TestSpinnerModel_SpinnerUpdate_OtherMsg(t *testing.T) {
	s := NewSpinnerModel().Show()
	s, cmd := s.SpinnerUpdate(nil)
	assert.True(t, s.Visible()) // unchanged
	assert.Nil(t, cmd)
}

func TestIsSpinnerMsg(t *testing.T) {
	assert.True(t, IsSpinnerMsg(spinner.TickMsg{Time: time.Now()}))
	assert.True(t, IsSpinnerMsg(SpinnerShowMsg{}))
	assert.True(t, IsSpinnerMsg(SpinnerHideMsg{}))
	assert.False(t, IsSpinnerMsg(nil))
}

func TestStartSpinner(t *testing.T) {
	cmd := StartSpinner()
	assert.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SpinnerShowMsg)
	assert.True(t, ok)
}

func TestStopSpinner(t *testing.T) {
	cmd := StopSpinner()
	assert.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SpinnerHideMsg)
	assert.True(t, ok)
}

func TestRenderSpinnerClean(t *testing.T) {
	result := RenderSpinnerClean(0, "test", 0)
	assert.Contains(t, result, "test")
}

func TestRenderSpinnerClean_Truncates(t *testing.T) {
	result := RenderSpinnerClean(0, "Thinking...", 5)
	assert.LessOrEqual(t, utf8.RuneCountInString(result), 5)
}

// Verify spinner.TickMsg is properly handled as a tea.Msg
func TestSpinnerTickMsgIsTeaMsg(t *testing.T) {
	var _ tea.Msg = spinner.TickMsg{}
}
