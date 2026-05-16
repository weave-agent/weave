package subagent

import (
	"strings"
	"testing"
	"time"

	"weave/sdk"

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
	}
}

func TestAgentPanelDrawer_Draw_RunningAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	agent := tracker.Start("agent-1", "researcher", "background")

	drawer := newAgentPanelDrawer(agent.ID, tracker, testTheme())
	canvas := uv.NewScreenBuffer(80, 6)
	area := uv.Rect(0, 0, 80, 6)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "researcher")
	assert.Contains(t, rendered, "background")
	assert.Contains(t, rendered, "●")
	// Elapsed time should be a small number of seconds (pattern match, not exact).
	assert.Regexp(t, `\d+s`, rendered)
}

func TestAgentPanelDrawer_Draw_CompletedAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	agent := tracker.Start("agent-2", "planner", "background")
	agent.SpawnedAt = time.Now().Add(-5 * time.Second)

	tracker.Done("agent-2", "completed", "Task completed successfully")

	// Re-fetch to get updated state
	drawer := newAgentPanelDrawer("agent-2", tracker, testTheme())
	canvas := uv.NewScreenBuffer(80, 6)
	area := uv.Rect(0, 0, 80, 6)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "planner")
	assert.Contains(t, rendered, "✓")
	assert.Contains(t, rendered, "Task completed successfully")
}

func TestAgentPanelDrawer_Draw_FailedAgent(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-3", "explorer", "background")
	tracker.Done("agent-3", "failed", "Error: timeout")

	drawer := newAgentPanelDrawer("agent-3", tracker, testTheme())
	canvas := uv.NewScreenBuffer(80, 6)
	area := uv.Rect(0, 0, 80, 6)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "explorer")
	assert.Contains(t, rendered, "✗")
	assert.Contains(t, rendered, "Error: timeout")
}

func TestAgentPanelDrawer_Draw_NoResultWhenRunning(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-4", "coder", "background")

	drawer := newAgentPanelDrawer("agent-4", tracker, testTheme())
	canvas := uv.NewScreenBuffer(80, 6)
	area := uv.Rect(0, 0, 80, 6)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "coder")
	assert.NotContains(t, rendered, "result")
}

func TestAgentPanelDrawer_Draw_AgentRemoved(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-gone", "ghost", "background")
	tracker.Remove("agent-gone")

	drawer := newAgentPanelDrawer("agent-gone", tracker, testTheme())
	canvas := uv.NewScreenBuffer(80, 6)
	area := uv.Rect(0, 0, 80, 6)

	// Should not panic when agent is gone
	drawer.Draw(canvas, area)

	rendered := strings.TrimSpace(canvas.Render())
	assert.Empty(t, rendered)
}

func TestAgentPanelDrawer_Draw_ZeroSize(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	tracker.Start("agent-5", "test", "background")

	drawer := newAgentPanelDrawer("agent-5", tracker, testTheme())

	// Zero-area should not panic
	canvas := uv.NewScreenBuffer(0, 0)
	area := uv.Rect(0, 0, 0, 0)

	assert.NotPanics(t, func() {
		drawer.Draw(canvas, area)
	})
}

func TestAgentPanelDrawer_Draw_ResultTruncated(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	longResult := strings.Repeat("abcdefghij", 50) // 500 chars

	tracker.Start("agent-6", "writer", "background")
	tracker.Done("agent-6", "completed", longResult)

	drawer := newAgentPanelDrawer("agent-6", tracker, testTheme())
	canvas := uv.NewScreenBuffer(30, 6)
	area := uv.Rect(0, 0, 30, 6)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	// Result should be truncated (not the full 500 chars)
	assert.Contains(t, rendered, "...")
}

func TestAgentPanelDrawer_Draw_MultilineResult(t *testing.T) {
	tracker := NewAgentTracker(gracePeriod, nil)
	result := "line 1\nline 2\nline 3\nline 4\nline 5"

	tracker.Start("agent-7", "multi", "background")
	tracker.Done("agent-7", "completed", result)

	drawer := newAgentPanelDrawer("agent-7", tracker, testTheme())
	canvas := uv.NewScreenBuffer(80, 6)
	area := uv.Rect(0, 0, 80, 6)

	drawer.Draw(canvas, area)

	rendered := canvas.Render()
	assert.Contains(t, rendered, "line 1")
	assert.Contains(t, rendered, "line 2")
	assert.Contains(t, rendered, "line 3")
	assert.Contains(t, rendered, "line 5")
}

func TestAgentPanelDrawer_Handles_AlwaysFalse(t *testing.T) {
	drawer := newAgentPanelDrawer("x", nil, testTheme())
	assert.False(t, drawer.Handles(tea.KeyPressMsg{}))
	assert.False(t, drawer.Handles(tea.WindowSizeMsg{}))
}

