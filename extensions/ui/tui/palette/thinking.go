package palette

import "weave/sdk"

// ThinkingBorderColor returns the ANSI 256-color code for a thinking level.
func ThinkingBorderColor(level sdk.ThinkingLevel) string {
	switch level {
	case sdk.ThinkingMinimal:
		return "246"
	case sdk.ThinkingLow:
		return "67"
	case sdk.ThinkingMedium:
		return "99"
	case sdk.ThinkingHigh:
		return "139"
	case sdk.ThinkingXHigh:
		return "177"
	default:
		return "240"
	}
}
