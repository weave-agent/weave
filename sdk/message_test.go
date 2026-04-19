package sdk

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUserMessage(t *testing.T) {
	before := time.Now()
	msg := NewUserMessage("hello")
	after := time.Now()

	assert.Equal(t, RoleUser, msg.Role)
	assert.Equal(t, "hello", msg.Content)
	assert.Empty(t, msg.ToolCallID)
	assert.Empty(t, msg.ToolName)
	assert.True(t, !msg.Timestamp.Before(before) && !msg.Timestamp.After(after))
}

func TestNewAssistantMessage(t *testing.T) {
	before := time.Now()
	msg := NewAssistantMessage(map[string]any{"text": "response"})
	after := time.Now()

	assert.Equal(t, RoleAssistant, msg.Role)
	require.NotNil(t, msg.Content)

	m, ok := msg.Content.(map[string]any)
	require.True(t, ok, "Content type = %T, want map[string]any", msg.Content)
	assert.Equal(t, "response", m["text"])
	assert.True(t, !msg.Timestamp.Before(before) && !msg.Timestamp.After(after))
}

func TestNewToolResultMessage(t *testing.T) {
	before := time.Now()
	msg := NewToolResultMessage("call_123", "bash", "output text", false)
	after := time.Now()

	assert.Equal(t, RoleToolResult, msg.Role)
	assert.Equal(t, "output text", msg.Content)
	assert.Equal(t, "call_123", msg.ToolCallID)
	assert.Equal(t, "bash", msg.ToolName)
	assert.False(t, msg.IsError)
	assert.True(t, !msg.Timestamp.Before(before) && !msg.Timestamp.After(after))
}

func TestNewToolResultMessage_Error(t *testing.T) {
	msg := NewToolResultMessage("call_err", "bash", "command failed", true)
	assert.True(t, msg.IsError)
}

func TestNewUserMessage_NilContent(t *testing.T) {
	msg := NewUserMessage(nil)
	assert.Nil(t, msg.Content)
}

func TestNewAssistantMessage_NilContent(t *testing.T) {
	msg := NewAssistantMessage(nil)
	assert.Nil(t, msg.Content)
}

func TestMessageValidate(t *testing.T) {
	tests := []struct {
		name    string
		msg     Message
		wantErr bool
	}{
		{
			name:    "user role valid",
			msg:     NewUserMessage("hi"),
			wantErr: false,
		},
		{
			name:    "assistant role valid",
			msg:     NewAssistantMessage("hello"),
			wantErr: false,
		},
		{
			name:    "tool_result role valid",
			msg:     NewToolResultMessage("id", "tool", "result", false),
			wantErr: false,
		},
		{
			name:    "empty role invalid",
			msg:     Message{Role: "", Timestamp: time.Now()},
			wantErr: true,
		},
		{
			name:    "system role invalid",
			msg:     Message{Role: "system", Timestamp: time.Now()},
			wantErr: true,
		},
		{
			name:    "unknown role invalid",
			msg:     Message{Role: "custom", Timestamp: time.Now()},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if tt.wantErr {
				require.Error(t, err, tt.name)
			} else {
				require.NoError(t, err, tt.name)
			}
		})
	}
}

func TestMessageValidate_ErrorMessage(t *testing.T) {
	msg := Message{Role: "bad_role", Timestamp: time.Now()}

	err := msg.Validate()
	require.Error(t, err, "expected error for invalid role")
	assert.Contains(t, err.Error(), "bad_role")
}

// Suppress unused import warning.
var _ = strings.TrimSpace
