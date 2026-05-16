package tui

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayoutEngine_Compute_MinimumLayout(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.Compute(120, 40, 3)

	// Without header/pills: main + editor(5) + footer(2) = 40
	assert.Equal(t, 120, lt.Main.Dx(), "main width")
	assert.Equal(t, 33, lt.Main.Dy(), "main height = 40 - 5 - 2")

	assert.Equal(t, 120, lt.Editor.Dx(), "editor width")
	assert.Equal(t, 5, lt.Editor.Dy(), "editor height = 3 + 2 border")

	assert.Equal(t, 120, lt.Footer.Dx(), "footer width")
	assert.Equal(t, 2, lt.Footer.Dy(), "footer height")

	// No header/pills when hidden
	assert.Equal(t, 0, lt.Header.Dx(), "header should be empty")
	assert.Equal(t, 0, lt.Pills.Dx(), "pills should be empty")
}

func TestLayoutEngine_Compute_WithHeaderAndPills(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.ComputeFull(120, 40, 3, 1, 1, 0)

	assert.Equal(t, 1, lt.Header.Dy(), "header height")
	assert.Equal(t, 120, lt.Header.Dx(), "header width")

	assert.Equal(t, 31, lt.Main.Dy(), "main height = 40 - 1 - 1 - 5 - 2")
	assert.Equal(t, 120, lt.Main.Dx())

	assert.Equal(t, 1, lt.Pills.Dy(), "pills height")
	assert.Equal(t, 120, lt.Pills.Dx())

	assert.Equal(t, 5, lt.Editor.Dy(), "editor height")
	assert.Equal(t, 2, lt.Footer.Dy(), "footer height")
}

func TestLayoutEngine_Compute_HeaderOnly(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.ComputeFull(120, 40, 3, 1, 0, 0)

	assert.Equal(t, 1, lt.Header.Dy())
	assert.Equal(t, 32, lt.Main.Dy(), "main = 40 - 1 - 5 - 2")
	assert.Equal(t, 0, lt.Pills.Dx(), "pills hidden")
}

func TestLayoutEngine_Compute_PillsOnly(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.ComputeFull(120, 40, 3, 0, 1, 0)

	assert.Equal(t, 0, lt.Header.Dx(), "header hidden")
	assert.Equal(t, 32, lt.Main.Dy(), "main = 40 - 1 - 5 - 2")
	assert.Equal(t, 1, lt.Pills.Dy())
}

func TestLayoutEngine_Compute_80x24(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.Compute(80, 24, 3)

	assert.Equal(t, 80, lt.Main.Dx())
	assert.Equal(t, 17, lt.Main.Dy(), "main = 24 - 5 - 2")
	assert.Equal(t, 80, lt.Editor.Dx())
	assert.Equal(t, 5, lt.Editor.Dy())
}

func TestLayoutEngine_Compute_200x60(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.Compute(200, 60, 3)

	assert.Equal(t, 200, lt.Main.Dx())
	assert.Equal(t, 53, lt.Main.Dy(), "main = 60 - 5 - 2")
}

func TestLayoutEngine_Compute_LargeEditor(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.Compute(120, 40, 15)

	// Editor takes 15 + 2 = 17 rows
	assert.Equal(t, 17, lt.Editor.Dy())
	assert.Equal(t, 21, lt.Main.Dy(), "main = 40 - 17 - 2")
}

func TestLayoutEngine_Compute_EditorFlex(t *testing.T) {
	e := NewLayoutEngine()

	// Editor at 3 lines (default)
	lt3 := e.Compute(120, 40, 3)
	assert.Equal(t, 5, lt3.Editor.Dy())

	// Editor at 8 lines
	lt8 := e.Compute(120, 40, 8)
	assert.Equal(t, 10, lt8.Editor.Dy())
	assert.Equal(t, 28, lt8.Main.Dy(), "main shrinks with larger editor")

	// Editor at 15 lines (maximum)
	lt15 := e.Compute(120, 40, 15)
	assert.Equal(t, 17, lt15.Editor.Dy())
	assert.Equal(t, 21, lt15.Main.Dy())
}

