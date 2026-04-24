package palette

import (
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
)

func TestThinkingBorderColor_AllLevels(t *testing.T) {
	tests := []struct {
		level sdk.ThinkingLevel
		want  string
	}{
		{sdk.ThinkingOff, "240"},
		{sdk.ThinkingMinimal, "246"},
		{sdk.ThinkingLow, "67"},
		{sdk.ThinkingMedium, "99"},
		{sdk.ThinkingHigh, "139"},
		{sdk.ThinkingXHigh, "177"},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			assert.Equal(t, tt.want, ThinkingBorderColor(tt.level))
		})
	}
}

func TestThinkingBorderColor_UnknownLevel(t *testing.T) {
	assert.Equal(t, "240", ThinkingBorderColor("unknown"))
}
