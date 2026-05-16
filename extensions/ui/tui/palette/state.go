package palette

// State represents the current agent activity state, driving accent color changes.
type State int

const (
	StateIdle        State = iota // no activity
	StateStreaming                // receiving tokens from provider
	StateToolRunning              // executing a tool call
	StateError                    // error state
)

// AccentForState returns the Accent, AccentDim, and AccentBright color values
// for the given agent state.
func AccentForState(s State) (accent, accentDim, accentBright string) {
	switch s {
	case StateIdle:
		return "245", "243", "248"
	case StateStreaming:
		return "45", "39", "51"
	case StateToolRunning:
		return "172", "166", "178"
	case StateError:
		return "167", "160", "173"
	default:
		return "245", "243", "248"
	}
}
