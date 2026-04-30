package sdk

// UIExtension is a TUI-specific plugin that customizes the interactive UI.
// UI extensions are discovered by the launcher and wired by the TUI at startup.
// They are silently skipped in headless mode.
type UIExtension interface {
	Name() string
	Register(ui UI)
}
