package palette

// Theme provides semantic color slots for the TUI.
// All colors are ANSI 256-color codes (or ANSI names like "red").
type Theme struct {
	// Base grayscale
	Foreground       string // primary text
	ForegroundDim    string // secondary text
	ForegroundBright string // bright/emphasized text
	Muted            string // hints, borders, disabled
	MutedBright      string // hover states, active but unfocused
	Background       string // pure black background
	BackgroundTint   string // panels, pills, subtle surfaces
	BackgroundTint2  string // elevated surfaces, selected items

	// Structural colors
	Border        string // unfocused borders
	BorderFocused string // focused borders (idle state)
	Success       string // muted green for success text
	Error         string // muted red for error text
	Warning       string // amber for warnings

	// State-specific backgrounds
	BackgroundTintPending string
	BackgroundTintSuccess string
	BackgroundTintError   string

	// Dynamic accent (changes with agent state)
	Accent       string // main accent
	AccentDim    string // subdued accent
	AccentBright string // bright accent
}

var defaultTheme = &Theme{
	Foreground:            "250",
	ForegroundDim:         "245",
	ForegroundBright:      "15",
	Muted:                 "240",
	MutedBright:           "248",
	Background:            "16",
	BackgroundTint:        "234",
	BackgroundTint2:       "236",
	Border:                "240",
	BorderFocused:         "248",
	Success:               "114",
	Error:                 "167",
	Warning:               "172",
	BackgroundTintPending: "235",
	BackgroundTintSuccess: "22",
	BackgroundTintError:   "52",
	Accent:                "245",
	AccentDim:             "243",
	AccentBright:          "248",
}

// DefaultTheme returns the built-in dark theme with a monochrome grayscale base.
// Each call returns an independent copy to prevent accidental mutation of shared state.
func DefaultTheme() *Theme {
	t := *defaultTheme
	return &t
}
