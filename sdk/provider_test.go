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

func TestNewContextBudgetSnapshot(t *testing.T) {
	snapshot := NewContextBudgetSnapshot(1000, 400, 200, 50)

	assert.Equal(t, 1000, snapshot.ContextWindow)
	assert.Equal(t, 400, snapshot.InputTokens)
	assert.Equal(t, 200, snapshot.OutputReserveTokens)
	assert.Equal(t, 50, snapshot.SafetyMarginTokens)
	assert.Equal(t, 650, snapshot.UsedTokens)
	assert.Equal(t, 350, snapshot.RemainingTokens)
	assert.Equal(t, 65.0, snapshot.PercentUsed)
}

func TestNewContextBudgetSnapshotAllowsOverBudget(t *testing.T) {
	snapshot := NewContextBudgetSnapshot(1000, 900, 200, 50)

	assert.Equal(t, 1150, snapshot.UsedTokens)
	assert.Equal(t, -150, snapshot.RemainingTokens)
	assert.Equal(t, 115.0, snapshot.PercentUsed)
}

func TestNewContextBudgetSnapshotUnknownWindow(t *testing.T) {
	for _, contextWindow := range []int{0, -1} {
		snapshot := NewContextBudgetSnapshot(contextWindow, 100, 25, 10)

		assert.Equal(t, contextWindow, snapshot.ContextWindow)
		assert.Equal(t, 135, snapshot.UsedTokens)
		assert.Zero(t, snapshot.RemainingTokens)
		assert.Zero(t, snapshot.PercentUsed)
	}
}

func TestNewContextBudgetSnapshotRoundsPercentUsed(t *testing.T) {
	snapshot := NewContextBudgetSnapshot(3, 1, 0, 0)

	assert.Equal(t, 2, snapshot.RemainingTokens)
	assert.Equal(t, 33.33, snapshot.PercentUsed)

	snapshot = NewContextBudgetSnapshot(6, 1, 0, 0)

	assert.Equal(t, 5, snapshot.RemainingTokens)
	assert.Equal(t, 16.67, snapshot.PercentUsed)
}
