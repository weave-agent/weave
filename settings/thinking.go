package settings

import (
	"os"

	"github.com/weave-agent/weave/sdk/model"
)

// DefaultThinkingLevel reads the initial thinking level from WEAVE_THINKING_LEVEL,
// falling back to ThinkingMedium.
func DefaultThinkingLevel() model.ThinkingLevel {
	if v := os.Getenv("WEAVE_THINKING_LEVEL"); v != "" {
		if lvl, err := model.ParseThinkingLevel(v); err == nil {
			return lvl
		}
	}

	return model.ThinkingMedium
}
