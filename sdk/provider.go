package sdk

//go:generate moq -fmt goimports -stub -out provider_mock_test.go . Provider

import (
	"context"

	"github.com/weave-agent/weave/sdk/model"
)

type ProviderRequest struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDef
}

type ProviderEvent struct {
	Type    string
	Content any
}

const (
	ProviderEventTextDelta            = "text_delta"
	ProviderEventToolCall             = "tool_call"
	ProviderEventError                = "error"
	ProviderEventThinking             = "thinking_delta"
	ProviderEventThinkingDone         = "thinking_done"
	ProviderEventRedactedThinkingDone = "redacted_thinking_done"
	ProviderEventUsage                = "usage"
)

// ProviderUsage holds token usage information from a provider response.
type ProviderUsage struct {
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
}

// TokenCountSource describes how a preflight token count was produced.
type TokenCountSource string

const (
	// TokenCountSourceExact means the count came from the provider or the
	// provider's canonical tokenizer for the fully rendered request.
	TokenCountSourceExact TokenCountSource = "exact"
	// TokenCountSourceTokenizer means the count came from a compatible tokenizer
	// but may differ from the provider's final accounting.
	TokenCountSourceTokenizer TokenCountSource = "tokenizer"
	// TokenCountSourceHeuristic means the count came from an approximate fallback.
	TokenCountSourceHeuristic TokenCountSource = "heuristic"
)

// TokenCount holds provider-neutral preflight token counting metadata.
type TokenCount struct {
	InputTokens  int
	OutputTokens int
	Source       TokenCountSource
	Confidence   float64
}

// IsZero reports whether no token count metadata is present.
func (c TokenCount) IsZero() bool {
	return c.InputTokens == 0 &&
		c.OutputTokens == 0 &&
		c.Source == "" &&
		c.Confidence == 0
}

// ContextBudgetSnapshot captures provider-neutral request budget accounting.
type ContextBudgetSnapshot struct {
	ContextWindow       int
	InputTokens         int
	OutputReserveTokens int
	SafetyMarginTokens  int
	UsedTokens          int
	RemainingTokens     int
	PercentUsed         float64
}

// NewContextBudgetSnapshot returns request budget arithmetic for a model
// context window without deciding what policy should act on the result.
func NewContextBudgetSnapshot(contextWindow, inputTokens, outputReserveTokens, safetyMarginTokens int) ContextBudgetSnapshot {
	usedTokens := inputTokens + outputReserveTokens + safetyMarginTokens
	snapshot := ContextBudgetSnapshot{
		ContextWindow:       contextWindow,
		InputTokens:         inputTokens,
		OutputReserveTokens: outputReserveTokens,
		SafetyMarginTokens:  safetyMarginTokens,
		UsedTokens:          usedTokens,
	}
	if contextWindow <= 0 {
		return snapshot
	}

	snapshot.RemainingTokens = contextWindow - usedTokens
	snapshot.PercentUsed = roundPercent(float64(usedTokens) / float64(contextWindow) * 100)
	return snapshot
}

func roundPercent(percent float64) float64 {
	if percent < 0 {
		return float64(int(percent*100-0.5)) / 100
	}
	return float64(int(percent*100+0.5)) / 100
}

// SignedThinking holds a signed thinking block from a provider response.
type SignedThinking struct {
	Signature string
	Thinking  string
}

// RedactedThinking holds a redacted thinking block from a provider response.
type RedactedThinking struct {
	Data string
}

// ToolCall represents a parsed tool call from the provider response.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type Provider interface {
	Stream(ctx context.Context, req ProviderRequest, opts ...model.StreamOption) (<-chan ProviderEvent, error)
}

// TokenCounter is an optional provider capability for preflight token counting.
type TokenCounter interface {
	CountTokens(ctx context.Context, req ProviderRequest, opts ...model.StreamOption) (TokenCount, error)
}
