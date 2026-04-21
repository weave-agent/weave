package components

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestNewSpinnerModel(t *testing.T) {
	s := NewSpinnerModel()
	assert.False(t, s.Visible())
	assert.Equal(t, 0, s.Frame())
}

func TestSpinnerModel_Show(t *testing.T) {
	s := NewSpinnerModel().Show()
	assert.True(t, s.Visible())
}

func TestSpinnerModel_Hide(t *testing.T) {
	s := NewSpinnerModel().Show().Hide()
	assert.False(t, s.Visible())
	assert.Equal(t, 0, s.Frame())
}

func TestSpinnerModel_HideResetsFrame(t *testing.T) {
	s := NewSpinnerModel().Show()
	// Simulate some frames
	for range 5 {
		s, _ = s.Update(tickMsg{})
	}
	assert.True(t, s.Frame() > 0)

	s = s.Hide()
	assert.Equal(t, 0, s.Frame())
}

func TestSpinnerModel_ViewHidden(t *testing.T) {
	s := NewSpinnerModel()
	assert.Empty(t, s.View())
}

func TestSpinnerModel_ViewVisible(t *testing.T) {
	s := NewSpinnerModel().Show()
	view := s.View()
	assert.Contains(t, view, "Thinking...")
	// Should contain a spinner character
	found := false
	for _, ch := range SpinnerCharSet {
		if containsStr(view, ch) {
			found = true
			break
		}
	}
	assert.True(t, found, "spinner view should contain a spinner character")
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
	assert.Equal(t, 0, s.Frame())

	s, cmd := s.Update(tickMsg{})
	assert.Equal(t, 1, s.Frame())
	assert.NotNil(t, cmd) // should return next tick cmd
}

func TestSpinnerModel_UpdateIgnoredWhenHidden(t *testing.T) {
	s := NewSpinnerModel()
	s, cmd := s.Update(tickMsg{})
	assert.Equal(t, 0, s.Frame())
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
	s, cmd := s.SpinnerUpdate(nil) //nolint:staticcheck // testing nil msg
	assert.True(t, s.Visible())    // unchanged
	assert.Nil(t, cmd)
}

func TestIsSpinnerMsg(t *testing.T) {
	assert.True(t, IsSpinnerMsg(SpinnerShowMsg{}))
	assert.True(t, IsSpinnerMsg(SpinnerHideMsg{}))
	assert.True(t, IsSpinnerMsg(tickMsg{}))
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

func TestSpinnerFrameWraps(t *testing.T) {
	s := NewSpinnerModel().Show()
	for i := 0; i < len(SpinnerCharSet); i++ {
		s, _ = s.Update(tickMsg{})
	}
	assert.Equal(t, 0, s.Frame()) // should wrap around
}

func TestRenderSpinnerClean(t *testing.T) {
	result := RenderSpinnerClean(0, "test", 0)
	assert.Contains(t, result, "test")
}

func TestRenderSpinnerClean_Truncates(t *testing.T) {
	result := RenderSpinnerClean(0, "Thinking...", 5)
	assert.LessOrEqual(t, utf8.RuneCountInString(result), 5)
}

// helper
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && len(sub) > 0 && findSubstr(s, sub)))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
