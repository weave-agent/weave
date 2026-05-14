package palette

// Theme provides semantic color slots for the TUI.
// All colors are ANSI 256-color codes (or ANSI names like "red").
type Theme struct {
	Primary               string // main accent
	PrimaryDim            string // subdued accent
	PrimaryBright         string // bright accent
	Success               string // positive/success
	Error                 string // error/negative
	Warning               string // caution/warning
	Muted                 string // dimmed text
	MutedBright           string // lighter dimmed text
	Border                string // unfocused borders
	BorderFocused         string // focused borders
	BackgroundTint        string // subtle panel backgrounds
	BackgroundTintPending string // pending state background
	BackgroundTintSuccess string // success state background
	BackgroundTintError   string // error state background
	Foreground            string // main text
	ForegroundBright      string // bright text
}

var defaultTheme = &Theme{
	Primary:               "63",
	PrimaryDim:            "60",
	PrimaryBright:         "69",
	Success:               "84",
	Error:                 "204",
	Warning:               "221",
	Muted:                 "245",
	MutedBright:           "252",
	Border:                "240",
	BorderFocused:         "63",
	BackgroundTint:        "234",
	BackgroundTintPending: "235",
	BackgroundTintSuccess: "22",
	BackgroundTintError:   "52",
	Foreground:            "15",
	ForegroundBright:      "15",
}

// DefaultTheme returns the built-in dark theme with a purple-blue centered palette.
// Each call returns an independent copy to prevent accidental mutation of shared state.
func DefaultTheme() *Theme {
	t := *defaultTheme
	return &t
}
