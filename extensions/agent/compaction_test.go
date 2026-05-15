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

func makeLongText(nChars int) string {
	return string(make([]byte, nChars))
}

func TestFindCutPoint(t *testing.T) {
	t.Run("empty messages", func(t *testing.T) {
		assert.Equal(t, 0, findCutPoint(nil, 100))
		assert.Equal(t, 0, findCutPoint([]sdk.Message{}, 100))
	})

	t.Run("all messages fit no cut needed", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("hi"),
			sdk.NewAssistantMessage("hello"),
		}
		// keepRecentTokens=10000 is way more than these messages
		assert.Equal(t, 0, findCutPoint(msgs, 10000))
	})

	t.Run("cut in middle of conversation", func(t *testing.T) {
		// 10 user messages of 400 chars each (100 tokens each)
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
		}

		// With keepRecentTokens=300, we want to keep the last ~3 messages
		// (300 tokens = 3 messages of 100 tokens each)
		cut := findCutPoint(msgs, 300)
		assert.Greater(t, cut, 0, "should find a cut point")
		assert.Less(t, cut, len(msgs), "cut should be within bounds")
		assert.Equal(t, sdk.RoleUser, msgs[cut].Role, "cut should be at a user message")
	})

	t.Run("tool result boundary preservation", func(t *testing.T) {
		// Build: user -> assistant(tool_call) -> tool_result -> assistant(text)
		user := sdk.NewUserMessage("run this")
		asstWithTool := sdk.NewAssistantMessage("")
		asstWithTool.ToolCalls = []sdk.ToolCall{
			{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "echo hi"}},
		}
		toolResult := sdk.NewToolResultMessage("tc1", "bash", makeLongText(400), false)
		asstText := sdk.NewAssistantMessage("done")

		// Prepend large messages to force a cut
		bigMsgs := make([]sdk.Message, 5)
		for i := range bigMsgs {
			bigMsgs[i] = sdk.NewUserMessage(makeLongText(400))
		}

		msgs := append(bigMsgs,
			user, asstWithTool, toolResult, asstText,
		)

		// keepRecentTokens small enough to force cut into bigMsgs
		cut := findCutPoint(msgs, 200)

		// Verify the cut never lands on a tool_result message
		if cut > 0 && cut < len(msgs) {
			assert.NotEqual(t, sdk.RoleToolResult, msgs[cut].Role,
				"cut point must never be a tool_result message")
		}

		// Verify: tool_result at index 7 (toolResult) must either be before
		// the cut (summarized together with its parent assistant) or after
		// the cut (kept with its parent). The parent assistant with tool
		// calls is at index 6.
		toolResultIdx := len(bigMsgs) + 1 // asstWithTool is at +1, toolResult at +2
		if cut > 0 && cut <= toolResultIdx {
			// Cut is at or after the assistant with tool calls — verify it
			// doesn't split them: cut must be <= index of asstWithTool
			// or > index of toolResult
			asstIdx := len(bigMsgs) + 1
			resultIdx := len(bigMsgs) + 2
			if cut > asstIdx && cut <= resultIdx {
				t.Errorf("cut at %d splits assistant(tool) at %d from result at %d",
					cut, asstIdx, resultIdx)
			}
		}
	})

	t.Run("oversized single turn all kept", func(t *testing.T) {
		// Single message larger than keepRecentTokens — must return 0
		// (can't summarize everything, so keep all)
		msgs := []sdk.Message{
			sdk.NewUserMessage(makeLongText(4000)),
		}
		// keepRecentTokens=50, single message is 1000 tokens
		// It exceeds keepRecentTokens but it's the only message, so cut=0
		assert.Equal(t, 0, findCutPoint(msgs, 50))
	})

	t.Run("cut respects user message boundary", func(t *testing.T) {
		// user(100 tok) -> assistant(100 tok) -> user(100 tok) -> assistant(100 tok)
		msgs := []sdk.Message{
			sdk.NewUserMessage(makeLongText(400)), // 100 tokens
			sdk.NewAssistantMessage(makeLongText(400)),
			sdk.NewUserMessage(makeLongText(400)),
			sdk.NewAssistantMessage(makeLongText(400)),
		}

		// keepRecentTokens=150 -> last 2 messages (200 tokens) exceed threshold
		// Cut should land at index 2 (third user message) or later
		cut := findCutPoint(msgs, 150)
		assert.GreaterOrEqual(t, cut, 2, "cut should skip to at least message 2")
		assert.Less(t, cut, len(msgs))
	})

	t.Run("assistant without tool calls is valid boundary", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage(makeLongText(400)),
			sdk.NewAssistantMessage(makeLongText(400)),
			sdk.NewUserMessage(makeLongText(400)),
			sdk.NewAssistantMessage(makeLongText(400)),
		}

		// keepRecentTokens=150 should cut near the middle
		cut := findCutPoint(msgs, 150)
		if cut > 0 && cut < len(msgs) {
			role := msgs[cut].Role
			assert.True(t, role == sdk.RoleUser || role == sdk.RoleAssistant,
				"cut should be at user or assistant message")
		}
	})
}

func TestFindValidBoundary(t *testing.T) {
	t.Run("user message is valid boundary", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("hello"),
		}
		assert.Equal(t, 0, findValidBoundary(msgs, 0))
	})

	t.Run("assistant without tools is valid boundary", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewAssistantMessage("hi"),
		}
		assert.Equal(t, 0, findValidBoundary(msgs, 0))
	})

	t.Run("tool result is not valid boundary skips forward", func(t *testing.T) {
		asst := sdk.NewAssistantMessage("")
		asst.ToolCalls = []sdk.ToolCall{
			{ID: "tc1", Name: "bash", Arguments: map[string]any{}},
		}
		msgs := []sdk.Message{
			asst,
			sdk.NewToolResultMessage("tc1", "bash", "out", false),
			sdk.NewUserMessage("next"),
		}
		// Starting at index 1 (tool_result) should skip to index 2 (user)
		assert.Equal(t, 2, findValidBoundary(msgs, 1))
	})

	t.Run("assistant with tool calls skips past results", func(t *testing.T) {
		asst := sdk.NewAssistantMessage("")
		asst.ToolCalls = []sdk.ToolCall{
			{ID: "tc1", Name: "bash", Arguments: map[string]any{}},
		}
		msgs := []sdk.Message{
			asst,
			sdk.NewToolResultMessage("tc1", "bash", "out", false),
			sdk.NewAssistantMessage("done"),
		}
		// Starting at index 0 (assistant with tools), should skip past
		// tool_result to index 2
		assert.Equal(t, 2, findValidBoundary(msgs, 0))
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
