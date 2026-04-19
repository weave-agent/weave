package sdk

import (
	"strings"
	"testing"
	"time"
)

func TestNewUserMessage(t *testing.T) {
	before := time.Now()
	msg := NewUserMessage("hello")
	after := time.Now()

	if msg.Role != RoleUser {
		t.Errorf("Role = %q, want %q", msg.Role, RoleUser)
	}

	if msg.Content != "hello" {
		t.Errorf("Content = %v, want %v", msg.Content, "hello")
	}

	if msg.ToolCallID != "" {
		t.Errorf("ToolCallID = %q, want empty", msg.ToolCallID)
	}

	if msg.ToolName != "" {
		t.Errorf("ToolName = %q, want empty", msg.ToolName)
	}

	if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", msg.Timestamp, before, after)
	}
}

func TestNewAssistantMessage(t *testing.T) {
	before := time.Now()
	msg := NewAssistantMessage(map[string]any{"text": "response"})
	after := time.Now()

	if msg.Role != RoleAssistant {
		t.Errorf("Role = %q, want %q", msg.Role, RoleAssistant)
	}

	if msg.Content == nil {
		t.Fatal("Content is nil")
	}

	m, ok := msg.Content.(map[string]any)
	if !ok {
		t.Fatalf("Content type = %T, want map[string]any", msg.Content)
	}

	if m["text"] != "response" {
		t.Errorf("Content[text] = %v, want %q", m["text"], "response")
	}

	if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", msg.Timestamp, before, after)
	}
}

func TestNewToolResultMessage(t *testing.T) {
	before := time.Now()
	msg := NewToolResultMessage("call_123", "bash", "output text")
	after := time.Now()

	if msg.Role != RoleToolResult {
		t.Errorf("Role = %q, want %q", msg.Role, RoleToolResult)
	}

	if msg.Content != "output text" {
		t.Errorf("Content = %v, want %v", msg.Content, "output text")
	}

	if msg.ToolCallID != "call_123" {
		t.Errorf("ToolCallID = %q, want %q", msg.ToolCallID, "call_123")
	}

	if msg.ToolName != "bash" {
		t.Errorf("ToolName = %q, want %q", msg.ToolName, "bash")
	}

	if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", msg.Timestamp, before, after)
	}
}

func TestNewUserMessage_NilContent(t *testing.T) {
	msg := NewUserMessage(nil)
	if msg.Content != nil {
		t.Errorf("Content = %v, want nil", msg.Content)
	}
}

func TestNewAssistantMessage_NilContent(t *testing.T) {
	msg := NewAssistantMessage(nil)
	if msg.Content != nil {
		t.Errorf("Content = %v, want nil", msg.Content)
	}
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
			msg:     NewToolResultMessage("id", "tool", "result"),
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
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMessageValidate_ErrorMessage(t *testing.T) {
	msg := Message{Role: "bad_role", Timestamp: time.Now()}

	err := msg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid role")
	}

	if !strings.Contains(err.Error(), "bad_role") {
		t.Errorf("error message %q should contain role %q", err.Error(), "bad_role")
	}
}
