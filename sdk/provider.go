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
)

// ToolCall represents a parsed tool call from the provider response.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type Provider interface {
	Stream(ctx context.Context, req ProviderRequest) (<-chan ProviderEvent, error)
}
