package palette

import (
	"testing"

	"github.com/weave-agent/weave/sdk/model"

	"github.com/stretchr/testify/assert"
)

func TestThinkingBorderColor_AllLevels(t *testing.T) {
	tests := []struct {
		level model.ThinkingLevel
		want  string
	}{
		{model.ThinkingOff, "240"},
		{model.ThinkingMinimal, "242"},
		{model.ThinkingLow, "244"},
		{model.ThinkingMedium, "246"},
		{model.ThinkingHigh, "248"},
		{model.ThinkingXHigh, "250"},
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