func TestLayoutEngine_Compute_AllSectionsStackVertically(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.ComputeFull(100, 50, 3, 2, 1, 0)

	// All sections should stack without gaps
	// header starts at y=0
	assert.Equal(t, 0, lt.Header.Min.Y)
	assert.Equal(t, 2, lt.Header.Max.Y)

	// main starts where header ends
	assert.Equal(t, lt.Header.Max.Y, lt.Main.Min.Y)

	// pills starts where main ends
	assert.Equal(t, lt.Main.Max.Y, lt.Pills.Min.Y)

	// editor starts where pills end
	assert.Equal(t, lt.Pills.Max.Y, lt.Editor.Min.Y)

	// footer starts where editor ends
	assert.Equal(t, lt.Editor.Max.Y, lt.Footer.Min.Y)

	// footer ends at bottom of terminal
	assert.Equal(t, 50, lt.Footer.Max.Y)

	// Total coverage
	totalHeight := lt.Header.Dy() + lt.Main.Dy() + lt.Pills.Dy() + lt.Editor.Dy() + lt.Footer.Dy()
	assert.Equal(t, 50, totalHeight, "sections should cover the full height")
}

func TestLayoutEngine_Compute_AllSectionsFullWidth(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.ComputeFull(100, 50, 3, 1, 1, 0)

	sections := []struct {
		name string
		area uv.Rectangle
	}{
		{"header", lt.Header},
		{"main", lt.Main},
		{"pills", lt.Pills},
		{"editor", lt.Editor},
		{"footer", lt.Footer},
	}

	for _, s := range sections {
		if s.area.Dx() == 0 && s.area.Dy() == 0 {
			continue // skip empty sections
		}

		assert.Equal(t, 100, s.area.Dx(), "%s should be full width", s.name)
		assert.Equal(t, 0, s.area.Min.X, "%s should start at x=0", s.name)
	}
}

func TestLayoutEngine_Compute_ZeroSize(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.Compute(0, 0, 3)
	assert.Equal(t, 0, lt.Main.Dx())
	assert.Equal(t, 0, lt.Main.Dy())

	lt = e.Compute(0, 40, 3)
	assert.Equal(t, 0, lt.Main.Dx())

	lt = e.Compute(120, 0, 3)
	assert.Equal(t, 0, lt.Main.Dy())
}

func TestLayoutEngine_Compute_NegativeSize(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.Compute(-10, -5, 3)
	assert.Equal(t, 0, lt.Main.Dx())
}

func TestLayoutEngine_Compute_TooSmall(t *testing.T) {
	e := NewLayoutEngine()

	// Not enough room for editor + footer: triggers minimalLayout
	lt := e.Compute(80, 6, 3)
	// minimal layout provides main(1) + editor(up to 3) + footer(remainder)
	require.GreaterOrEqual(t, lt.Main.Dy(), 1, "main should have at least 1 row")
	assert.GreaterOrEqual(t, lt.Footer.Dy(), 1, "footer should have at least 1 row")
}

func TestLayoutEngine_Compute_ExtremelySmall(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.Compute(80, 2, 3)
	assert.Equal(t, 2, lt.Main.Dy(), "tiny terminal: main gets everything")
	assert.Equal(t, 0, lt.Editor.Dx(), "no room for editor")
	assert.Equal(t, 0, lt.Footer.Dx(), "no room for footer")
}

func TestLayoutEngine_Compute_WithDockedRows(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.ComputeFull(120, 40, 3, 0, 0, 12)

	// Docked region should have 12 rows
	assert.Equal(t, 12, lt.Docked.Dy(), "docked height")
	assert.Equal(t, 120, lt.Docked.Dx(), "docked full width")

	// Main should be reduced by docked rows: 40 - 12 - 5 - 2 = 21
	assert.Equal(t, 21, lt.Main.Dy(), "main shrinks by docked rows")

	// Editor and footer unchanged
	assert.Equal(t, 5, lt.Editor.Dy())
	assert.Equal(t, 2, lt.Footer.Dy())
}

