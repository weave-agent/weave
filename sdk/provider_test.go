package sdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weave-agent/weave/sdk/model"
)

type streamOnlyProvider struct{}

func (streamOnlyProvider) Stream(context.Context, ProviderRequest, ...model.StreamOption) (<-chan ProviderEvent, error) {
	return nil, nil
}

type countingProvider struct {
	streamOnlyProvider
}

func (countingProvider) CountTokens(context.Context, ProviderRequest, ...model.StreamOption) (TokenCount, error) {
	return TokenCount{
		InputTokens:  123,
		OutputTokens: 45,
		Source:       TokenCountSourceExact,
		Confidence:   1,
	}, nil
}

var (
	_ Provider     = streamOnlyProvider{}
	_ Provider     = countingProvider{}
	_ TokenCounter = countingProvider{}
)

func TestTokenCounterIsOptionalProviderCapability(t *testing.T) {
	var provider Provider = streamOnlyProvider{}

	_, ok := provider.(TokenCounter)
	assert.False(t, ok)
}

func TestTokenCounterCanCountProviderRequest(t *testing.T) {
	var provider Provider = countingProvider{}

	counter, ok := provider.(TokenCounter)
	require.True(t, ok)

	count, err := counter.CountTokens(
		context.Background(),
		ProviderRequest{SystemPrompt: "system", Messages: []Message{{Role: "user", Content: "hello"}}},
		model.WithMaxTokens(1000),
	)
	require.NoError(t, err)
	assert.Equal(t, 123, count.InputTokens)
	assert.Equal(t, 45, count.OutputTokens)
	assert.Equal(t, TokenCountSourceExact, count.Source)
	assert.Equal(t, 1.0, count.Confidence)
}

func TestTokenCountIsZero(t *testing.T) {
	assert.True(t, TokenCount{}.IsZero())

	assert.False(t, TokenCount{InputTokens: 1}.IsZero())
	assert.False(t, TokenCount{OutputTokens: 1}.IsZero())
	assert.False(t, TokenCount{Source: TokenCountSourceTokenizer}.IsZero())
	assert.False(t, TokenCount{Confidence: 0.5}.IsZero())
}
