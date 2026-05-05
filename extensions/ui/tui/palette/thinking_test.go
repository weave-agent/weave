package palette

import (
	"testing"

	"weave/sdk/model"

	"github.com/stretchr/testify/assert"
)

func TestThinkingBorderColor_AllLevels(t *testing.T) {
	tests := []struct {
		level model.ThinkingLevel
		want  string
	}{
		{model.ThinkingOff, "240"},
		{model.ThinkingMinimal, "246"},
		{model.ThinkingLow, "67"},
		{model.ThinkingMedium, "99"},
		{model.ThinkingHigh, "139"},
		{model.ThinkingXHigh, "177"},
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
