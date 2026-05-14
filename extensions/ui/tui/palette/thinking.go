package palette

import "weave/sdk/model"

// ThinkingBorderColor returns the ANSI 256-color code for a thinking level.
// Uses the primary color family with brightness mapped to intensity.
func ThinkingBorderColor(level model.ThinkingLevel) string {
	theme := DefaultTheme()

	switch level {
	case model.ThinkingMinimal:
		return theme.PrimaryDim
	case model.ThinkingLow:
		return theme.Primary
	case model.ThinkingMedium:
		return theme.PrimaryBright
	case model.ThinkingHigh:
		return "141"
	case model.ThinkingXHigh:
		return "177"
	default:
		return theme.Border
	}
}
