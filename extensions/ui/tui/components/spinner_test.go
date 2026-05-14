package components

import (
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
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

func TestSpinnerModel_Draw_Hidden(t *testing.T) {
	s := NewSpinnerModel()
	canvas := uv.NewScreenBuffer(80, 1)
	s.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Empty(t, output)
}

func TestSpinnerModel_Draw_Visible(t *testing.T) {
	s := NewSpinnerModel().Show()
	canvas := uv.NewScreenBuffer(80, 1)
	s.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "Thinking...")
}

func TestSpinnerModel_Draw_ZeroArea(t *testing.T) {
	s := NewSpinnerModel().Show()
	canvas := uv.NewScreenBuffer(80, 1)
	s.Draw(canvas, uv.Rect(0, 0, 0, 0))
}

// --- Task 6: Spinner color pulse tests ---

func TestSpinnerModel_ColorPulse_Alternates(t *testing.T) {
	s := NewSpinnerModel().Show()

	// First 2 ticks should use Primary color (63)
	for range 2 {
		s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	}

	view := s.View()
	assert.Contains(t, view, "63")

	// Next 3 ticks should use PrimaryBright color (69)
	for range 3 {
		s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	}

	view = s.View()
	assert.Contains(t, view, "69")
}

func TestSpinnerModel_ColorPulse_CyclesBack(t *testing.T) {
	s := NewSpinnerModel().Show()

	// 6 ticks complete one full cycle (3 Primary + 3 PrimaryBright)
	for range 6 {
		s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	}
	// After a full cycle, back to Primary (63)
	view := s.View()
	assert.Contains(t, view, "63")
}

func TestSpinnerModel_TickCount_Increments(t *testing.T) {
	s := NewSpinnerModel().Show()
	assert.Equal(t, 0, s.tickCount)

	s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	assert.Equal(t, 1, s.tickCount)

	s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	assert.Equal(t, 2, s.tickCount)
}

func TestSpinnerModel_NonTickMsg_DoesNotChangeTickCount(t *testing.T) {
	s := NewSpinnerModel().Show()
	assert.Equal(t, 0, s.tickCount)

	// Window size message should not increment tick count
	s, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Equal(t, 0, s.tickCount)
}
