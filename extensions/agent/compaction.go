package agent

import (
	"encoding/json"
	"fmt"

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