func TestAgentPanelDrawer_Update_ReturnsSelf(t *testing.T) {
	drawer := newAgentPanelDrawer("x", nil, testTheme())
	newDrawer, cmd := drawer.Update(tea.KeyPressMsg{})
	assert.Nil(t, cmd)
	assert.Same(t, drawer, newDrawer)
}

func TestAgentPanelDrawer_FormatElapsed_Negative(t *testing.T) {
	theme := testTheme()
	drawer := newAgentPanelDrawer("x", nil, theme)

	// Clock skew: DoneAt before SpawnedAt should be clamped to 0.
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
	drawer := newAgentPanelDrawer("x", nil, theme)

	icon, color := drawer.statusIndicator(AgentRunning)
	assert.Equal(t, "●", icon)
	assert.Equal(t, theme.Accent, color)

	icon, color = drawer.statusIndicator(AgentCompleted)
	assert.Equal(t, "✓", icon)
	assert.Equal(t, theme.Success, color)

	icon, color = drawer.statusIndicator(AgentFailed)
	assert.Equal(t, "✗", icon)
	assert.Equal(t, theme.Error, color)

	// Unknown status should default to muted dot.
	icon, color = drawer.statusIndicator(AgentStatus(99))
	assert.Equal(t, "●", icon)
	assert.Equal(t, theme.Muted, color)
}

func TestAgentPanelDrawer_FormatElapsed(t *testing.T) {
	theme := testTheme()
	drawer := newAgentPanelDrawer("x", nil, theme)

	// Running agent: elapsed since spawned
	agent := &TrackedAgent{
		Status:    AgentRunning,
		SpawnedAt: time.Now().Add(-30 * time.Second),
	}
	elapsed := drawer.formatElapsed(agent)
	assert.Contains(t, elapsed, "30s")

	// Completed agent: time between spawn and done
	agent = &TrackedAgent{
		Status:    AgentCompleted,
		SpawnedAt: time.Now().Add(-125 * time.Second),
		DoneAt:    time.Now().Add(-60 * time.Second),
	}
	elapsed = drawer.formatElapsed(agent)
	assert.Contains(t, elapsed, "1m")
}

func TestAgentPanelDrawer_FormatResult_Empty(t *testing.T) {
	theme := testTheme()
	drawer := newAgentPanelDrawer("x", nil, theme)

	result := drawer.formatResult("", 80, 3)
	assert.Empty(t, result)
}

func TestAgentPanelDrawer_FormatResult_SmallMaxWidth(t *testing.T) {
	theme := testTheme()
	drawer := newAgentPanelDrawer("x", nil, theme)

	// maxWidth=5 should be clamped to 10 runes minimum.
	longLine := strings.Repeat("a", 50)
	result := drawer.formatResult(longLine, 5, 3)
	assert.Contains(t, result, "...")
}

func TestAgentPanelDrawer_FormatResult(t *testing.T) {
	theme := testTheme()
	drawer := newAgentPanelDrawer("x", nil, theme)

	// Short result
	result := drawer.formatResult("hello", 80, 3)
	assert.Equal(t, "hello", result)

	// Long result truncated
	longLine := strings.Repeat("a", 200)
	result = drawer.formatResult(longLine, 40, 3)
	assert.Contains(t, result, "...")
	assert.Less(t, len(result), 200)

	// Multi-line result limited by available height
	multi := "line1\nline2\nline3\nline4\nline5"
	result = drawer.formatResult(multi, 80, 4)
	lines := strings.Split(result, "\n")
	assert.Len(t, lines, 4)

	result = drawer.formatResult(multi, 80, 0)
	assert.Empty(t, result)
}

func TestAgentPanelDrawer_Integration_WithTracker(t *testing.T) {
	// Test that the drawer works with a real tracker through its lifecycle
	tracker := NewAgentTracker(gracePeriod, nil)
	theme := testTheme()

	// Start
	tracker.Start("int-agent", "researcher", "background")

	drawer := newAgentPanelDrawer("int-agent", tracker, theme)
	canvas := uv.NewScreenBuffer(80, 6)
	area := uv.Rect(0, 0, 80, 6)

	// Draw while running
	drawer.Draw(canvas, area)
	rendered := canvas.Render()
	require.Contains(t, rendered, "researcher")
	require.Contains(t, rendered, "●")

	// Complete
	tracker.Done("int-agent", "completed", "Found 3 files")

	// Redraw after completion
	canvas = uv.NewScreenBuffer(80, 6)
	drawer.Draw(canvas, area)
	rendered = canvas.Render()
	require.Contains(t, rendered, "✓")
	require.Contains(t, rendered, "Found 3 files")
}
