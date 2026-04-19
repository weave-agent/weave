package sdk

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
	ProviderEventDone      = "done"
	ProviderEventError     = "error"
)

type Provider interface {
	Stream(ctx context.Context, req ProviderRequest) (<-chan ProviderEvent, error)
}
