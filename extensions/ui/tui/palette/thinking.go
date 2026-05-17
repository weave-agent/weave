package palette

import "github.com/weave-agent/weave/sdk/model"

// ThinkingBorderColor returns the ANSI 256-color code for a thinking level.
// Uses grayscale temperature mapping for intensity.
func ThinkingBorderColor(level model.ThinkingLevel) string {
	switch level {
	case model.ThinkingMinimal:
		return "242"
	case model.ThinkingLow:
		return "244"
	case model.ThinkingMedium:
		return "246"
	case model.ThinkingHigh:
		return "248"
	case model.ThinkingXHigh:
		return "250"
	default:
		return "240"
	}
}
