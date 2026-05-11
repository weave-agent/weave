package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllThinkingLevels(t *testing.T) {
	assert.Len(t, AllThinkingLevels, 6)
	assert.Equal(t, ThinkingOff, AllThinkingLevels[0])
	assert.Equal(t, ThinkingXHigh, AllThinkingLevels[5])
}

func TestParseThinkingLevel(t *testing.T) {
	for _, l := range AllThinkingLevels {
		got, err := ParseThinkingLevel(string(l))
		require.NoError(t, err)
		assert.Equal(t, l, got)
	}

	_, err := ParseThinkingLevel("invalid")
	assert.Error(t, err)
}

func TestClampForModel(t *testing.T) {
	modelWithXHigh := ModelDef{ID: "test", SupportsXHigh: true}
	modelNoXHigh := ModelDef{ID: "test", SupportsXHigh: false}

	assert.Equal(t, ThinkingXHigh, ClampForModel(ThinkingXHigh, modelWithXHigh))
	assert.Equal(t, ThinkingHigh, ClampForModel(ThinkingXHigh, modelNoXHigh))
	assert.Equal(t, ThinkingMedium, ClampForModel(ThinkingMedium, modelNoXHigh))
	assert.Equal(t, ThinkingOff, ClampForModel(ThinkingOff, modelNoXHigh))
}

func TestNewStreamOptions_Defaults(t *testing.T) {
	opts := NewStreamOptions()
	assert.Empty(t, opts.Model)
	assert.Equal(t, ThinkingOff, opts.ThinkingLevel)
	assert.Equal(t, int64(0), opts.MaxTokens)
}

func TestNewStreamOptions_FunctionalOptions(t *testing.T) {
	opts := NewStreamOptions(
		WithModel("claude-opus-4-7"),
		WithThinkingLevel(ThinkingHigh),
		WithMaxTokens(8192),
	)
	assert.Equal(t, "claude-opus-4-7", opts.Model)
	assert.Equal(t, ThinkingHigh, opts.ThinkingLevel)
	assert.Equal(t, int64(8192), opts.MaxTokens)
}

func TestNewStreamOptions_PartialOptions(t *testing.T) {
	opts := NewStreamOptions(WithThinkingLevel(ThinkingMedium))
	assert.Empty(t, opts.Model)
	assert.Equal(t, ThinkingMedium, opts.ThinkingLevel)
	assert.Equal(t, int64(0), opts.MaxTokens)
}
