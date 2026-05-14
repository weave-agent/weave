package tui

// FocusTarget tracks which UI element has keyboard focus.
type FocusTarget int

const (
	FocusEditor FocusTarget = iota
	FocusTray
	FocusPanel
)
