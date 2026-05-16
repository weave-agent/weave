package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"weave/sdk"
	"weave/sdk/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		// Content is wrapped in <tool_output> tags: 60 chars -> 15 tokens
		assert.Equal(t, 15, estimateTokens(msgs))
	})

	t.Run("mixed conversation", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("Write a function"),                   // 16 chars -> 4 tokens
			sdk.NewAssistantMessage("Here is the function:"),         // 21 chars -> 5 tokens
			sdk.NewToolResultMessage("tc1", "bash", "output", false), // wrapped: ~45 chars -> 11 tokens
			sdk.NewAssistantMessage("Done"),                          // 4 chars -> 1 token
		}
		assert.Equal(t, 21, estimateTokens(msgs))
	})

	t.Run("with thinking blocks", func(t *testing.T) {
		msg := sdk.NewAssistantMessage("answer")
		msg.Thinking = []sdk.SignedThinking{
			{Thinking: "I need to consider multiple approaches", Signature: "sig123"},
		}
		msgs := []sdk.Message{msg}

		got := estimateTokens(msgs)
		assert.Positive(t, got)

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
			{Role: sdk.RoleUser, Content: 12345},
		}
		// 12345 -> "12345" = 5 chars -> 1 token
		assert.Equal(t, 1, estimateTokens(msgs))
	})

	t.Run("large text scales linearly", func(t *testing.T) {
		small := sdk.NewUserMessage("hi")
		large := sdk.NewUserMessage(string(make([]byte, 4000)))

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
					"path":       "/some/long/path/to/file.go",
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
		assert.Equal(t, 0, findCutPoint(msgs, 10000))
	})

	t.Run("cut in middle of conversation", func(t *testing.T) {
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
		}

		cut := findCutPoint(msgs, 300)
		assert.Positive(t, cut, "should find a cut point")
		assert.Less(t, cut, len(msgs), "cut should be within bounds")
		assert.Equal(t, sdk.RoleUser, msgs[cut].Role, "cut should be at a user message")
	})

	t.Run("tool result boundary preservation", func(t *testing.T) {
		user := sdk.NewUserMessage("run this")
		asstWithTool := sdk.NewAssistantMessage("")
		asstWithTool.ToolCalls = []sdk.ToolCall{
			{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "echo hi"}},
		}
		toolResult := sdk.NewToolResultMessage("tc1", "bash", makeLongText(400), false)
		asstText := sdk.NewAssistantMessage("done")

		msgs := make([]sdk.Message, 0, 9)
		for range 5 {
			msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
		}

		msgs = append(msgs, user, asstWithTool, toolResult, asstText)

		cut := findCutPoint(msgs, 200)
		require.Positive(t, cut, "should find a cut point")
		require.Less(t, cut, len(msgs), "cut should be within bounds")

		assert.NotEqual(t, sdk.RoleToolResult, msgs[cut].Role,
			"cut point must never be a tool_result message")

		// The tool call/result pair starts at index 6 (after 5 big messages).
		// findValidBoundary should skip past the tool result.
		const bigMsgCount = 5

		asstIdx := bigMsgCount + 1

		resultIdx := bigMsgCount + 2
		if cut > asstIdx && cut <= resultIdx {
			t.Errorf("cut at %d splits assistant(tool) at %d from result at %d",
				cut, asstIdx, resultIdx)
		}
	})

	t.Run("oversized single turn all kept", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage(makeLongText(4000)),
		}
		assert.Equal(t, 0, findCutPoint(msgs, 50))
	})

	t.Run("cut respects user message boundary", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage(makeLongText(400)),
			sdk.NewAssistantMessage(makeLongText(400)),
			sdk.NewUserMessage(makeLongText(400)),
			sdk.NewAssistantMessage(makeLongText(400)),
		}

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
		assert.Equal(t, 2, findValidBoundary(msgs, 0))
	})

	t.Run("consecutive tool groups iterate without stack overflow", func(t *testing.T) {
		msgs := make([]sdk.Message, 0, 201)

		for i := range 100 {
			asst := sdk.NewAssistantMessage("")
			asst.ToolCalls = []sdk.ToolCall{
				{ID: fmt.Sprintf("tc%d", i), Name: "bash", Arguments: map[string]any{}},
			}
			msgs = append(msgs, asst, sdk.NewToolResultMessage(fmt.Sprintf("tc%d", i), "bash", "out", false))
		}

		msgs = append(msgs, sdk.NewUserMessage("next"))
		assert.Equal(t, len(msgs)-1, findValidBoundary(msgs, 0))
	})

	t.Run("interleaved tool results do not skip past user message", func(t *testing.T) {
		asstA := sdk.NewAssistantMessage("")
		asstA.ToolCalls = []sdk.ToolCall{
			{ID: "tcA", Name: "bash", Arguments: map[string]any{}},
		}
		msgs := []sdk.Message{
			asstA,
			sdk.NewToolResultMessage("tcA", "bash", "out", false),
			sdk.NewUserMessage("interleaved user"),
			sdk.NewToolResultMessage("tcB", "bash", "out", false),
			sdk.NewUserMessage("final"),
		}
		// Starting from asstA (index 0), should skip tcA result (index 1),
		// then stop at the interleaved user message (index 2).
		assert.Equal(t, 2, findValidBoundary(msgs, 0))
	})

	t.Run("all tool results returns len", func(t *testing.T) {
		// A conversation ending with orphaned tool results (no trailing user/assistant).
		asst := sdk.NewAssistantMessage("")
		asst.ToolCalls = []sdk.ToolCall{
			{ID: "tc1", Name: "bash", Arguments: map[string]any{}},
		}
		msgs := []sdk.Message{
			asst,
			sdk.NewToolResultMessage("tc1", "bash", "out", false),
			sdk.NewToolResultMessage("tc1", "bash", "more out", false),
		}
		// No valid boundary after startIdx, should return len(msgs).
		assert.Equal(t, len(msgs), findValidBoundary(msgs, 1))
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
		assert.Equal(t, 0, estimateContentTokens(42))
	})
}

