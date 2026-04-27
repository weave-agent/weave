package tui

// Composer holds the layout engine for screen buffer rendering.
type Composer struct {
	Engine LayoutEngine
}

// NewComposer creates a Composer with a default LayoutEngine.
func NewComposer() Composer {
	return Composer{Engine: NewLayoutEngine()}
}
