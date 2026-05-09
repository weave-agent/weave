package sdk

// UIExtension is a TUI-specific plugin that customizes the interactive UI.
// UI extensions are discovered by the launcher and wired by the TUI at startup.
// They are silently skipped in headless mode.
type UIExtension interface {
	Name() string
	Register(ui UI)
}

// UIExtensionWithBus is an optional interface that UI extensions can implement
// to receive the event bus during TUI wiring, enabling them to subscribe to
// bus events and publish responses.
type UIExtensionWithBus interface {
	UIExtension
	RegisterWithBus(ui UI, bus Bus)
}