func TestSerializeForSummary(t *testing.T) {
	t.Run("empty messages", func(t *testing.T) {
		result := serializeForSummary(nil, "", nil)
		assert.Empty(t, result)
	})

	t.Run("single user message", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("Hello"),
		}
		result := serializeForSummary(msgs, "", nil)
		assert.Contains(t, result, "[User]: Hello")
	})

	t.Run("single turn", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("Write a function"),
			sdk.NewAssistantMessage("Here is the function:"),
		}
		result := serializeForSummary(msgs, "", nil)
		assert.Contains(t, result, "[User]: Write a function")
		assert.Contains(t, result, "[Assistant]: Here is the function:")
	})

	t.Run("multi-turn with tools", func(t *testing.T) {
		msg := sdk.NewAssistantMessage("")
		msg.ToolCalls = []sdk.ToolCall{
			{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "echo hi"}},
		}
		msgs := []sdk.Message{
			sdk.NewUserMessage("run echo"),
			msg,
			sdk.NewToolResultMessage("tc1", "bash", "hi\n", false),
			sdk.NewAssistantMessage("Done"),
		}
		result := serializeForSummary(msgs, "", nil)
		assert.Contains(t, result, "[User]: run echo")
		assert.Contains(t, result, "[Tool call]: bash(")
		assert.Contains(t, result, `"command":"echo hi"`)
		assert.Contains(t, result, "[Tool result]:")
		assert.Contains(t, result, "hi")
		assert.Contains(t, result, "[Assistant]: Done")
	})

	t.Run("truncation of long tool results", func(t *testing.T) {
		longOutput := strings.Repeat("x", 3000)
		msgs := []sdk.Message{
			sdk.NewToolResultMessage("tc1", "bash", longOutput, false),
		}
		result := serializeForSummary(msgs, "", nil)
		assert.Contains(t, result, "... (truncated)")
		assert.Less(t, len(result), 2500)

		// Verify truncation point: content includes XML wrapper + 2000 chars of output.
		// When truncated, the closing </tool_output> tag may be cut off.
		assert.Contains(t, result, "[Tool result]:")
		assert.Contains(t, result, strings.Repeat("x", 1000), "truncation should preserve substantial x characters")
		assert.Contains(t, result, "<tool_output")
	})

	t.Run("previous summary inclusion", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("continue"),
		}
		result := serializeForSummary(msgs, "Previous conversation summary", nil)
		assert.Contains(t, result, "<previous-summary>")
		assert.Contains(t, result, "Previous conversation summary")
		assert.Contains(t, result, "</previous-summary>")
		assert.Contains(t, result, "[User]: continue")
	})

	t.Run("file operations appended", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("do stuff"),
		}
		ops := newFileOperations()
		ops.readFiles["/path/to/read.go"] = true
		ops.modifiedFiles["/path/to/edit.go"] = true

		result := serializeForSummary(msgs, "", ops)
		assert.Contains(t, result, "<read-files>")
		assert.Contains(t, result, "/path/to/read.go")
		assert.Contains(t, result, "</read-files>")
		assert.Contains(t, result, "<modified-files>")
		assert.Contains(t, result, "/path/to/edit.go")
		assert.Contains(t, result, "</modified-files>")
	})

	t.Run("nil content in user message", func(t *testing.T) {
		msgs := []sdk.Message{
			{Role: sdk.RoleUser, Content: nil},
		}
		result := serializeForSummary(msgs, "", nil)
		assert.Contains(t, result, "[User]:")
	})
}

