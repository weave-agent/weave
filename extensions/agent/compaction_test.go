package agent

import (
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
)

func TestEstimateTokens(t *testing.T) {
	t.Run("empty messages", func(t *testing.T) {
		assert.Equal(t, 0, estimateTokens(nil))
		assert.Equal(t, 0, estimateTokens([]sdk.Message{}))
	})

	t.Run("user text", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("Hello, world!"), // 13 chars -> 3 tokens
		}
		assert.Equal(t, 3, estimateTokens(msgs))
	})

	t.Run("assistant with tool calls", func(t *testing.T) {
		msg := sdk.NewAssistantMessage("Let me run that.")
		msg.ToolCalls = []sdk.ToolCall{
			{Name: "bash", Arguments: map[string]any{"command": "echo hi"}},
		}
		msgs := []sdk.Message{msg}

		got := estimateTokens(msgs)
		assert.Positive(t, got, "should estimate non-zero tokens for assistant with tool calls")
	})

	t.Run("tool result message", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewToolResultMessage("tc1", "bash", "command output here", false),
		}
		got := estimateTokens(msgs)
		assert.Positive(t, got)
	})

	t.Run("mixed conversation", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("Write a function"),
			sdk.NewAssistantMessage("Here is the function:"),
			sdk.NewToolResultMessage("tc1", "bash", "output", false),
			sdk.NewAssistantMessage("Done"),
		}
		got := estimateTokens(msgs)
		assert.Positive(t, got)
	})

	t.Run("with thinking blocks", func(t *testing.T) {
		msg := sdk.NewAssistantMessage("answer")
		msg.Thinking = []sdk.SignedThinking{
			{Thinking: "I need to consider multiple approaches", Signature: "sig123"},
		}
		msgs := []sdk.Message{msg}

		got := estimateTokens(msgs)
		assert.Positive(t, got)

		// Verify thinking content adds tokens beyond just the text content
		plainMsg := sdk.NewAssistantMessage("answer")
		plainTokens := estimateTokens([]sdk.Message{plainMsg})
		assert.Greater(t, got, plainTokens, "thinking blocks should add to token estimate")
	})

	t.Run("with redacted thinking", func(t *testing.T) {
		msg := sdk.NewAssistantMessage("answer")
		msg.RedactedThinking = []sdk.RedactedThinking{
			{Data: "redacted-block-data-here"},
		}
		msgs := []sdk.Message{msg}

		got := estimateTokens(msgs)
		assert.Positive(t, got)
	})

	t.Run("nil content", func(t *testing.T) {
		msgs := []sdk.Message{
			{Role: sdk.RoleUser, Content: nil},
		}
		assert.Equal(t, 0, estimateTokens(msgs))
	})

	t.Run("integer content uses Sprint", func(t *testing.T) {
		msgs := []sdk.Message{
			{Role: sdk.RoleUser, Content: 42},
		}
		// "42" = 2 chars -> 0 tokens (integer division)
		assert.Equal(t, 0, estimateTokens(msgs))
	})

	t.Run("large text scales linearly", func(t *testing.T) {
		small := sdk.NewUserMessage("hi")                       // 2 chars -> 0 tokens
		large := sdk.NewUserMessage(string(make([]byte, 4000))) // 4000 chars -> 1000 tokens

		smallTokens := estimateTokens([]sdk.Message{small})
		largeTokens := estimateTokens([]sdk.Message{large})

		assert.Equal(t, 0, smallTokens)
		assert.Equal(t, 1000, largeTokens)
	})

	t.Run("tool call arguments serialized", func(t *testing.T) {
		msg := sdk.NewAssistantMessage("")
		msg.ToolCalls = []sdk.ToolCall{
			{
				Name: "edit",
				Arguments: map[string]any{
					"file_path":  "/some/long/path/to/file.go",
					"old_string": "func old()",
					"new_string": "func new()",
				},
			},
		}
		msgs := []sdk.Message{msg}

		got := estimateTokens(msgs)
		assert.Positive(t, got)
	})

	t.Run("multiple tool calls in single message", func(t *testing.T) {
		msg := sdk.NewAssistantMessage("")
		msg.ToolCalls = []sdk.ToolCall{
			{Name: "bash", Arguments: map[string]any{"command": "echo 1"}},
			{Name: "bash", Arguments: map[string]any{"command": "echo 2"}},
		}
		msgs := []sdk.Message{msg}

		singleMsg := sdk.NewAssistantMessage("")
		singleMsg.ToolCalls = []sdk.ToolCall{
			{Name: "bash", Arguments: map[string]any{"command": "echo 1"}},
		}
		singleTokens := estimateTokens([]sdk.Message{singleMsg})

		doubleTokens := estimateTokens(msgs)
		assert.Greater(t, doubleTokens, singleTokens, "two tool calls should estimate more than one")
	})
}

func TestEstimateContentTokens(t *testing.T) {
	t.Run("string content", func(t *testing.T) {
		assert.Equal(t, 5, estimateContentTokens("abcdefghijklmnopqrst"))
	})

	t.Run("nil content", func(t *testing.T) {
		assert.Equal(t, 0, estimateContentTokens(nil))
	})

	t.Run("byte slice content", func(t *testing.T) {
		assert.Equal(t, 2, estimateContentTokens([]byte("abcdefgh")))
	})

	t.Run("default uses Sprint", func(t *testing.T) {
		assert.Equal(t, 0, estimateContentTokens(42)) // "42" = 2 chars / 4 = 0
	})
}
