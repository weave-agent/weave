package palette

import "weave/sdk/model"

// ThinkingBorderColor returns the ANSI 256-color code for a thinking level.
func ThinkingBorderColor(level model.ThinkingLevel) string {
	switch level {
	case model.ThinkingMinimal:
		return "246"
	case model.ThinkingLow:
		return "67"
	case model.ThinkingMedium:
		return "99"
	case model.ThinkingHigh:
		return "139"
	case model.ThinkingXHigh:
		return "177"
	default:
		return "240"
	}
}