func TestTrackFileOps(t *testing.T) {
	t.Run("no file ops", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("hello"),
			sdk.NewAssistantMessage("hi"),
		}
		ops := newFileOperations()
		trackFileOps(msgs, ops)
		assert.Empty(t, ops.readFiles)
		assert.Empty(t, ops.modifiedFiles)
	})

	t.Run("read tracking", func(t *testing.T) {
		msg := sdk.NewAssistantMessage("")
		msg.ToolCalls = []sdk.ToolCall{
			{Name: "read", Arguments: map[string]any{"path": "/path/to/file.go"}},
		}
		msgs := []sdk.Message{msg}
		ops := newFileOperations()
		trackFileOps(msgs, ops)
		assert.True(t, ops.readFiles["/path/to/file.go"])
		assert.Empty(t, ops.modifiedFiles)
	})

	t.Run("edit/write tracking", func(t *testing.T) {
		editMsg := sdk.NewAssistantMessage("")
		editMsg.ToolCalls = []sdk.ToolCall{
			{Name: "edit", Arguments: map[string]any{"path": "/path/to/edit.go"}},
		}
		writeMsg := sdk.NewAssistantMessage("")
		writeMsg.ToolCalls = []sdk.ToolCall{
			{Name: "write", Arguments: map[string]any{"path": "/path/to/write.go"}},
		}
		msgs := []sdk.Message{editMsg, writeMsg}
		ops := newFileOperations()
		trackFileOps(msgs, ops)
		assert.Empty(t, ops.readFiles)
		assert.True(t, ops.modifiedFiles["/path/to/edit.go"])
		assert.True(t, ops.modifiedFiles["/path/to/write.go"])
	})

	t.Run("accumulation across calls", func(t *testing.T) {
		msg1 := sdk.NewAssistantMessage("")
		msg1.ToolCalls = []sdk.ToolCall{
			{Name: "read", Arguments: map[string]any{"path": "/first.go"}},
		}
		msg2 := sdk.NewAssistantMessage("")
		msg2.ToolCalls = []sdk.ToolCall{
			{Name: "read", Arguments: map[string]any{"path": "/second.go"}},
		}
		ops := newFileOperations()
		trackFileOps([]sdk.Message{msg1}, ops)
		trackFileOps([]sdk.Message{msg2}, ops)
		assert.True(t, ops.readFiles["/first.go"])
		assert.True(t, ops.readFiles["/second.go"])
	})

	t.Run("accumulation across compaction", func(t *testing.T) {
		ops := newFileOperations()

		// Phase 1: initial conversation with read and edit
		readMsg := sdk.NewAssistantMessage("")
		readMsg.ToolCalls = []sdk.ToolCall{
			{Name: "read", Arguments: map[string]any{"path": "/old_file.go"}},
		}
		editMsg := sdk.NewAssistantMessage("")
		editMsg.ToolCalls = []sdk.ToolCall{
			{Name: "edit", Arguments: map[string]any{"path": "/old_file.go"}},
		}
		initialMsgs := []sdk.Message{
			sdk.NewUserMessage("refactor this"),
			readMsg,
			sdk.NewToolResultMessage("tc1", "read", "contents", false),
			editMsg,
			sdk.NewToolResultMessage("tc2", "edit", "ok", false),
		}
		trackFileOps(initialMsgs, ops)

		assert.True(t, ops.readFiles["/old_file.go"])
		assert.True(t, ops.modifiedFiles["/old_file.go"])

		// Phase 2: compaction replaces old messages with a summary.
		// New messages reference different files. The same ops tracker persists.
		summaryMsg := sdk.NewAssistantMessage("[Compaction Summary]\nPrevious conversation about refactoring old_file.go")
		newReadMsg := sdk.NewAssistantMessage("")
		newReadMsg.ToolCalls = []sdk.ToolCall{
			{Name: "read", Arguments: map[string]any{"path": "/new_file.go"}},
		}
		postCompactMsgs := []sdk.Message{
			summaryMsg,
			sdk.NewUserMessage("now work on new_file.go"),
			newReadMsg,
			sdk.NewToolResultMessage("tc3", "read", "new contents", false),
		}
		trackFileOps(postCompactMsgs, ops)

		// Old file ops from before compaction are still tracked
		assert.True(t, ops.readFiles["/old_file.go"], "old read should persist across compaction")
		assert.True(t, ops.modifiedFiles["/old_file.go"], "old modification should persist across compaction")
		// New file ops are also tracked
		assert.True(t, ops.readFiles["/new_file.go"])
	})
}

func TestFileOpsXML(t *testing.T) {
	t.Run("empty operations", func(t *testing.T) {
		ops := newFileOperations()
		result := fileOpsXML(ops)
		assert.Empty(t, result)
	})

	t.Run("sorted output", func(t *testing.T) {
		ops := newFileOperations()
		ops.readFiles["/z.go"] = true
		ops.readFiles["/a.go"] = true
		ops.readFiles["/m.go"] = true

		result := fileOpsXML(ops)
		idxA := strings.Index(result, "/a.go")
		idxM := strings.Index(result, "/m.go")
		idxZ := strings.Index(result, "/z.go")

		assert.Less(t, idxA, idxM)
		assert.Less(t, idxM, idxZ)
	})
}

func TestAgent_FileOpsTrackingInLoop(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("read", sdk.ToolDef{Name: "read", Description: "read files"}, nil)
	registerMockTool(mt)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "read", Arguments: map[string]any{"path": "/tracked.go"}},
			},
		},
		{textDeltas: []string{"done"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "read file"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	require.NoError(t, a.Close())

	require.NotNil(t, a.fileOps)
	assert.True(t, a.fileOps.readFiles["/tracked.go"])
	assert.Empty(t, a.fileOps.modifiedFiles, "read tool should not populate modifiedFiles")
}

func TestAgent_FileOpsResetOnNewConversation(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("read", sdk.ToolDef{Name: "read", Description: "read files"}, nil)
	registerMockTool(mt)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "read", Arguments: map[string]any{"path": "/first.go"}},
			},
		},
		{textDeltas: []string{"done"}},
		{textDeltas: []string{"new conversation"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "read first"))

	// First conversation: tool call turn + "done" turn = 2 turn_end events
	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for tool call turn_end")
	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for done turn_end")

	require.NotNil(t, a.fileOps)
	assert.True(t, a.fileOps.readFiles["/first.go"])

	b.Publish(sdk.NewEvent(TopicPrompt, "new conversation"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for second conversation turn_end")

	require.NoError(t, a.Close())

	assert.False(t, a.fileOps.readFiles["/first.go"], "file ops should be reset on new conversation")
}

func TestShouldCompact(t *testing.T) {
	t.Run("disabled returns false", func(t *testing.T) {
		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(100000))}
		cfg := CompactionConfig{Enabled: false}
		assert.False(t, shouldCompact(msgs, "", cfg, "test-model"))
	})

	t.Run("empty model under budget returns false", func(t *testing.T) {
		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(100000))}
		cfg := CompactionConfig{Enabled: true}
		assert.False(t, shouldCompact(msgs, "", cfg, ""))
	})

	t.Run("empty model over budget returns true", func(t *testing.T) {
		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(800000))}
		cfg := CompactionConfig{Enabled: true, ReserveTokens: 100}
		assert.True(t, shouldCompact(msgs, "", cfg, ""))
	})

	t.Run("unknown model under budget returns false", func(t *testing.T) {
		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(100000))}
		cfg := CompactionConfig{Enabled: true}
		assert.False(t, shouldCompact(msgs, "", cfg, "nonexistent-model"))
	})

	t.Run("unknown model over budget returns true", func(t *testing.T) {
		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(800000))}
		cfg := CompactionConfig{Enabled: true, ReserveTokens: 100}
		assert.True(t, shouldCompact(msgs, "", cfg, "nonexistent-model"))
	})

	t.Run("under budget returns false", func(t *testing.T) {
		model.ResetModelRegistry()
		defer model.ResetModelRegistry()

		model.RegisterModel(model.ModelDef{
			ID:            "test-model",
			Provider:      "test",
			ContextWindow: 100000,
		})

		msgs := []sdk.Message{sdk.NewUserMessage("short message")}
		cfg := CompactionConfig{Enabled: true, ReserveTokens: 16384}
		assert.False(t, shouldCompact(msgs, "", cfg, "test-model"))
	})

	t.Run("over budget returns true", func(t *testing.T) {
		model.ResetModelRegistry()
		defer model.ResetModelRegistry()

		model.RegisterModel(model.ModelDef{
			ID:            "test-model",
			Provider:      "test",
			ContextWindow: 1000,
		})

		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(4000))}
		cfg := CompactionConfig{Enabled: true, ReserveTokens: 100}
		assert.True(t, shouldCompact(msgs, "", cfg, "test-model"))
	})

	t.Run("system prompt included in total", func(t *testing.T) {
		model.ResetModelRegistry()
		defer model.ResetModelRegistry()

		model.RegisterModel(model.ModelDef{
			ID:            "test-model",
			Provider:      "test",
			ContextWindow: 1000,
		})

		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(2000))} // 500 tokens
		cfg := CompactionConfig{Enabled: true, ReserveTokens: 100}
		assert.False(t, shouldCompact(msgs, "", cfg, "test-model"), "500 tokens should fit in 900 budget")
		// System prompt of 2000 chars = 500 tokens, total = 1000 > 900
		assert.True(t, shouldCompact(msgs, makeLongText(2000), cfg, "test-model"), "with system prompt should exceed")
	})

	t.Run("exact boundary", func(t *testing.T) {
		model.ResetModelRegistry()
		defer model.ResetModelRegistry()

		model.RegisterModel(model.ModelDef{
			ID:            "test-model",
			Provider:      "test",
			ContextWindow: 1000,
		})

		cfg := CompactionConfig{Enabled: true, ReserveTokens: 100}
		// 900 tokens exactly at the boundary should NOT compact.
		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(900 * tokensPerChar))}
		assert.False(t, shouldCompact(msgs, "", cfg, "test-model"),
			"exactly at boundary should not compact")

		// 901 tokens should compact.
		msgs = []sdk.Message{sdk.NewUserMessage(makeLongText(901 * tokensPerChar))}
		assert.True(t, shouldCompact(msgs, "", cfg, "test-model"),
			"one token over boundary should compact")
	})
}