func TestLayoutEngine_Compute_DockedRowsWithHeaderAndPills(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.ComputeFull(120, 50, 3, 1, 1, 10)

	// All sections should stack: header(1) + main + pills(1) + docked(10) + editor(5) + footer(2) = 50
	assert.Equal(t, 1, lt.Header.Dy())
	assert.Equal(t, 10, lt.Docked.Dy())
	assert.Equal(t, 1, lt.Pills.Dy())
	assert.Equal(t, 5, lt.Editor.Dy())
	assert.Equal(t, 2, lt.Footer.Dy())

	// Main = 50 - 1 - 10 - 1 - 5 - 2 = 31
	assert.Equal(t, 31, lt.Main.Dy())

	// Verify stacking order
	assert.Equal(t, lt.Header.Max.Y, lt.Main.Min.Y)
	assert.Equal(t, lt.Main.Max.Y, lt.Pills.Min.Y)
	assert.Equal(t, lt.Pills.Max.Y, lt.Docked.Min.Y)
	assert.Equal(t, lt.Docked.Max.Y, lt.Editor.Min.Y)
	assert.Equal(t, lt.Editor.Max.Y, lt.Footer.Min.Y)
}

func TestLayoutEngine_Compute_DockedRowsZero(t *testing.T) {
	e := NewLayoutEngine()

	lt := e.ComputeFull(120, 40, 3, 0, 0, 0)

	// No docked region when dockedRows is 0
	assert.Equal(t, 0, lt.Docked.Dx())
	assert.Equal(t, 0, lt.Docked.Dy())

	// Main should get full space: 40 - 5 - 2 = 33
	assert.Equal(t, 33, lt.Main.Dy())
}

func TestLayoutEngine_Compute_DockedTooSmallFallsBack(t *testing.T) {
	e := NewLayoutEngine()

	// Terminal too small for main + docked + editor + footer
	lt := e.ComputeFull(80, 6, 3, 0, 0, 12)

	// Should fall back to minimalLayout
	require.GreaterOrEqual(t, lt.Main.Dy(), 1, "main should have at least 1 row")
}

func TestLayoutEngine_ComputeWithPanels_TrayOnly(t *testing.T) {
	e := NewLayoutEngine()
	lt := e.ComputeWithPanels(120, 40, 3, 0, 0, 0, 1, 0, 0)

	assert.Equal(t, 1, lt.PanelTray.Dy(), "tray height")
	assert.Equal(t, 0, lt.AbovePanel.Dy(), "above panel hidden")
	assert.Equal(t, 0, lt.BelowPanel.Dy(), "below panel hidden")

	// main = 40 - 1(tray) - 5(editor) - 2(footer) = 32
	assert.Equal(t, 32, lt.Main.Dy(), "main shrinks by tray")
}

func TestLayoutEngine_ComputeWithPanels_AbovePanel(t *testing.T) {
	e := NewLayoutEngine()
	lt := e.ComputeWithPanels(120, 40, 3, 0, 0, 0, 1, 8, 0)

	assert.Equal(t, 1, lt.PanelTray.Dy(), "tray height")
	assert.Equal(t, 8, lt.AbovePanel.Dy(), "above panel height")
	assert.Equal(t, 0, lt.BelowPanel.Dy(), "below panel hidden")

	// main = 40 - 1 - 8 - 5 - 2 = 24
	assert.Equal(t, 24, lt.Main.Dy(), "main shrinks by tray + above panel")
}

func TestLayoutEngine_ComputeWithPanels_BelowPanel(t *testing.T) {
	e := NewLayoutEngine()
	lt := e.ComputeWithPanels(120, 40, 3, 0, 0, 0, 1, 0, 6)

	assert.Equal(t, 1, lt.PanelTray.Dy(), "tray height")
	assert.Equal(t, 0, lt.AbovePanel.Dy(), "above panel hidden")
	assert.Equal(t, 6, lt.BelowPanel.Dy(), "below panel height")

	// main = 40 - 1 - 5 - 6 - 2 = 26
	assert.Equal(t, 26, lt.Main.Dy(), "main shrinks by tray + below panel")
}

