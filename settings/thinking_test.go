package settings

import (
	"testing"

	"github.com/weave-agent/weave/sdk/model"

	"github.com/stretchr/testify/assert"
)

func TestDefaultThinkingLevel(t *testing.T) {
	t.Setenv("WEAVE_THINKING_LEVEL", "high")
	assert.Equal(t, model.ThinkingHigh, DefaultThinkingLevel())
}

func TestDefaultThinkingLevel_Unset(t *testing.T) {
	t.Setenv("WEAVE_THINKING_LEVEL", "")
	assert.Equal(t, model.ThinkingMedium, DefaultThinkingLevel())
}

func TestDefaultThinkingLevel_Invalid(t *testing.T) {
	t.Setenv("WEAVE_THINKING_LEVEL", "garbage")
	assert.Equal(t, model.ThinkingMedium, DefaultThinkingLevel())
}
