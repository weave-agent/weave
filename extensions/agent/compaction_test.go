package agent

import (
	"context"
	"strings"
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
		assert.Equal(t, 0, estimateTokens(msgs))
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
		assert.Equal(t, 0, findCutPoint(msgs, 10000))
	})

	t.Run("cut in middle of conversation", func(t *testing.T) {
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
		}

		cut := findCutPoint(msgs, 300)
		assert.Greater(t, cut, 0, "should find a cut point")
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

		bigMsgs := make([]sdk.Message, 5)
		for i := range bigMsgs {
			bigMsgs[i] = sdk.NewUserMessage(makeLongText(400))
		}

		msgs := append(bigMsgs,
			user, asstWithTool, toolResult, asstText,
		)

		cut := findCutPoint(msgs, 200)

		if cut > 0 && cut < len(msgs) {
			assert.NotEqual(t, sdk.RoleToolResult, msgs[cut].Role,
				"cut point must never be a tool_result message")
		}

		toolResultIdx := len(bigMsgs) + 1
		if cut > 0 && cut <= toolResultIdx {
			asstIdx := len(bigMsgs) + 1
			resultIdx := len(bigMsgs) + 2
			if cut > asstIdx && cut <= resultIdx {
				t.Errorf("cut at %d splits assistant(tool) at %d from result at %d",
					cut, asstIdx, resultIdx)
			}
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
		assert.Contains(t, result, "[Tool result]: hi")
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
			{Name: "read", Arguments: map[string]any{"file_path": "/path/to/file.go"}},
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
			{Name: "edit", Arguments: map[string]any{"file_path": "/path/to/edit.go"}},
		}
		writeMsg := sdk.NewAssistantMessage("")
		writeMsg.ToolCalls = []sdk.ToolCall{
			{Name: "write", Arguments: map[string]any{"file_path": "/path/to/write.go"}},
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
			{Name: "read", Arguments: map[string]any{"file_path": "/first.go"}},
		}
		msg2 := sdk.NewAssistantMessage("")
		msg2.ToolCalls = []sdk.ToolCall{
			{Name: "read", Arguments: map[string]any{"file_path": "/second.go"}},
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
			{Name: "read", Arguments: map[string]any{"file_path": "/old_file.go"}},
		}
		editMsg := sdk.NewAssistantMessage("")
		editMsg.ToolCalls = []sdk.ToolCall{
			{Name: "edit", Arguments: map[string]any{"file_path": "/old_file.go"}},
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
			{Name: "read", Arguments: map[string]any{"file_path": "/new_file.go"}},
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
				{ID: "tc1", Name: "read", Arguments: map[string]any{"file_path": "/tracked.go"}},
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
}

func TestAgent_FileOpsResetOnNewConversation(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	mt := newMockTool("read", sdk.ToolDef{Name: "read", Description: "read files"}, nil)
	registerMockTool(mt)

	mp := newMockProvider([]providerResponse{
		{
			toolCalls: []sdk.ToolCall{
				{ID: "tc1", Name: "read", Arguments: map[string]any{"file_path": "/first.go"}},
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

	t.Run("empty model returns false", func(t *testing.T) {
		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(100000))}
		cfg := CompactionConfig{Enabled: true}
		assert.False(t, shouldCompact(msgs, "", cfg, ""))
	})

	t.Run("unknown model returns false", func(t *testing.T) {
		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(100000))}
		cfg := CompactionConfig{Enabled: true}
		assert.False(t, shouldCompact(msgs, "", cfg, "nonexistent-model"))
	})

	t.Run("under budget returns false", func(t *testing.T) {
		model.ResetModelRegistry()
		defer model.ResetModelRegistry()
		model.RegisterModel(model.ModelDef{
			ID:             "test-model",
			Provider:       "test",
			ContextWindow:  100000,
		})

		msgs := []sdk.Message{sdk.NewUserMessage("short message")}
		cfg := CompactionConfig{Enabled: true, ReserveTokens: 16384}
		assert.False(t, shouldCompact(msgs, "", cfg, "test-model"))
	})

	t.Run("over budget returns true", func(t *testing.T) {
		model.ResetModelRegistry()
		defer model.ResetModelRegistry()
		model.RegisterModel(model.ModelDef{
			ID:             "test-model",
			Provider:       "test",
			ContextWindow:  1000,
		})

		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(4000))}
		cfg := CompactionConfig{Enabled: true, ReserveTokens: 100}
		assert.True(t, shouldCompact(msgs, "", cfg, "test-model"))
	})

	t.Run("system prompt included in total", func(t *testing.T) {
		model.ResetModelRegistry()
		defer model.ResetModelRegistry()
		model.RegisterModel(model.ModelDef{
			ID:             "test-model",
			Provider:       "test",
			ContextWindow:  1000,
		})

		msgs := []sdk.Message{sdk.NewUserMessage(makeLongText(2000))} // 500 tokens
		cfg := CompactionConfig{Enabled: true, ReserveTokens: 100}
		assert.False(t, shouldCompact(msgs, "", cfg, "test-model"), "500 tokens should fit in 900 budget")
		// System prompt of 2000 chars = 500 tokens, total = 1000 > 900
		assert.True(t, shouldCompact(msgs, makeLongText(2000), cfg, "test-model"), "with system prompt should exceed")
	})
}

func TestContextWindowSize(t *testing.T) {
	t.Run("empty model name", func(t *testing.T) {
		assert.Equal(t, 0, contextWindowSize(""))
	})

	t.Run("unknown model", func(t *testing.T) {
		assert.Equal(t, 0, contextWindowSize("nonexistent"))
	})

	t.Run("known model", func(t *testing.T) {
		model.ResetModelRegistry()
		defer model.ResetModelRegistry()
		model.RegisterModel(model.ModelDef{
			ID:             "test-ctx-model",
			Provider:       "test",
			ContextWindow:  50000,
		})
		assert.Equal(t, 50000, contextWindowSize("test-ctx-model"))
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
		assert.Nil(t, result, "should return nil when no cut point found")
	})

	t.Run("basic compaction with mock provider", func(t *testing.T) {
		// Create enough messages to force a cut
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
		}
		// Last 2 messages should be kept
		msgs = append(msgs, sdk.NewUserMessage("keep me"))
		msgs = append(msgs, sdk.NewAssistantMessage("kept reply"))

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

		assert.Greater(t, result.summarized, 0, "should summarize some messages")
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
			{Name: "read", Arguments: map[string]any{"file_path": "/old.go"}},
		}

		// Build messages where the tool call is in the to-summarize portion
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
		}
		msgs[3] = readMsg // read call in old portion
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

		msgs := []sdk.Message{summaryMsg}
		for i := 0; i < 10; i++ {
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
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
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
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("provider event error returns error", func(t *testing.T) {
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
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
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
		assert.Nil(t, result)
	})

	t.Run("custom model option passed to provider", func(t *testing.T) {
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
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
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
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
		msgs := make([]sdk.Message, 10)
		for i := range msgs {
			msgs[i] = sdk.NewUserMessage(makeLongText(400))
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
