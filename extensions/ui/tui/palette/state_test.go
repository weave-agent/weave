package palette

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAccentForState_AllStates(t *testing.T) {
	tests := []struct {
		state            State
		wantAccent       string
		wantAccentDim    string
		wantAccentBright string
	}{
		{StateIdle, "245", "243", "248"},
		{StateStreaming, "45", "39", "51"},
		{StateToolRunning, "172", "166", "178"},
		{StateError, "167", "160", "173"},
	}

	for _, tt := range tests {
		t.Run(stateName(tt.state), func(t *testing.T) {
			a, ad, ab := AccentForState(tt.state)
			assert.Equal(t, tt.wantAccent, a)
			assert.Equal(t, tt.wantAccentDim, ad)
			assert.Equal(t, tt.wantAccentBright, ab)
		})
	}
}

func TestAccentForState_DefaultFallback(t *testing.T) {
	// State(-1) is not a valid state, should return idle defaults
	a, ad, ab := AccentForState(State(-1))
	assert.Equal(t, "245", a)
	assert.Equal(t, "243", ad)
	assert.Equal(t, "248", ab)
}

func TestAccentForState_IdleMatchesDefaultTheme(t *testing.T) {
	theme := DefaultTheme()
	a, _, _ := AccentForState(StateIdle)
	assert.Equal(t, theme.Accent, a)
}

func stateName(s State) string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateStreaming:
		return "Streaming"
	case StateToolRunning:
		return "ToolRunning"
	case StateError:
		return "Error"
	default:
		return "Unknown"
	}
}
