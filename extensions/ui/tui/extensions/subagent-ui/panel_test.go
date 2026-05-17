package subagent

import (
	"strings"
	"testing"
	"time"

	"github.com/weave-agent/weave/sdk"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTheme() sdk.ThemeInfo {
	return sdk.ThemeInfo{
		Accent:           "63",
		AccentDim:        "60",
		AccentBright:     "69",
		Success:          "82",
		Error:            "203",
		Warning:          "215",
		Muted:            "243",
		MutedBright:      "246",
		Foreground:       "252",
		ForegroundBright: "15",
		Border:           "240",
	}
}

func TestAgentPanelDrawer_Draw_RunningAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	agent := tracker.Start("agent-1", "researcher", "background")

	drawer := newAgentPanelDrawer(agent.ID, tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "researcher")
	assert.Contains(t, rendered, "background")
	assert.Contains(t, rendered, "●")
	assert.Regexp(t, `\d+s`, rendered)
}

func TestAgentPanelDrawer_Draw_ShowsCancelButtonForRunningAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-1", "researcher", "background")

	drawer := newAgentPanelDrawer("agent-1", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "✕ cancel")
}

func TestAgentPanelDrawer_Draw_NoCancelButtonForCompletedAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-2", "planner", "background")
	tracker.Done("agent-2", "completed", "done")

	drawer := newAgentPanelDrawer("agent-2", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.NotContains(t, rendered, "✕ cancel")
}

func TestAgentPanelDrawer_Draw_Separator(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-sep", "test", "background")

	drawer := newAgentPanelDrawer("agent-sep", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "───")
}

func TestAgentPanelDrawer_Draw_ToolLogFromRingBuffer(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-log", "researcher", "background")
	tracker.AppendOutput("agent-log", outputEntry{Type: "tool_call", Tool: "read", Content: "main.go", Time: time.Now()})
	tracker.AppendOutput("agent-log", outputEntry{Type: "tool_result", Tool: "read", Content: "done", Time: time.Now()})
	tracker.AppendOutput("agent-log", outputEntry{Type: "message_update", Content: "streaming text", Time: time.Now()})

	drawer := newAgentPanelDrawer("agent-log", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "⚙")
	assert.Contains(t, rendered, "read")
	assert.Contains(t, rendered, "main.go")
	assert.Contains(t, rendered, "✓")
	assert.Contains(t, rendered, "→")
	assert.Contains(t, rendered, "streaming text")
}

func TestAgentPanelDrawer_Draw_EmptyRingBuffer(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-empty", "test", "background")

	drawer := newAgentPanelDrawer("agent-empty", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	assert.NotPanics(t, func() {
		drawer.Draw(canvas, area)
	})

	// Header and separator should still be there
	rendered := canvas.Render()
	assert.Contains(t, rendered, "test")
}

func TestAgentPanelDrawer_Draw_NilRingBuffer(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)

	drawer := newAgentPanelDrawer("agent-nil", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	assert.NotPanics(t, func() {
		drawer.Draw(canvas, area)
	})
}

func TestAgentPanelDrawer_Draw_ScrollOffset(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-scroll", "test", "background")

	// Add 20 entries to a small visible area
	for range 20 {
		tracker.AppendOutput("agent-scroll", outputEntry{
			Type:    "tool_call",
			Tool:    "read",
			Content: strings.Repeat("a", 20),
			Time:    time.Now(),
		})
	}

	drawer := newAgentPanelDrawer("agent-scroll", tracker, testTheme(), nil)
	drawer.scrollOffset = 15

	canvas := uv.NewScreenBuffer(80, 8) // small area, only ~4 lines for output
	area := uv.Rect(0, 0, 80, 8)

	drawer.Draw(canvas, area)

	// Should not panic and scroll offset should be clamped
	assert.LessOrEqual(t, drawer.scrollOffset, 20)
}

func TestAgentPanelDrawer_Draw_CompletedAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	agent := tracker.Start("agent-2", "planner", "background")
	agent.SpawnedAt = time.Now().Add(-5 * time.Second)

	tracker.Done("agent-2", "completed", "Task completed successfully")

	drawer := newAgentPanelDrawer("agent-2", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "planner")
	assert.Contains(t, rendered, "✓")
}