func TestCompact(t *testing.T) {
	t.Run("nothing to compact returns nil", func(t *testing.T) {
		msgs := []sdk.Message{
			sdk.NewUserMessage("short"),
			sdk.NewAssistantMessage("reply"),
		}
		cfg := CompactionConfig{KeepRecentTokens: 100000}
		ops := newFileOperations()

		result, err := compact(
			context.Background(),
			&ProviderMock{},
			msgs,
			cfg,
			"test-model",
			ops,
			"summarize this",
		)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 0, result.summarized)
		assert.Equal(t, msgs, result.messages)
	})

	t.Run("basic compaction with mock provider", func(t *testing.T) {
		// Create enough messages to force a cut
		msgs := make([]sdk.Message, 0, 12)
		for range 10 {
			msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
		}
		// Last 2 messages should be kept
		msgs = append(msgs, sdk.NewUserMessage("keep me"), sdk.NewAssistantMessage("kept reply"))

		mp := newMockProvider([]providerResponse{
			{textDeltas: []string{"## Goal\nTest goal\n\n## Progress\nDid stuff"}},
		})

		cfg := CompactionConfig{KeepRecentTokens: 200}
		ops := newFileOperations()

		result, err := compact(
			context.Background(),
			mp,
			msgs,
			cfg,
			"test-model",
			ops,
			"summarize this",
		)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Positive(t, result.summarized, "should summarize some messages")
		assert.Less(t, result.summarized, len(msgs), "should keep some messages")
		assert.Greater(t, result.tokensBefore, result.tokensAfter, "tokens should decrease after compaction")

		// First message should be the summary
		require.GreaterOrEqual(t, len(result.messages), 3, "should have summary + kept messages")
		assert.Equal(t, sdk.RoleAssistant, result.messages[0].Role)
		content, ok := result.messages[0].Content.(string)
		require.True(t, ok)
		assert.True(t, strings.HasPrefix(content, "[Compaction Summary]\n"))
		assert.Contains(t, content, "Test goal")
		assert.Contains(t, content, "Did stuff")

		// Last kept messages preserved
		lastIdx := len(result.messages) - 1
		assert.Equal(t, sdk.RoleAssistant, result.messages[lastIdx].Role)
		assert.Equal(t, "kept reply", result.messages[lastIdx].Content)

		// Provider was called with summary prompt
		calls := mp.StreamCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "summarize this", calls[0].Req.SystemPrompt)
		assert.Len(t, calls[0].Req.Messages, 1)
		assert.Empty(t, calls[0].Req.Tools)
	})

	t.Run("file operations tracked from summarized messages", func(t *testing.T) {
		readMsg := sdk.NewAssistantMessage("")
		readMsg.ToolCalls = []sdk.ToolCall{
			{Name: "read", Arguments: map[string]any{"path": "/old.go"}},
		}

		// Build messages where the tool call is in the to-summarize portion
		msgs := make([]sdk.Message, 0, 11)

		for i := range 10 {
			if i == 3 {
				msgs = append(msgs, readMsg)
			} else {
				msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
			}
		}

		msgs = append(msgs, sdk.NewUserMessage("keep me"))

		mp := newMockProvider([]providerResponse{
			{textDeltas: []string{"summary"}},
		})

		cfg := CompactionConfig{KeepRecentTokens: 200}
		ops := newFileOperations()

		result, err := compact(
			context.Background(),
			mp,
			msgs,
			cfg,
			"test-model",
			ops,
			"summarize this",
		)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, ops.readFiles["/old.go"], "file ops from summarized messages should be tracked")
	})

	t.Run("previous summary extracted", func(t *testing.T) {
		// First message is an existing summary
		summaryMsg := sdk.NewAssistantMessage("[Compaction Summary]\nOld summary here")

		msgs := make([]sdk.Message, 1, 12)

		msgs[0] = summaryMsg

		for range 10 {
			msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
		}

		msgs = append(msgs, sdk.NewUserMessage("keep me"))

		var capturedReq sdk.ProviderRequest

		mp := &ProviderMock{
			StreamFunc: func(_ context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
				<-chan sdk.ProviderEvent, error,
			) {
				capturedReq = req

				ch := make(chan sdk.ProviderEvent, 1)
				ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "new summary"}

				close(ch)

				return ch, nil
			},
		}

		cfg := CompactionConfig{KeepRecentTokens: 200}
		ops := newFileOperations()

		result, err := compact(
			context.Background(),
			mp,
			msgs,
			cfg,
			"test-model",
			ops,
			"summarize this",
		)
		require.NoError(t, err)
		require.NotNil(t, result)

		// The serialized prompt should contain the previous summary
		assert.Contains(t, capturedReq.Messages[0].Content, "<previous-summary>")
		assert.Contains(t, capturedReq.Messages[0].Content, "Old summary here")
		assert.Contains(t, capturedReq.Messages[0].Content, "</previous-summary>")
	})

	t.Run("provider error returns error", func(t *testing.T) {
		msgs := make([]sdk.Message, 0, 11)
		for range 10 {
			msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
		}

		msgs = append(msgs, sdk.NewUserMessage("keep me"))

		mp := newMockProvider([]providerResponse{
			{err: context.DeadlineExceeded},
		})

		cfg := CompactionConfig{KeepRecentTokens: 200}
		ops := newFileOperations()

		result, err := compact(
			context.Background(),
			mp,
			msgs,
			cfg,
			"test-model",
			ops,
			"summarize this",
		)
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("provider event error returns error", func(t *testing.T) {
		msgs := make([]sdk.Message, 0, 11)
		for range 10 {
			msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
		}

		msgs = append(msgs, sdk.NewUserMessage("keep me"))

		mp := &ProviderMock{
			StreamFunc: func(_ context.Context, _ sdk.ProviderRequest, _ ...model.StreamOption) (
				<-chan sdk.ProviderEvent, error,
			) {
				ch := make(chan sdk.ProviderEvent, 1)
				ch <- sdk.ProviderEvent{Type: sdk.ProviderEventError, Content: "boom"}

				close(ch)

				return ch, nil
			},
		}

		cfg := CompactionConfig{KeepRecentTokens: 200}
		ops := newFileOperations()

		result, err := compact(
			context.Background(),
			mp,
			msgs,
			cfg,
			"test-model",
			ops,
			"summarize this",
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
		assert.Nil(t, result)
	})

	t.Run("custom model option passed to provider", func(t *testing.T) {
		msgs := make([]sdk.Message, 0, 11)
		for range 10 {
			msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
		}

		msgs = append(msgs, sdk.NewUserMessage("keep me"))

		var capturedOpts []model.StreamOption

		mp := &ProviderMock{
			StreamFunc: func(_ context.Context, _ sdk.ProviderRequest, opts ...model.StreamOption) (
				<-chan sdk.ProviderEvent, error,
			) {
				capturedOpts = opts

				ch := make(chan sdk.ProviderEvent, 1)
				ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "summary"}

				close(ch)

				return ch, nil
			},
		}

		cfg := CompactionConfig{
			KeepRecentTokens: 200,
			Model:            "custom-summary-model",
		}
		ops := newFileOperations()

		_, err := compact(
			context.Background(),
			mp,
			msgs,
			cfg,
			"default-model",
			ops,
			"summarize this",
		)
		require.NoError(t, err)

		so := model.NewStreamOptions(capturedOpts...)
		assert.Equal(t, "custom-summary-model", so.Model, "should use config model for summarization")
		assert.Equal(t, model.ThinkingOff, so.ThinkingLevel, "thinking should be off for summarization")
	})

	t.Run("falls back to current model when no custom model", func(t *testing.T) {
		msgs := make([]sdk.Message, 0, 11)
		for range 10 {
			msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
		}

		msgs = append(msgs, sdk.NewUserMessage("keep me"))

		var capturedOpts []model.StreamOption

		mp := &ProviderMock{
			StreamFunc: func(_ context.Context, _ sdk.ProviderRequest, opts ...model.StreamOption) (
				<-chan sdk.ProviderEvent, error,
			) {
				capturedOpts = opts

				ch := make(chan sdk.ProviderEvent, 1)
				ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "summary"}

				close(ch)

				return ch, nil
			},
		}

		cfg := CompactionConfig{KeepRecentTokens: 200}
		ops := newFileOperations()

		_, err := compact(
			context.Background(),
			mp,
			msgs,
			cfg,
			"current-model",
			ops,
			"summarize this",
		)
		require.NoError(t, err)

		so := model.NewStreamOptions(capturedOpts...)
		assert.Equal(t, "current-model", so.Model, "should use current model when no custom model set")
	})

	t.Run("context cancellation propagated to provider", func(t *testing.T) {
		msgs := make([]sdk.Message, 0, 11)
		for range 10 {
			msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
		}

		msgs = append(msgs, sdk.NewUserMessage("keep me"))

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		mp := &ProviderMock{
			StreamFunc: func(ctx context.Context, _ sdk.ProviderRequest, _ ...model.StreamOption) (
				<-chan sdk.ProviderEvent, error,
			) {
				// Respect context cancellation
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}

				ch := make(chan sdk.ProviderEvent, 1)
				ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: "summary"}

				close(ch)

				return ch, nil
			},
		}

		cfg := CompactionConfig{KeepRecentTokens: 200}
		ops := newFileOperations()

		_, err := compact(ctx, mp, msgs, cfg, "test-model", ops, "summarize this")
		assert.Error(t, err)
	})
}

