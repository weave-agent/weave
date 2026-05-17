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
