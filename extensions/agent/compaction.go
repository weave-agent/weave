package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"weave/sdk"
	"weave/sdk/model"
)

const (
	tokensPerChar = 4
	toolNameRead  = "read"
	toolNameEdit  = "edit"
	toolNameWrite = "write"
	toolNameGrep  = "grep"
	toolNameFind  = "find"
	toolNameLs    = "ls"
)

// estimateTokens returns a rough token count for the given messages using
// the chars/4 heuristic.
func estimateTokens(msgs []sdk.Message) int {
	total := 0

	for _, msg := range msgs {
		total += estimateMessageTokens(msg)
	}

	return total
}

func estimateMessageTokens(msg sdk.Message) int {
	total := 0

	// Content
	total += estimateContentTokens(msg.Content)

	// Tool calls
	for _, tc := range msg.ToolCalls {
		total += len(tc.Name) / tokensPerChar
		if args, err := json.Marshal(tc.Arguments); err == nil {
			total += len(args) / tokensPerChar
		}
	}

	// Signed thinking blocks
	for _, st := range msg.Thinking {
		total += len(st.Thinking) / tokensPerChar
		total += len(st.Signature) / tokensPerChar
	}

	// Redacted thinking blocks
	for _, rt := range msg.RedactedThinking {
		total += len(rt.Data) / tokensPerChar
	}

	return total
}

// findCutPoint determines the index at which to split messages for compaction.
// Messages before the returned index will be summarized; messages from the
// index onward are kept verbatim. It walks backwards accumulating tokens
// until keepRecentTokens is exceeded, then finds the nearest valid boundary
// (user or assistant message without pending tool calls). tool_result
// messages are never valid boundaries — they must stay with their parent
// tool call. Returns 0 if all messages fit within keepRecentTokens.
func findCutPoint(msgs []sdk.Message, keepRecentTokens int) int {
	if len(msgs) == 0 {
		return 0
	}

	acc := 0

	for i, v := range slices.Backward(msgs) {
		acc += estimateMessageTokens(v)

		if acc >= keepRecentTokens {
			cutIdx := findValidBoundary(msgs, i)
			if cutIdx >= len(msgs) {
				// No valid boundary found after startIdx — summarize everything.
				return 0
			}

			return cutIdx
		}
	}

	return 0
}

// findValidBoundary walks forward from startIdx to find the first valid cut
// boundary. A valid boundary is a user message or an assistant message
// without pending tool calls. tool_result messages are never valid boundaries.
func findValidBoundary(msgs []sdk.Message, startIdx int) int {
	for i := startIdx; i < len(msgs); {
		msg := msgs[i]
		switch msg.Role {
		case sdk.RoleUser:
			return i
		case sdk.RoleAssistant:
			if len(msg.ToolCalls) == 0 {
				return i
			}
			// Skip past matching tool_result messages for this assistant's tool calls.
			toolCallIDs := make(map[string]bool, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				toolCallIDs[tc.ID] = true
			}

			j := i + 1
			for ; j < len(msgs); j++ {
				if msgs[j].Role != sdk.RoleToolResult || !toolCallIDs[msgs[j].ToolCallID] {
					break
				}
			}
			// Advance past the assistant and its tool results.
			i = j
		case sdk.RoleToolResult:
			i++
		}
	}

	return len(msgs)
}

func estimateContentTokens(content any) int {
	return len(contentString(content)) / tokensPerChar
}

// fileOperations tracks which files have been read and modified across the
// conversation. It accumulates across compactions — when a summary replaces
// messages, the preserved file lists become the baseline.
type fileOperations struct {
	readFiles     map[string]bool
	modifiedFiles map[string]bool
}

func newFileOperations() *fileOperations {
	return &fileOperations{
		readFiles:     make(map[string]bool),
		modifiedFiles: make(map[string]bool),
	}
}

// trackFileOps scans messages for read/edit/write tool calls and records
// the file paths in the fileOperations tracker.
func trackFileOps(msgs []sdk.Message, ops *fileOperations) {
	for _, msg := range msgs {
		for _, tc := range msg.ToolCalls {
			switch tc.Name {
			case toolNameRead:
				if path, ok := tc.Arguments["path"].(string); ok && path != "" {
					ops.readFiles[path] = true
				}
			case toolNameEdit, toolNameWrite:
				if path, ok := tc.Arguments["path"].(string); ok && path != "" {
					ops.modifiedFiles[path] = true
				}
			}
		}
	}
}

const maxToolResultLen = 2000

// serializeForSummary formats messages into a text representation suitable
// for LLM summarization. It prepends a previous summary if present and
// appends cumulative file operation lists.
func serializeForSummary(msgs []sdk.Message, previousSummary string, ops *fileOperations) string {
	var b strings.Builder

	if previousSummary != "" {
		b.WriteString("<previous-summary>\n")
		b.WriteString(previousSummary)
		b.WriteString("\n</previous-summary>\n\n")
	}

	for _, msg := range msgs {
		switch msg.Role {
		case sdk.RoleUser:
			fmt.Fprintf(&b, "[User]: %s\n", contentString(msg.Content))
		case sdk.RoleAssistant:
			// Skip compaction summary messages — they are already included as <previous-summary>.
			if content := contentString(msg.Content); content != "" && strings.HasPrefix(content, compactionSummaryPrefix) {
				break
			}

			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					args, err := json.Marshal(tc.Arguments)
					if err != nil {
						fmt.Fprintf(&b, "[Tool call]: %s(<marshal error: %v>)\n", tc.Name, err)
					} else {
						fmt.Fprintf(&b, "[Tool call]: %s(%s)\n", tc.Name, string(args))
					}
				}
			}

			if content := contentString(msg.Content); content != "" {
				fmt.Fprintf(&b, "[Assistant]: %s\n", content)
			}
		case sdk.RoleToolResult:
			content := contentString(msg.Content)
			if len(content) > maxToolResultLen {
				content = content[:maxToolResultLen] + "... (truncated)"
			}

			fmt.Fprintf(&b, "[Tool result]: %s\n", content)
		}
	}

	if ops != nil {
		b.WriteString(fileOpsXML(ops))
	}

	return b.String()
}