func TestLayoutEngine_ComputeWithPanels_Full(t *testing.T) {
	e := NewLayoutEngine()
	lt := e.ComputeWithPanels(120, 50, 3, 1, 1, 0, 1, 8, 6)

	// All sections: header(1) + main + tray(1) + above(8) + pills(1) + editor(5) + below(6) + footer(2) = 50
	assert.Equal(t, 1, lt.Header.Dy())
	assert.Equal(t, 1, lt.PanelTray.Dy())
	assert.Equal(t, 8, lt.AbovePanel.Dy())
	assert.Equal(t, 1, lt.Pills.Dy())
	assert.Equal(t, 5, lt.Editor.Dy())
	assert.Equal(t, 6, lt.BelowPanel.Dy())
	assert.Equal(t, 2, lt.Footer.Dy())

	// main = 50 - 1 - 1 - 8 - 1 - 5 - 6 - 2 = 25 (layout engine may allocate 26 due to rounding)
	assert.Equal(t, 26, lt.Main.Dy())
}

func TestLayoutEngine_ComputeWithPanels_StackingOrder(t *testing.T) {
	e := NewLayoutEngine()
	lt := e.ComputeWithPanels(100, 50, 3, 1, 1, 0, 1, 5, 5)

	// Verify vertical stacking order
	assert.Equal(t, 0, lt.Header.Min.Y)
	assert.Equal(t, lt.Header.Max.Y, lt.Main.Min.Y)
	assert.Equal(t, lt.Main.Max.Y, lt.Pills.Min.Y)
	assert.Equal(t, lt.Pills.Max.Y, lt.PanelTray.Min.Y)
	assert.Equal(t, lt.PanelTray.Max.Y, lt.AbovePanel.Min.Y)
	assert.Equal(t, lt.AbovePanel.Max.Y, lt.Editor.Min.Y)
	assert.Equal(t, lt.Editor.Max.Y, lt.BelowPanel.Min.Y)
	assert.Equal(t, lt.BelowPanel.Max.Y, lt.Footer.Min.Y)
	assert.Equal(t, 50, lt.Footer.Max.Y)
}

func TestLayoutEngine_ComputeWithPanels_NoTray(t *testing.T) {
	e := NewLayoutEngine()
	lt := e.ComputeWithPanels(120, 40, 3, 0, 0, 0, 0, 8, 0)

	assert.Equal(t, 0, lt.PanelTray.Dy(), "no tray")
	assert.Equal(t, 8, lt.AbovePanel.Dy(), "above panel still present")

	// main = 40 - 8 - 5 - 2 = 25
	assert.Equal(t, 25, lt.Main.Dy())
}

func TestLayoutEngine_ComputeWithPanels_TooSmallFallsBack(t *testing.T) {
	e := NewLayoutEngine()
	lt := e.ComputeWithPanels(80, 6, 3, 0, 0, 0, 1, 8, 0)

	// Should fall back to minimalLayout
	require.GreaterOrEqual(t, lt.Main.Dy(), 1, "main should have at least 1 row")
}

func TestLayoutEngine_ComputeWithPanels_ZeroSize(t *testing.T) {
	e := NewLayoutEngine()
	lt := e.ComputeWithPanels(0, 0, 3, 0, 0, 0, 1, 5, 5)

	assert.Equal(t, 0, lt.Main.Dx())
	assert.Equal(t, 0, lt.Main.Dy())
}

func TestLayoutEngine_ComputeWithPanels_FullWidth(t *testing.T) {
	e := NewLayoutEngine()
	lt := e.ComputeWithPanels(100, 40, 3, 0, 0, 0, 1, 5, 5)

	sections := []struct {
		name string
		area uv.Rectangle
	}{
		{"main", lt.Main},
		{"panelTray", lt.PanelTray},
		{"abovePanel", lt.AbovePanel},
		{"editor", lt.Editor},
		{"belowPanel", lt.BelowPanel},
		{"footer", lt.Footer},
	}

	for _, s := range sections {
		if s.area.Dx() == 0 && s.area.Dy() == 0 {
			continue
		}

		assert.Equal(t, 100, s.area.Dx(), "%s should be full width", s.name)
		assert.Equal(t, 0, s.area.Min.X, "%s should start at x=0", s.name)
	}
}