func TestDrainSteeringCompactExtraction(t *testing.T) {
	t.Run("compact without args", func(t *testing.T) {
		steerCh := make(chan sdk.Event, 4)
		steerCh <- sdk.NewEvent(TopicSteer, "compact")

		messages, hasSteering, compactInstr, compactRequested := drainSteering(steerCh, nil)
		assert.False(t, hasSteering, "compact-only steering should not count as new steering")
		assert.True(t, compactRequested)
		assert.Empty(t, compactInstr)
		assert.Nil(t, messages)
	})

	t.Run("compact with custom instructions", func(t *testing.T) {
		steerCh := make(chan sdk.Event, 4)
		steerCh <- sdk.NewEvent(TopicSteer, "compact focus on the auth refactor")

		messages, hasSteering, compactInstr, compactRequested := drainSteering(steerCh, nil)
		assert.False(t, hasSteering, "compact-only steering should not count as new steering")
		assert.True(t, compactRequested)
		assert.Equal(t, "focus on the auth refactor", compactInstr)
		assert.Nil(t, messages)
	})

	t.Run("non-compact steering adds as user message", func(t *testing.T) {
		steerCh := make(chan sdk.Event, 4)
		steerCh <- sdk.NewEvent(TopicSteer, "steer this")

		messages, hasSteering, compactInstr, compactRequested := drainSteering(steerCh, nil)
		assert.True(t, hasSteering)
		assert.False(t, compactRequested)
		assert.Empty(t, compactInstr)
		require.Len(t, messages, 1)
		assert.Equal(t, "steer this", messages[0].Content)
	})

	t.Run("mixed steering and compact", func(t *testing.T) {
		steerCh := make(chan sdk.Event, 4)
		steerCh <- sdk.NewEvent(TopicSteer, "some steering")

		steerCh <- sdk.NewEvent(TopicSteer, "compact focus on auth")

		messages, hasSteering, compactInstr, compactRequested := drainSteering(steerCh, nil)
		assert.True(t, hasSteering)
		assert.True(t, compactRequested)
		assert.Equal(t, "focus on auth", compactInstr)
		require.Len(t, messages, 1)
		assert.Equal(t, "some steering", messages[0].Content)
	})
}

