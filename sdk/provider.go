package sdk

//go:generate moq -fmt goimports -stub -out provider_mock_test.go . Provider

import "context"

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
	ProviderEventTextDelta = "text_delta"
	ProviderEventToolCall  = "tool_call"
	ProviderEventError     = "error"
	ProviderEventThinking  = "thinking_delta"
)

// ToolCall represents a parsed tool call from the provider response.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// StreamOption is a functional option for configuring per-request stream behavior.
type StreamOption func(*StreamOptions)

// NewStreamOptions creates StreamOptions with defaults, applying any given options.
func NewStreamOptions(opts ...StreamOption) *StreamOptions {
	so := &StreamOptions{
		ThinkingLevel: ThinkingOff,
	}
	for _, o := range opts {
		o(so)
	}

	return so
}

// WithModel sets the model for this request.
func WithModel(model string) StreamOption {
	return func(o *StreamOptions) { o.Model = model }
}

// WithThinkingLevel sets the thinking level for this request.
func WithThinkingLevel(level ThinkingLevel) StreamOption {
	return func(o *StreamOptions) { o.ThinkingLevel = level }
}

// WithMaxTokens sets the max output tokens for this request.
func WithMaxTokens(n int64) StreamOption {
	return func(o *StreamOptions) { o.MaxTokens = n }
}

type Provider interface {
	Stream(ctx context.Context, req ProviderRequest, opts ...StreamOption) (<-chan ProviderEvent, error)
}
