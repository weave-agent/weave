package tui

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/ultraviolet/layout"
)

const footerRows = 2

// Layout holds the computed rectangle regions for each TUI section.
//
//	┌─────────────────────────────────┐
//	│  Header (0-2 rows)              │  hints, landing header
//	├─────────────────────────────────┤
//	│                                 │
//	│  Main (flex)                    │  chat viewport or landing
//	│                                 │
//	├─────────────────────────────────┤
//	│  Pills (0-1 rows)               │  spinner, status, tool progress
//	├─────────────────────────────────┤
//	│  PanelTray (0-1 rows)           │  tab strip for visible panels
//	├─────────────────────────────────┤
//	│  AbovePanel (0-N rows)          │  active panel above editor
//	├─────────────────────────────────┤
//	│  Docked (0-N rows)              │  docked overlay dialog
//	├─────────────────────────────────┤
//	│  Editor (3-15 rows, dynamic)    │  textarea with border
//	├─────────────────────────────────┤
//	│  BelowPanel (0-N rows)          │  active panel below editor
//	├─────────────────────────────────┤
//	│  Footer (2 rows)                │  status bar + token rate
//	└─────────────────────────────────┘
type Layout struct {
	Header     uv.Rectangle
	Main       uv.Rectangle
	PanelTray  uv.Rectangle
	AbovePanel uv.Rectangle
	Docked     uv.Rectangle
	Pills      uv.Rectangle
	Editor     uv.Rectangle
	BelowPanel uv.Rectangle
	Footer     uv.Rectangle
}

// LayoutEngine computes Layout regions from terminal dimensions using
// ultraviolet's constraint-based layout solver.
type LayoutEngine struct{}

// NewLayoutEngine creates a LayoutEngine.
func NewLayoutEngine() LayoutEngine {
	return LayoutEngine{}
}

// Compute calculates regions for the minimum layout: main + editor + footer.
// Header, pills, and panels are hidden (0 rows).
func (e LayoutEngine) Compute(width, height, editorLines int) Layout {
	return e.ComputeFull(width, height, editorLines, 0, 0, 0)
}

// ComputeFull calculates regions with optional header, pill, and docked rows.
//
// editorLines is the desired content lines (borders add 2).
// headerRows is 0-2 rows for the header section.
// pillRows is 0-1 rows for the pills section.
// dockedRows is 0-N rows for a docked overlay dialog.
func (e LayoutEngine) ComputeFull(width, height, editorLines, headerRows, pillRows, dockedRows int) Layout {
	return e.ComputeWithPanels(width, height, editorLines, headerRows, pillRows, dockedRows, 0, 0, 0)
}

// ComputeWithPanels calculates regions with optional panel areas.
//
// trayRows is 0-1 rows for the panel tab strip.
// abovePanelRows is 0-N rows for a panel above the editor.
// belowPanelRows is 0-N rows for a panel below the editor (above footer).
func (e LayoutEngine) ComputeWithPanels(width, height, editorLines, headerRows, pillRows, dockedRows, trayRows, abovePanelRows, belowPanelRows int) Layout {
	if width <= 0 || height <= 0 {
		return Layout{}
	}

	editorRows := editorLines + 2 // content + top/bottom border
	mainRows := height - headerRows - trayRows - abovePanelRows - pillRows - dockedRows - editorRows - belowPanelRows - footerRows

	if mainRows < 1 {
		return minimalLayout(width, height)
	}

	var (
		constraints []layout.Constraint
		targets     []*uv.Rectangle
	)

	var header, main, tray, abovePanel, docked, pills, editor, belowPanel, footer uv.Rectangle

	if headerRows > 0 {
		constraints = append(constraints, layout.Len(headerRows))
		targets = append(targets, &header)
	}

	constraints = append(constraints, layout.Fill(1))
	targets = append(targets, &main)

	if pillRows > 0 {
		constraints = append(constraints, layout.Len(pillRows))
		targets = append(targets, &pills)
	}

	if trayRows > 0 {
		constraints = append(constraints, layout.Len(trayRows))
		targets = append(targets, &tray)
	}

	if abovePanelRows > 0 {
		constraints = append(constraints, layout.Len(abovePanelRows))
		targets = append(targets, &abovePanel)
	}

	if dockedRows > 0 {
		constraints = append(constraints, layout.Len(dockedRows))
		targets = append(targets, &docked)
	}

	constraints = append(constraints, layout.Len(editorRows))
	targets = append(targets, &editor)

	if belowPanelRows > 0 {
		constraints = append(constraints, layout.Len(belowPanelRows))
		targets = append(targets, &belowPanel)
	}

	constraints = append(constraints, layout.Len(footerRows))
	targets = append(targets, &footer)

	area := uv.Rect(0, 0, width, height)
	layout.Vertical(constraints...).Split(area).Assign(targets...)

	return Layout{
		Header:     header,
		Main:       main,
		PanelTray:  tray,
		AbovePanel: abovePanel,
		Docked:     docked,
		Pills:      pills,
		Editor:     editor,
		BelowPanel: belowPanel,
		Footer:     footer,
	}
}

// minimalLayout returns a fallback for very small terminals.
// Provides main (1 row), editor (3+2 rows), footer (2 rows).
func minimalLayout(width, height int) Layout {
	if height < 4 {
		return Layout{Main: uv.Rect(0, 0, width, max(height, 1))}
	}

	mainH := 1
	editorH := min(height-mainH-footerRows, 5)
	footerH := height - mainH - editorH

	main := uv.Rect(0, 0, width, mainH)
	editor := uv.Rect(0, main.Max.Y, width, editorH)
	footer := uv.Rect(0, editor.Max.Y, width, footerH)

	return Layout{Main: main, Editor: editor, Footer: footer}
}