func TestAgent_ManualCompactionViaSteering(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	var capturedReqs []sdk.ProviderRequest

	responses := []providerResponse{
		{textDeltas: []string{"initial response"}},
		{textDeltas: []string{"## Goal\nRefactoring auth\n\n## Progress\nStarted work"}},
		{textDeltas: []string{"after compaction"}},
	}

	var idx atomic.Int32

	mp := &ProviderMock{
		StreamFunc: func(_ context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			capturedReqs = append(capturedReqs, req)
			ch := make(chan sdk.ProviderEvent, 1)

			i := int(idx.Load())
			if i < len(responses) {
				if responses[i].err != nil {
					close(ch)
					return ch, responses[i].err
				}

				for _, d := range responses[i].textDeltas {
					ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: d}
				}

				for _, tc := range responses[i].toolCalls {
					ch <- sdk.ProviderEvent{Type: sdk.ProviderEventToolCall, Content: tc}
				}

				idx.Add(1)
			}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start conversation"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	b.Publish(sdk.NewEvent(TopicSteer, "compact focus on auth refactoring"))

	compactedEvt, ok := waitForTopic(allCh, TopicCompacted, 2*time.Second)
	require.True(t, ok, "timeout waiting for compacted event")
	payload, ok := compactedEvt.Payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, payload["summarized"], "should have summarized the initial messages")

	// Verify compaction occurred: second provider request should be the summary
	// request with the compact prompt as system prompt.
	require.Eventually(t, func() bool {
		return len(capturedReqs) >= 2
	}, 2*time.Second, 100*time.Millisecond, "should have 2 provider requests")

	// Second request is the compaction summary request.
	req := capturedReqs[1]
	assert.Equal(t, "focus on auth refactoring", req.SystemPrompt,
		"compaction request should use custom instructions as system prompt")
	assert.Len(t, req.Messages, 1, "compaction request should have exactly one user message")

	require.NoError(t, a.Close())
}

