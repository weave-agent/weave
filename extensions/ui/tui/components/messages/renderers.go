package messages

import "github.com/weave-agent/weave/sdk"

// messageRenderers holds custom renderers registered by TUI extensions.
var messageRenderers = make(map[string]sdk.MessageRenderer)

// SetMessageRenderer registers a custom renderer for a message type.
func SetMessageRenderer(msgType string, renderer sdk.MessageRenderer) {
	if renderer == nil {
		delete(messageRenderers, msgType)
		return
	}

	messageRenderers[msgType] = renderer
}

// GetMessageRenderer returns a registered renderer for the given message type.
func GetMessageRenderer(msgType string) (sdk.MessageRenderer, bool) {
	r, ok := messageRenderers[msgType]
	return r, ok
}

// ResetMessageRenderers clears all registered message renderers (for testing).
func ResetMessageRenderers() {
	messageRenderers = make(map[string]sdk.MessageRenderer)
}

// GetThemeInfo returns the current theme info for message rendering.
// Set by the TUI during initialization.
var GetThemeInfo func() sdk.ThemeInfo
