package sdk

import "time"

type Message struct {
	Role       string
	Content    any
	ToolCallID string
	ToolName   string
	Timestamp  time.Time
}

func NewUserMessage(content any) Message {
	return Message{Role: "user", Content: content, Timestamp: time.Now()}
}

func NewAssistantMessage(content any) Message {
	return Message{Role: "assistant", Content: content, Timestamp: time.Now()}
}

func NewToolResultMessage(toolCallID, toolName string, content any) Message {
	return Message{Role: "tool_result", Content: content, ToolCallID: toolCallID, ToolName: toolName, Timestamp: time.Now()}
}
