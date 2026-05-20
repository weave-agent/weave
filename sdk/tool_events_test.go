package sdk

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolProgress_Marshal(t *testing.T) {
	p := ToolProgress{
		ToolCallID: "call_123",
		ToolName:   "read",
		Content:    "partial output",
		IsError:    true,
	}

	b, err := json.Marshal(p)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	assert.Equal(t, "call_123", got["tool_call_id"])
	assert.Equal(t, "read", got["tool_name"])
	assert.Equal(t, "partial output", got["content"])
	assert.Equal(t, true, got["is_error"])
}

func TestToolProgress_MarshalOmitempty(t *testing.T) {
	p := ToolProgress{
		ToolCallID: "call_456",
		ToolName:   "edit",
	}

	b, err := json.Marshal(p)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	assert.Equal(t, "call_456", got["tool_call_id"])
	assert.Equal(t, "edit", got["tool_name"])
	assert.NotContains(t, got, "content")
	assert.NotContains(t, got, "is_error")
}

func TestToolProgress_Unmarshal(t *testing.T) {
	in := `{"tool_call_id":"call_789","tool_name":"bash","content":"done","is_error":false}`

	var p ToolProgress
	require.NoError(t, json.Unmarshal([]byte(in), &p))

	assert.Equal(t, "call_789", p.ToolCallID)
	assert.Equal(t, "bash", p.ToolName)
	assert.Equal(t, "done", p.Content)
	assert.False(t, p.IsError)
}
