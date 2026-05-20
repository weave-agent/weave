package sdk

// ToolProgress carries a partial or final output chunk from a running tool.
// Extensions that support streaming emit this on the bus at tool.progress.
type ToolProgress struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Content    string `json:"content,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
}

const (
	TopicToolStart       = "tool.start"
	TopicToolProgress    = "tool.progress"
	TopicToolComplete    = "tool.complete"
	TopicToolError       = "tool.error"
	TopicToolInterrupted = "tool.interrupted"
)