func TestAgentPanelDrawer_Draw_FailedAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-3", "explorer", "background")
	tracker.Done("agent-3", "failed", "Error: timeout")

	drawer := newAgentPanelDrawer("agent-3", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "explorer")
	assert.Contains(t, rendered, "✗")
}

func TestAgentPanelDrawer_Draw_CancelledAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-cancel", "researcher", "background")
	tracker.Done("agent-cancel", "canceled", "context canceled")

	drawer := newAgentPanelDrawer("agent-cancel", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "researcher")
	assert.Contains(t, rendered, "⊘")
	assert.NotContains(t, rendered, "✕ cancel")
}

func TestAgentPanelDrawer_Draw_AgentRemoved(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-gone", "ghost", "background")
	tracker.Remove("agent-gone")

	drawer := newAgentPanelDrawer("agent-gone", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := strings.TrimSpace(canvas.Render())
	assert.Empty(t, rendered)
}

func TestAgentPanelDrawer_Draw_ZeroSize(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-5", "test", "background")

	drawer := newAgentPanelDrawer("agent-5", tracker, testTheme(), nil)

	canvas := uv.NewScreenBuffer(0, 0)
	area := uv.Rect(0, 0, 0, 0)

	assert.NotPanics(t, func() {
		drawer.Draw(canvas, area)
	})
}

func TestAgentPanelDrawer_Handles_ReturnsTrueForKeyPress(t *testing.T) {
	drawer := newAgentPanelDrawer("x", nil, testTheme(), nil)
	assert.True(t, drawer.Handles(tea.KeyPressMsg{}))
	assert.False(t, drawer.Handles(tea.WindowSizeMsg{}))
}

func TestAgentPanelDrawer_Update_CancelWithCtrlX(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-cancel", "test", "background")

	bus := newMockBus()
	drawer := newAgentPanelDrawer("agent-cancel", tracker, testTheme(), bus)

	_, cmd := drawer.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	assert.Nil(t, cmd)

	require.Len(t, bus.published, 1)
	assert.Equal(t, "subagent.cancel", bus.published[0].Topic)

	payload, ok := bus.published[0].Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "agent-cancel", payload["id"])
}

func TestAgentPanelDrawer_Update_CancelSkipsCompletedAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-done", "test", "background")
	tracker.Done("agent-done", "completed", "done")

	bus := newMockBus()
	drawer := newAgentPanelDrawer("agent-done", tracker, testTheme(), bus)

	_, cmd := drawer.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	assert.Nil(t, cmd)
	assert.Empty(t, bus.published, "cancel should not be published for completed agent")
}

func TestAgentPanelDrawer_Update_CancelWithNilBus(t *testing.T) {
	drawer := newAgentPanelDrawer("agent-nil-bus", nil, testTheme(), nil)

	assert.NotPanics(t, func() {
		drawer.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	})
}

func TestAgentPanelDrawer_Update_ScrollUp(t *testing.T) {
	drawer := newAgentPanelDrawer("agent-scroll", nil, testTheme(), nil)
	drawer.scrollOffset = 5

	_, cmd := drawer.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	assert.Nil(t, cmd)
	assert.Equal(t, 4, drawer.scrollOffset)
}

func TestAgentPanelDrawer_Update_ScrollUpClamped(t *testing.T) {
	drawer := newAgentPanelDrawer("agent-scroll", nil, testTheme(), nil)
	drawer.scrollOffset = 0

	drawer.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	assert.Equal(t, 0, drawer.scrollOffset)
}

func TestAgentPanelDrawer_Update_ScrollDown(t *testing.T) {
	drawer := newAgentPanelDrawer("agent-scroll", nil, testTheme(), nil)
	drawer.scrollOffset = 3

	drawer.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	assert.Equal(t, 4, drawer.scrollOffset)
}

