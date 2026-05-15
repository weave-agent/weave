package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"weave/sdk"
)

const (
	tokensPerChar      = 4
	imageTokenEstimate = 1200
)

// estimateTokens returns a rough token count for the given messages using
// the chars/4 heuristic. Each image is estimated at 1200 tokens.
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
	cutIdx := 0

	for i := len(msgs) - 1; i >= 0; i-- {
		acc += estimateMessageTokens(msgs[i])

		if acc >= keepRecentTokens {
			// We've accumulated enough tokens — find the next valid boundary
			// at or after the current position.
			cutIdx = findValidBoundary(msgs, i)
			break
		}
	}

	return cutIdx
}

// findValidBoundary walks forward from startIdx to find the first valid cut
// boundary. A valid boundary is a user message or an assistant message
// without pending tool calls. tool_result messages are never valid boundaries.
func findValidBoundary(msgs []sdk.Message, startIdx int) int {
	for i := startIdx; i < len(msgs); i++ {
		msg := msgs[i]
		switch msg.Role {
		case sdk.RoleUser:
			return i
		case sdk.RoleAssistant:
			// Only valid if there are no tool calls that need results following
			if len(msg.ToolCalls) == 0 {
				return i
			}
			// Assistant with tool calls — the cut must include all following
			// tool_result messages, so skip past them.
			toolCallIDs := make(map[string]bool, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				toolCallIDs[tc.ID] = true
			}
			for j := i + 1; j < len(msgs); j++ {
				if msgs[j].Role == sdk.RoleToolResult && toolCallIDs[msgs[j].ToolCallID] {
					i = j // skip past matching tool results
				}
			}
			// After the tool results, the next message is a valid boundary
			if i+1 < len(msgs) {
				// Recurse from the next position
				return findValidBoundary(msgs, i+1)
			}
			return len(msgs) // can't split — everything must be kept
		case sdk.RoleToolResult:
			// Skip — will be handled by the assistant tool-call case above
			continue
		}
	}

	return len(msgs)
}

func estimateContentTokens(content any) int {
	switch v := content.(type) {
	case string:
		return len(v) / tokensPerChar
	case []byte:
		return len(v) / tokensPerChar
	case fmt.Stringer:
		return len(v.String()) / tokensPerChar
	case nil:
		return 0
	default:
		return len(fmt.Sprint(v)) / tokensPerChar
	}
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
			case "read":
				if path, ok := tc.Arguments["file_path"]; ok {
					ops.readFiles[fmt.Sprint(path)] = true
				}
			case "edit", "write":
				if path, ok := tc.Arguments["file_path"]; ok {
					ops.modifiedFiles[fmt.Sprint(path)] = true
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
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					args, _ := json.Marshal(tc.Arguments)
					fmt.Fprintf(&b, "[Tool call]: %s(%s)\n", tc.Name, string(args))
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