func TestAgent_ManualCompactionNoArgs(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"initial response"}},
		{textDeltas: []string{"summary of conversation"}},
		{textDeltas: []string{"after compaction"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	b.Publish(sdk.NewEvent(TopicSteer, "compact"))

	compactedEvt, ok := waitForTopic(allCh, TopicCompacted, 2*time.Second)
	require.True(t, ok, "timeout waiting for compacted event")
	_, ok = compactedEvt.Payload.(map[string]any)
	require.True(t, ok)

	require.NoError(t, a.Close())
}

func TestAgent_AutoCompaction(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	model.ResetModelRegistry()
	defer model.ResetModelRegistry()

	// Small context window so accumulated messages trigger auto-compaction
	model.RegisterModel(model.ModelDef{
		ID:            "test-small-model",
		Provider:      "anthropic",
		ContextWindow: 1000,
	})

	var (
		capturedReqs []sdk.ProviderRequest
		mu           sync.Mutex
	)

	responses := []providerResponse{
		{textDeltas: []string{strings.Repeat("x", 4000)}},
		{textDeltas: []string{"compacted summary"}},
		{textDeltas: []string{"final response"}},
	}

	var idx atomic.Int32

	mp := &ProviderMock{
		StreamFunc: func(_ context.Context, req sdk.ProviderRequest, _ ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			mu.Lock()

			capturedReqs = append(capturedReqs, req)
			mu.Unlock()

			ch := make(chan sdk.ProviderEvent, 1)

			i := int(idx.Load())
			if i < len(responses) {
				if responses[i].err != nil {
					close(ch)
					return ch, responses[i].err
				}

				for _, d := range responses[i].textDeltas {
					ch <- sdk.ProviderEvent{Type: sdk.ProviderEventTextDelta, Content: d}
				}

				for _, tc := range responses[i].toolCalls {
					ch <- sdk.ProviderEvent{Type: sdk.ProviderEventToolCall, Content: tc}
				}

				idx.Add(1)
			}

			close(ch)

			return ch, nil
		},
	}
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	a.modelName = "test-small-model"
	a.compactionCfg = CompactionConfig{
		Enabled:          true,
		ReserveTokens:    100,
		KeepRecentTokens: 50,
	}

	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	// First turn: large response builds up message history
	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Follow-up triggers re-entry to inner loop where shouldCompact is checked
	b.Publish(sdk.NewEvent(TopicFollowup, "continue"))

	compactedEvt, ok := waitForTopic(allCh, TopicCompacted, 3*time.Second)
	require.True(t, ok, "timeout waiting for auto compacted event")
	payload, ok := compactedEvt.Payload.(map[string]any)
	require.True(t, ok)
	assert.Positive(t, payload["summarized"])

	// Verify the messages slice was actually replaced by checking the provider
	// request after compaction.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()

		return len(capturedReqs) >= 3
	}, 2*time.Second, 100*time.Millisecond, "should have 3 provider requests")

	// Third request (post-compaction turn) should include the summary message.
	mu.Lock()
	req := capturedReqs[2]
	mu.Unlock()
	require.NotEmpty(t, req.Messages, "request after compaction should have messages")

	if len(req.Messages) > 0 {
		content, ok := req.Messages[0].Content.(string)
		require.True(t, ok, "first message should have string content")
		assert.True(t, strings.HasPrefix(content, compactionSummaryPrefix),
			"first message after compaction should be the summary, got: %s", content)
	}

	require.NoError(t, a.Close())
}

func TestAgent_CompactionErrorInWaitForInputRecovers(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	// Provider: first turn produces large output, second stream call errors
	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{strings.Repeat("y", 4000)}},
		{err: context.DeadlineExceeded},
		{textDeltas: []string{"recovery after error (should not reach)"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Send /compact — the messages are large enough to compact, but the provider errors
	b.Publish(sdk.NewEvent(TopicSteer, "compact"))

	// In waitForInput, compaction errors publish TopicCompacted with error field and continue
	compactedEvt, ok := waitForTopic(allCh, TopicCompacted, 3*time.Second)
	require.True(t, ok, "timeout waiting for compacted event after error")
	payload, ok := compactedEvt.Payload.(map[string]any)
	require.True(t, ok)

	errStr, _ := payload["error"].(string)
	assert.Contains(t, errStr, "compaction stream: context deadline exceeded")

	// Agent should still be alive — send another prompt
	b.Publish(sdk.NewEvent(TopicFollowup, "continue after error"))

	_, ok = waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end after compaction error recovery")

	require.NoError(t, a.Close())
}

func TestAgent_CompactionDisabledNoAutoTrigger(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	model.ResetModelRegistry()
	defer model.ResetModelRegistry()

	model.RegisterModel(model.ModelDef{
		ID:            "test-tiny-model",
		Provider:      "anthropic",
		ContextWindow: 500,
	})

	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{"response"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	a.modelName = "test-tiny-model"
	a.compactionCfg = CompactionConfig{Enabled: false}

	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for turn_end")

	// Verify no compaction occurred by checking provider was called exactly once.
	assert.Len(t, mp.StreamCalls(), 1, "provider should only be called once when compaction is disabled")

	require.NoError(t, a.Close())
}

func TestSerializeForSummary_AssistantWithContentAndToolCalls(t *testing.T) {
	msg := sdk.NewAssistantMessage("I'll run that for you.")
	msg.ToolCalls = []sdk.ToolCall{
		{ID: "tc1", Name: "bash", Arguments: map[string]any{"command": "echo hi"}},
	}
	msgs := []sdk.Message{
		sdk.NewUserMessage("run echo"),
		msg,
		sdk.NewToolResultMessage("tc1", "bash", "hi", false),
	}
	result := serializeForSummary(msgs, "", nil)
	assert.Contains(t, result, "[Tool call]: bash(")
	assert.Contains(t, result, "[Assistant]: I'll run that for you.")

	// Tool calls must appear before assistant content in serialization.
	toolCallIdx := strings.Index(result, "[Tool call]")
	assistantIdx := strings.Index(result, "[Assistant]: I'll run that for you.")
	require.Greater(t, assistantIdx, toolCallIdx,
		"tool calls should be serialized before assistant content")
}

func TestFindCutPoint_ZeroKeepRecentTokens(t *testing.T) {
	msgs := []sdk.Message{
		sdk.NewUserMessage("first"),
		sdk.NewAssistantMessage("reply"),
		sdk.NewUserMessage("second"),
		sdk.NewAssistantMessage("reply2"),
	}
	cut := findCutPoint(msgs, 0)
	assert.Equal(t, 3, cut, "with zero keepRecentTokens, only the last message boundary is kept")
}

func TestCompact_EmptySummaryError(t *testing.T) {
	msgs := make([]sdk.Message, 0, 11)
	for range 10 {
		msgs = append(msgs, sdk.NewUserMessage(makeLongText(400)))
	}

	msgs = append(msgs, sdk.NewUserMessage("keep me"))

	mp := &ProviderMock{
		StreamFunc: func(_ context.Context, _ sdk.ProviderRequest, _ ...model.StreamOption) (
			<-chan sdk.ProviderEvent, error,
		) {
			ch := make(chan sdk.ProviderEvent)
			close(ch) // no events — empty summary

			return ch, nil
		},
	}

	cfg := CompactionConfig{KeepRecentTokens: 200}
	ops := newFileOperations()

	result, err := compact(context.Background(), mp, msgs, cfg, "test-model", ops, "summarize this")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty summary")
	assert.Nil(t, result)
}

func TestAgent_AutoCompactionErrorInInnerLoopRecovers(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	model.ResetModelRegistry()
	defer model.ResetModelRegistry()

	model.RegisterModel(model.ModelDef{
		ID:            "test-recover-model",
		Provider:      "anthropic",
		ContextWindow: 1000,
	})

	// First response is large (triggers auto-compaction on next loop iteration),
	// second stream call (compaction) errors, third is the recovery turn
	mp := newMockProvider([]providerResponse{
		{textDeltas: []string{strings.Repeat("x", 4000)}},
		{err: context.DeadlineExceeded},
		{textDeltas: []string{"recovered"}},
	})
	registerMockProvider("anthropic", mp)

	a, b, cleanup := setupAgent(t, "anthropic")
	a.modelName = "test-recover-model"
	a.compactionCfg = CompactionConfig{
		Enabled:          true,
		ReserveTokens:    100,
		KeepRecentTokens: 50,
	}

	defer cleanup()

	allCh := subscribeAllToChan(b)
	require.NoError(t, a.Subscribe(b))

	b.Publish(sdk.NewEvent(TopicPrompt, "start"))

	_, ok := waitForTopic(allCh, TopicTurnEnd, 2*time.Second)
	require.True(t, ok, "timeout waiting for first turn_end")

	// Follow-up triggers re-entry to inner loop where auto-compaction fires and fails
	b.Publish(sdk.NewEvent(TopicFollowup, "continue"))

	// Compaction error is published as a TopicCompacted event — agent continues
	compactedEvt, ok := waitForTopic(allCh, TopicCompacted, 3*time.Second)
	require.True(t, ok, "timeout waiting for compacted event after auto-compaction error")
	payload, ok := compactedEvt.Payload.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, payload["error"], "compaction")

	// Agent should continue with the turn normally
	msgEnd, ok := waitForTopic(allCh, TopicMsgEnd, 3*time.Second)
	require.True(t, ok, "timeout waiting for msg_end after compaction error recovery")
	msgEndPayload, ok := msgEnd.Payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "recovered", msgEndPayload["content"])
}

