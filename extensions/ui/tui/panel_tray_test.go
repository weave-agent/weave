package tui

import (
	"strings"
	"testing"

	"github.com/weave-agent/weave-tui/palette"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
)

func TestPanelTray_New(t *testing.T) {
	pt := NewPanelTray()
	assert.Equal(t, -1, pt.activeIdx)
	assert.Empty(t, pt.tabs)
	assert.Empty(t, pt.ActiveID())
	assert.Equal(t, 0, pt.Len())
	assert.False(t, pt.IsFocused())
}

func TestPanelTray_SetTabs(t *testing.T) {
	pt := NewPanelTray()
	tabs := []PanelTab{
		{ID: "p1", Title: "Panel 1"},
		{ID: "p2", Title: "Panel 2"},
	}
	pt = pt.SetTabs(tabs, 0)

	assert.Equal(t, 2, pt.Len())
	assert.Equal(t, "p1", pt.ActiveID())
	assert.Equal(t, 0, pt.activeIdx)
}

func TestPanelTray_SetTabs_ActiveIdx(t *testing.T) {
	pt := NewPanelTray()
	tabs := []PanelTab{
		{ID: "p1", Title: "Panel 1"},
		{ID: "p2", Title: "Panel 2"},
	}
	pt = pt.SetTabs(tabs, 1)

	assert.Equal(t, "p2", pt.ActiveID())
	assert.Equal(t, 1, pt.activeIdx)
}

func TestPanelTray_Next(t *testing.T) {
	pt := NewPanelTray()
	pt = pt.SetTabs([]PanelTab{
		{ID: "p1", Title: "A"},
		{ID: "p2", Title: "B"},
		{ID: "p3", Title: "C"},
	}, 0)

	pt = pt.Next()
	assert.Equal(t, 1, pt.activeIdx)
	assert.Equal(t, "p2", pt.ActiveID())

	pt = pt.Next()
	assert.Equal(t, 2, pt.activeIdx)
	assert.Equal(t, "p3", pt.ActiveID())

	pt = pt.Next()
	assert.Equal(t, 0, pt.activeIdx)
	assert.Equal(t, "p1", pt.ActiveID())
}

func TestPanelTray_Next_Empty(t *testing.T) {
	pt := NewPanelTray()
	pt = pt.Next()
	assert.Equal(t, -1, pt.activeIdx)
}

func TestPanelTray_Prev(t *testing.T) {
	pt := NewPanelTray()
	pt = pt.SetTabs([]PanelTab{
		{ID: "p1", Title: "A"},
		{ID: "p2", Title: "B"},
		{ID: "p3", Title: "C"},
	}, 0)

	pt = pt.Prev()
	assert.Equal(t, 2, pt.activeIdx)
	assert.Equal(t, "p3", pt.ActiveID())

	pt = pt.Prev()
	assert.Equal(t, 1, pt.activeIdx)
	assert.Equal(t, "p2", pt.ActiveID())
}

func TestPanelTray_Prev_Empty(t *testing.T) {
	pt := NewPanelTray()
	pt = pt.Prev()
	assert.Equal(t, -1, pt.activeIdx)
}

func TestPanelTray_SetSize(t *testing.T) {
	pt := NewPanelTray()
	pt = pt.SetSize(120)
	assert.Equal(t, 120, pt.width)
}

func TestPanelTray_SetFocused(t *testing.T) {
	pt := NewPanelTray()
	assert.False(t, pt.IsFocused())

	pt = pt.SetFocused(true)
	assert.True(t, pt.IsFocused())

	pt = pt.SetFocused(false)
	assert.False(t, pt.IsFocused())
}

func TestPanelTray_ActiveID_OutOfRange(t *testing.T) {
	pt := NewPanelTray()
	assert.Empty(t, pt.ActiveID())

	pt = pt.SetTabs([]PanelTab{{ID: "p1"}}, 5)
	assert.Empty(t, pt.ActiveID())
}

func TestPanelTray_Draw_Empty(t *testing.T) {
	pt := NewPanelTray()
	canvas := uv.NewScreenBuffer(80, 24)
	area := uv.Rect(0, 0, 80, 1)

	// Should not panic
	pt.Draw(canvas, area, palette.DefaultTheme())
}

func TestPanelTray_Draw_WithTabs(t *testing.T) {
	pt := NewPanelTray()
	pt = pt.SetTabs([]PanelTab{
		{ID: "p1", Title: "Files"},
		{ID: "p2", Title: "Git"},
	}, 0)

	canvas := uv.NewScreenBuffer(80, 24)
	area := uv.Rect(0, 0, 80, 1)
	pt.Draw(canvas, area, palette.DefaultTheme())

	rendered := canvas.Render()
	assert.Contains(t, rendered, "Files")
	assert.Contains(t, rendered, "Git")
	assert.True(t, strings.HasPrefix(rendered, " "))
}

func TestPanelTray_Draw_ClearsFullRow(t *testing.T) {
	pt := NewPanelTray()
	pt = pt.SetTabs([]PanelTab{{ID: "p1", Title: "Files"}}, 0)

	canvas := uv.NewScreenBuffer(20, 1)
	area := uv.Rect(0, 0, 20, 1)
	uv.NewStyledString(strings.Repeat("x", 20)).Draw(canvas, area)

	pt.Draw(canvas, area, palette.DefaultTheme())

	rendered := canvas.Render()
	assert.Contains(t, rendered, "Files")
	assert.NotContains(t, rendered, "xxxxx")
}

func TestPanelTray_Draw_ActiveTabHighlighted(t *testing.T) {
	pt := NewPanelTray()
	pt = pt.SetTabs([]PanelTab{
		{ID: "p1", Title: "Files"},
		{ID: "p2", Title: "Git"},
	}, 0)
	pt = pt.SetFocused(true)

	canvas := uv.NewScreenBuffer(80, 24)
	area := uv.Rect(0, 0, 80, 1)
	pt.Draw(canvas, area, palette.DefaultTheme())

	rendered := canvas.Render()
	assert.Contains(t, rendered, "Files")
}

func TestPanelTray_Draw_LongLineTruncated(t *testing.T) {
	pt := NewPanelTray()
	// Very long titles that exceed area width
	pt = pt.SetTabs([]PanelTab{
		{ID: "p1", Title: "VeryLongPanelNameThatExceedsWidth"},
	}, 0)

	canvas := uv.NewScreenBuffer(10, 24)
	area := uv.Rect(0, 0, 10, 1)
	pt.Draw(canvas, area, palette.DefaultTheme())

	// Should not panic even with truncation
	rendered := canvas.Render()
	assert.NotEmpty(t, rendered)
}

func TestPanelTray_Draw_NoTitleFallsBackToID(t *testing.T) {
	pt := NewPanelTray()
	pt = pt.SetTabs([]PanelTab{
		{ID: "my-panel", Title: ""},
	}, 0)

	canvas := uv.NewScreenBuffer(80, 24)
	area := uv.Rect(0, 0, 80, 1)
	pt.Draw(canvas, area, palette.DefaultTheme())

	rendered := canvas.Render()
	assert.Contains(t, rendered, "my-panel")
}
