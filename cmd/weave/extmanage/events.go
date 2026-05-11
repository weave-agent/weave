package extmanage

// OutdatedInfo describes a single extension that has a newer version available.
type OutdatedInfo struct {
	Name       string
	LocalHead  string
	RemoteHead string
}

// OutdatedEvent is the payload for the "extension.outdated" bus event.
type OutdatedEvent struct {
	Extensions []OutdatedInfo
}