func TestTrackFileOps_NonStringFilePathIgnored(t *testing.T) {
	msg := sdk.NewAssistantMessage("")
	msg.ToolCalls = []sdk.ToolCall{
		{Name: "read", Arguments: map[string]any{"path": 123}},
		{Name: "edit", Arguments: map[string]any{"path": []string{"/path.go"}}},
		{Name: "read", Arguments: map[string]any{"other_key": "/path.go"}},
	}
	ops := newFileOperations()
	trackFileOps([]sdk.Message{msg}, ops)
	assert.Empty(t, ops.readFiles, "non-string file_path should be ignored")
	assert.Empty(t, ops.modifiedFiles, "non-string file_path should be ignored")
}

func TestShouldCompact_BudgetLessThanOrEqualZero(t *testing.T) {
	model.ResetModelRegistry()
	defer model.ResetModelRegistry()

	model.RegisterModel(model.ModelDef{
		ID:            "test-model",
		Provider:      "test",
		ContextWindow: 1000,
	})

	// ReserveTokens exceeds context window, so budget = 1000 - 2000 = -1000.
	// Fallback should use contextWindow / 2 = 500.
	cfg := CompactionConfig{Enabled: true, ReserveTokens: 2000}
	msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(2000))} // 500 tokens

	// 500 tokens < 500 budget (not >), so should not compact
	assert.False(t, shouldCompact(msgs, "", cfg, "test-model"),
		"exactly at fallback budget should not compact")

	// 501 tokens > 500 budget, so should compact
	msgs = []sdk.Message{sdk.NewUserMessage(makeLongText(2004))} // 501 tokens
	assert.True(t, shouldCompact(msgs, "", cfg, "test-model"),
		"one token over fallback budget should compact")
}

func TestDefaultCompactPrompt_HasActiveConstraintsSection(t *testing.T) {
	assert.Contains(t, defaultCompactPrompt, "## Active Constraints")
	assert.Contains(t, defaultCompactPrompt, "Preserve ALL user-stated constraints verbatim")
	assert.Contains(t, defaultCompactPrompt, "Do not paraphrase or summarize constraints")
}

func TestDefaultCompactPrompt_HasCurrentPlanSection(t *testing.T) {
	assert.Contains(t, defaultCompactPrompt, "## Current Plan")
	assert.Contains(t, defaultCompactPrompt, "step X of Y")
	assert.Contains(t, defaultCompactPrompt, "completed and remaining steps")
}

func TestDefaultCompactPrompt_HasAllRequiredSections(t *testing.T) {
	requiredSections := []string{
		"## Goal",
		"## Progress",
		"## Active Constraints",
		"## Current Plan",
		"## Key Context",
		"## Recent Tool Activity",
	}
	for _, section := range requiredSections {
		assert.Contains(t, defaultCompactPrompt, section, "default compact prompt missing section: %s", section)
	}
}

func TestDefaultCompactPrompt_ConstraintsInstructionsAreVerbatim(t *testing.T) {
	// Verify the instruction to preserve constraints verbatim appears before the Rules section
	constraintsIdx := strings.Index(defaultCompactPrompt, "Preserve ALL user-stated constraints verbatim")
	rulesIdx := strings.Index(defaultCompactPrompt, "Rules:")
	assert.Greater(t, constraintsIdx, 0, "constraints instruction should be present")
	assert.Greater(t, rulesIdx, 0, "rules section should be present")
	assert.Less(t, constraintsIdx, rulesIdx, "constraints instruction should appear before Rules")
}

func TestDefaultCompactPrompt_PlanInstructionsIncludeStepTracking(t *testing.T) {
	assert.Contains(t, defaultCompactPrompt, "completed steps")
	assert.Contains(t, defaultCompactPrompt, "remaining steps")
	assert.Contains(t, defaultCompactPrompt, "modified, skipped, or added")
}
