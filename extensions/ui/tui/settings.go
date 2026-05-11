package tui

// Settings holds TUI-specific preferences.
type Settings struct {
	Theme          string `json:"theme,omitempty"`
	EditorMaxLines int    `json:"editor_max_lines,omitempty"`
}