func TestAgentPanelDrawer_Update_IgnoresOtherMessages(t *testing.T) {
	drawer := newAgentPanelDrawer("x", nil, testTheme(), nil)

	newDrawer, cmd := drawer.Update(tea.WindowSizeMsg{})
	assert.Nil(t, cmd)
	assert.Same(t, drawer, newDrawer)
}

func TestAgentPanelDrawer_FormatElapsed_Negative(t *testing.T) {
	theme := testTheme()
	drawer := newAgentPanelDrawer("x", nil, theme, nil)

	agent := &TrackedAgent{
		Status:    AgentCompleted,
		SpawnedAt: time.Now(),
		DoneAt:    time.Now().Add(-5 * time.Second),
	}
	elapsed := drawer.formatElapsed(agent)
	assert.Equal(t, "0s", elapsed)
}

func TestAgentPanelDrawer_StatusIndicator(t *testing.T) {
	theme := testTheme()
	drawer := newAgentPanelDrawer("x", nil, theme, nil)

	icon, color := drawer.statusIndicator(AgentRunning)
	assert.Equal(t, "●", icon)
	assert.Equal(t, theme.Accent, color)

	icon, color = drawer.statusIndicator(AgentCompleted)
	assert.Equal(t, "✓", icon)
	assert.Equal(t, theme.Success, color)

	icon, color = drawer.statusIndicator(AgentFailed)
	assert.Equal(t, "✗", icon)
	assert.Equal(t, theme.Error, color)

	icon, color = drawer.statusIndicator(AgentCancelled)
	assert.Equal(t, "⊘", icon)
	assert.Equal(t, theme.Warning, color)

	icon, color = drawer.statusIndicator(AgentStatus(99))
	assert.Equal(t, "●", icon)
	assert.Equal(t, theme.Muted, color)
}

func TestAgentPanelDrawer_FormatElapsed(t *testing.T) {
	theme := testTheme()
	drawer := newAgentPanelDrawer("x", nil, theme, nil)

	agent := &TrackedAgent{
		Status:    AgentRunning,
		SpawnedAt: time.Now().Add(-30 * time.Second),
	}
	elapsed := drawer.formatElapsed(agent)
	assert.Contains(t, elapsed, "30s")

	agent = &TrackedAgent{
		Status:    AgentCompleted,
		SpawnedAt: time.Now().Add(-125 * time.Second),
		DoneAt:    time.Now().Add(-60 * time.Second),
	}
	elapsed = drawer.formatElapsed(agent)
	assert.Contains(t, elapsed, "1m")
}

func TestAgentPanelDrawer_Integration_WithTracker(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	theme := testTheme()

	tracker.Start("int-agent", "researcher", "background")

	drawer := newAgentPanelDrawer("int-agent", tracker, theme, nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)
	rendered := canvas.Render()
	require.Contains(t, rendered, "researcher")
	require.Contains(t, rendered, "●")

	tracker.Done("int-agent", "completed", "Found 3 files")

	canvas = uv.NewScreenBuffer(80, 18)
	drawer.Draw(canvas, area)
	rendered = canvas.Render()
	require.Contains(t, rendered, "✓")
}

func TestAgentPanelDrawer_Draw_MessageEndSkipped(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-msgend", "test", "background")
	tracker.AppendOutput("agent-msgend", outputEntry{Type: "tool_call", Tool: "read", Content: "file.go", Time: time.Now()})
	tracker.AppendOutput("agent-msgend", outputEntry{Type: "message_end", Content: "", Time: time.Now()})
	tracker.AppendOutput("agent-msgend", outputEntry{Type: "tool_call", Tool: "edit", Content: "file.go", Time: time.Now()})

	drawer := newAgentPanelDrawer("agent-msgend", tracker, testTheme(), nil)
	canvas := uv.NewScreenBuffer(80, 18)
	area := uv.Rect(0, 0, 80, 18)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "read")
	assert.Contains(t, rendered, "edit")
}
