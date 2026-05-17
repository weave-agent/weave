package components

import (
	"testing"
	"time"

	"github.com/weave-agent/weave-tui/palette"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
)

func TestNewSpinnerModel(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme())
	assert.False(t, s.Visible())
}

func TestSpinnerModel_Show(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()
	assert.True(t, s.Visible())
}

func TestSpinnerModel_Hide(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show().Hide()
	assert.False(t, s.Visible())
}

func TestSpinnerModel_ViewHidden(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme())
	assert.Empty(t, s.View())
}

func TestSpinnerModel_ViewVisible(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()
	view := s.View()
	assert.Contains(t, view, "Thinking...")
}

func TestSpinnerModel_SetLabel(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show().SetLabel("Loading...")
	view := s.View()
	assert.Contains(t, view, "Loading...")
}

func TestSpinnerModel_SetSize(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).SetSize(120)
	assert.Equal(t, 120, s.width)
}

func TestSpinnerModel_UpdateAdvancesFrame(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()

	// Simulate a tick message
	tick := spinner.TickMsg{Time: time.Now()}
	s, cmd := s.Update(tick)
	assert.True(t, s.Visible())
	assert.NotNil(t, cmd) // should return next tick cmd
}

func TestSpinnerModel_UpdateIgnoredWhenHidden(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme())
	tick := spinner.TickMsg{Time: time.Now()}
	_, cmd := s.Update(tick)
	assert.Nil(t, cmd)
}

func TestSpinnerModel_SpinnerUpdate_ShowMsg(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme())
	s, cmd := s.SpinnerUpdate(SpinnerShowMsg{})
	assert.True(t, s.Visible())
	assert.NotNil(t, cmd) // starts ticking
}

func TestSpinnerModel_SpinnerUpdate_HideMsg(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()
	s, cmd := s.SpinnerUpdate(SpinnerHideMsg{})
	assert.False(t, s.Visible())
	assert.Nil(t, cmd)
}

func TestSpinnerModel_SpinnerUpdate_OtherMsg(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()
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

// Verify spinner.TickMsg is properly handled as a tea.Msg
func TestSpinnerTickMsgIsTeaMsg(t *testing.T) {
	var _ tea.Msg = spinner.TickMsg{}
}

func TestSpinnerModel_Draw_Hidden(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme())
	canvas := uv.NewScreenBuffer(80, 1)
	s.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Empty(t, output)
}

func TestSpinnerModel_Draw_Visible(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()
	canvas := uv.NewScreenBuffer(80, 1)
	s.Draw(canvas, canvas.Bounds())
	output := uv.TrimSpace(canvas.Render())
	assert.Contains(t, output, "Thinking...")
}

func TestSpinnerModel_Draw_ZeroArea(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()
	canvas := uv.NewScreenBuffer(80, 1)
	s.Draw(canvas, uv.Rect(0, 0, 0, 0))
}

// --- Task 6: Spinner color pulse tests ---

func TestSpinnerModel_ColorPulse_Alternates(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()

	// First 2 ticks should use Accent color (245)
	for range 2 {
		s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	}

	// Verify spinner frame has Accent color by checking screen buffer
	canvas := uv.NewScreenBuffer(80, 1)
	s.Draw(canvas, canvas.Bounds())
	// The spinner character (first cell) should have the Accent color
	cell := canvas.CellAt(0, 0)
	if cell != nil && !cell.IsZero() {
		assert.Equal(t, lipgloss.Color(palette.DefaultTheme().Accent), cell.Style.Fg)
	}

	// Next 3 ticks should use AccentBright color (248)
	for range 3 {
		s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	}

	canvas = uv.NewScreenBuffer(80, 1)
	s.Draw(canvas, canvas.Bounds())

	cell = canvas.CellAt(0, 0)
	if cell != nil && !cell.IsZero() {
		assert.Equal(t, lipgloss.Color(palette.DefaultTheme().AccentBright), cell.Style.Fg)
	}
}

func TestSpinnerModel_ColorPulse_CyclesBack(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()

	// 6 ticks complete one full cycle (3 Accent + 3 AccentBright)
	for range 6 {
		s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	}
	// After a full cycle, back to Accent
	canvas := uv.NewScreenBuffer(80, 1)
	s.Draw(canvas, canvas.Bounds())

	cell := canvas.CellAt(0, 0)
	if cell != nil && !cell.IsZero() {
		assert.Equal(t, lipgloss.Color(palette.DefaultTheme().Accent), cell.Style.Fg)
	}
}

func TestSpinnerModel_TickCount_Increments(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()
	assert.Equal(t, 0, s.tickCount)

	s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	assert.Equal(t, 1, s.tickCount)

	s, _ = s.Update(spinner.TickMsg{Time: time.Now()})
	assert.Equal(t, 2, s.tickCount)
}

func TestSpinnerModel_NonTickMsg_DoesNotChangeTickCount(t *testing.T) {
	s := NewSpinnerModel(palette.DefaultTheme()).Show()
	assert.Equal(t, 0, s.tickCount)

	// Window size message should not increment tick count
	s, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Equal(t, 0, s.tickCount)
}