// fileOpsXML generates <read-files> and <modified-files> XML sections
// from the accumulated file operations.
func fileOpsXML(ops *fileOperations) string {
	var b strings.Builder

	if len(ops.readFiles) > 0 {
		b.WriteString("\n<read-files>\n")

		for _, path := range sortedKeys(ops.readFiles) {
			fmt.Fprintf(&b, "- %s\n", path)
		}

		b.WriteString("</read-files>\n")
	}

	if len(ops.modifiedFiles) > 0 {
		b.WriteString("\n<modified-files>\n")

		for _, path := range sortedKeys(ops.modifiedFiles) {
			fmt.Fprintf(&b, "- %s\n", path)
		}

		b.WriteString("</modified-files>\n")
	}

	return b.String()
}

func contentString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

const compactionSummaryPrefix = "[Compaction Summary]\n"

// shouldCompact checks whether the current token usage exceeds the context
// window budget (context window minus reserve tokens).
func shouldCompact(messages []sdk.Message, systemPrompt string, cfg CompactionConfig, modelName string) bool {
	if !cfg.Enabled {
		return false
	}

	contextWindow := 200000 // Conservative default for unknown models.

	if modelName != "" {
		if m, ok := model.GetModel(modelName); ok && m.ContextWindow > 0 {
			contextWindow = m.ContextWindow
		}
	}

	total := len(systemPrompt)/tokensPerChar + estimateTokens(messages)

	budget := contextWindow - cfg.ReserveTokens
	if budget <= 0 {
		budget = contextWindow / 2
	}

	return total > budget
}

// compactResult holds the data returned by a compaction operation.
type compactResult struct {
	messages     []sdk.Message
	summarized   int
	tokensBefore int
	tokensAfter  int
}

// compact summarizes old messages using the LLM provider and returns a new
// message slice with [summary] + [kept messages]. It tracks file operations
// across compactions by accumulating into the provided ops tracker.
func compact(
	ctx context.Context,
	provider sdk.Provider,
	messages []sdk.Message,
	cfg CompactionConfig,
	modelName string,
	ops *fileOperations,
	compactPrompt string,
) (*compactResult, error) {
	if provider == nil {
		return nil, errors.New("compaction: provider is nil")
	}

	tokensBefore := estimateTokens(messages)

	cutIdx := findCutPoint(messages, cfg.KeepRecentTokens)
	if cutIdx == 0 || cutIdx >= len(messages) {
		return &compactResult{
			messages:     messages,
			summarized:   0,
			tokensBefore: tokensBefore,
			tokensAfter:  tokensBefore,
		}, nil
	}

	toSummarize := messages[:cutIdx]
	kept := messages[cutIdx:]

	// Track file ops in the messages being summarized before they're replaced.
	trackFileOps(toSummarize, ops)

	// Extract any previous summary from the conversation.
	var previousSummary string

	for _, msg := range messages {
		if msg.Role == sdk.RoleAssistant {
			if content, ok := msg.Content.(string); ok && strings.HasPrefix(content, compactionSummaryPrefix) {
				previousSummary = strings.TrimPrefix(content, compactionSummaryPrefix)
				break
			}
		}
	}

	serialized := serializeForSummary(toSummarize, previousSummary, ops)

	// Build a summarization request — no tools, just system prompt + user message.
	req := sdk.ProviderRequest{
		SystemPrompt: compactPrompt,
		Messages:     []sdk.Message{sdk.NewUserMessage(serialized)},
	}

	var opts []model.StreamOption
	if cfg.Model != "" {
		opts = append(opts, model.WithModel(cfg.Model))
	} else if modelName != "" {
		opts = append(opts, model.WithModel(modelName))
	}

	opts = append(opts, model.WithThinkingLevel(model.ThinkingOff))

	ch, err := provider.Stream(ctx, req, opts...)
	if err != nil {
		return nil, fmt.Errorf("compaction stream: %w", err)
	}

	var summary strings.Builder

	for evt := range ch {
		switch evt.Type {
		case sdk.ProviderEventTextDelta:
			if s, ok := evt.Content.(string); ok {
				summary.WriteString(s)
			}
		case sdk.ProviderEventError:
			err := fmt.Errorf("compaction provider error: %v", evt.Content)
			// Drain remaining events to avoid leaking the provider's send goroutine.
			for range ch {
				_ = struct{}{}
			}

			return nil, err
		}
	}

	if summary.Len() == 0 {
		return nil, errors.New("compaction produced empty summary")
	}

	summaryMsg := sdk.NewAssistantMessage(compactionSummaryPrefix + summary.String())

	result := make([]sdk.Message, 0, 1+len(kept))
	result = append(result, summaryMsg)
	result = append(result, kept...)

	return &compactResult{
		messages:     result,
		summarized:   cutIdx,
		tokensBefore: tokensBefore,
		tokensAfter:  estimateTokens(result),
	}, nil
}
