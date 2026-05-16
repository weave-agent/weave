package sdk

import (
	"fmt"
	"time"
)

const (
	RoleUser       = "user"
	RoleAssistant  = "assistant"
	RoleToolResult = "tool_result"
)

type Message struct {
	Role             string
	Content          any
	ToolCalls        []ToolCall
	ToolCallID       string
	ToolName         string
	IsError          bool
	Thinking         []SignedThinking
	RedactedThinking []RedactedThinking
	Timestamp        time.Time
}

func (m Message) Validate() error {
	switch m.Role {
	case RoleUser, RoleAssistant, RoleToolResult:
		return nil
	default:
		return fmt.Errorf("invalid message role %q: must be %q, %q, or %q", m.Role, RoleUser, RoleAssistant, RoleToolResult)
	}
}

func NewUserMessage(content any) Message {
	return Message{Role: RoleUser, Content: content, Timestamp: time.Now()}
}

func NewAssistantMessage(content any) Message {
	return Message{Role: RoleAssistant, Content: content, Timestamp: time.Now()}
}

func NewToolResultMessage(toolCallID, toolName string, content any, isError bool) Message {
	var wrapped any

	switch c := content.(type) {
	case string:
		wrapped = fmt.Sprintf("<tool_output name=%q>\n%s\n</tool_output>", toolName, c)
	default:
		wrapped = fmt.Sprintf("<tool_output name=%q>\n%v\n</tool_output>", toolName, c)
	}

	return Message{Role: RoleToolResult, Content: wrapped, ToolCallID: toolCallID, ToolName: toolName, IsError: isError, Timestamp: time.Now()}
}
