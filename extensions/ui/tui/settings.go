package tui

// TUIConfig holds TUI-specific preferences.
type TUIConfig struct {
	Theme          string `json:"theme,omitempty"`
	EditorMaxLines int    `json:"editor_max_lines,omitempty"`
}
