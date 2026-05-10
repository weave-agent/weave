package subagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"weave/sdk"
)

// stdoutWriter and stdinReader are swappable for testing.
var (
	stdoutWriter io.Writer = os.Stdout
	stdinReader  io.Reader = os.Stdin
)

// registerMessagingTools registers the child-side inter-agent communication
// tools when running as a subagent (indicated by WEAVE_SUBAGENT_ID env var).
func registerMessagingTools() {
	if os.Getenv("WEAVE_SUBAGENT_ID") == "" {
		return
	}

	sdk.RegisterTool("send_message", func(sdk.Config) (sdk.Tool, error) {
		return &sendMessageTool{}, nil
	})
	sdk.RegisterTool("broadcast_message", func(sdk.Config) (sdk.Tool, error) {
		return &broadcastMessageTool{}, nil
	})
	sdk.RegisterTool(msgTypeListAgents, func(sdk.Config) (sdk.Tool, error) {
		return &listAgentsTool{}, nil
	})
}

// sendMessageTool implements the send_message inter-agent tool.
type sendMessageTool struct{}

func (t *sendMessageTool) Name() string { return "send_message" }

func (t *sendMessageTool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "send_message",
		Description: "Send a message to another running agent",
		Parameters: map[string]any{
			jsonType: "object",
			"properties": map[string]any{
				keyTo: map[string]any{
					jsonType:        jsonString,
					propDescription: "Target agent ID",
				},
				keyContent: map[string]any{
					jsonType:        jsonString,
					propDescription: "Message content",
				},
			},
			"required": []string{keyTo, keyContent},
		},
	}
}

func (t *sendMessageTool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	toVal, ok := args[keyTo]
	if !ok {
		return sdk.ToolResult{Content: "missing required parameter: to", IsError: true}, nil
	}

	to, ok := toVal.(string)
	if !ok || to == "" {
		return sdk.ToolResult{Content: "to must be a non-empty string", IsError: true}, nil
	}

	contentVal, ok := args[keyContent]
	if !ok {
		return sdk.ToolResult{Content: "missing required parameter: content", IsError: true}, nil
	}

	content, ok := contentVal.(string)
	if !ok {
		return sdk.ToolResult{Content: "content must be a string", IsError: true}, nil
	}

	msg := brokerMessage{
		Type:    msgTypeSend,
		To:      to,
		Content: content,
	}

	if err := writeBrokerMessage(stdoutWriter, msg); err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil //nolint:nilerr // tool protocol
	}

	return sdk.ToolResult{Content: "Message sent to " + to}, nil
}

// broadcastMessageTool implements the broadcast_message inter-agent tool.
type broadcastMessageTool struct{}

func (t *broadcastMessageTool) Name() string { return "broadcast_message" }

func (t *broadcastMessageTool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "broadcast_message",
		Description: "Send a message to all other running agents",
		Parameters: map[string]any{
			jsonType: "object",
			"properties": map[string]any{
				keyContent: map[string]any{
					jsonType:        jsonString,
					propDescription: "Message content",
				},
			},
			"required": []string{keyContent},
		},
	}
}

func (t *broadcastMessageTool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	contentVal, ok := args[keyContent]
	if !ok {
		return sdk.ToolResult{Content: "missing required parameter: content", IsError: true}, nil
	}

	content, ok := contentVal.(string)
	if !ok {
		return sdk.ToolResult{Content: "content must be a string", IsError: true}, nil
	}

	msg := brokerMessage{
		Type:    msgTypeBroadcast,
		Content: content,
	}

	if err := writeBrokerMessage(stdoutWriter, msg); err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil //nolint:nilerr // tool protocol
	}

	return sdk.ToolResult{Content: "Message broadcast to all agents"}, nil
}

// listAgentsTool implements the list_agents inter-agent tool.
type listAgentsTool struct{}

func (t *listAgentsTool) Name() string { return msgTypeListAgents }

func (t *listAgentsTool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        msgTypeListAgents,
		Description: "List all currently running agents and their IDs",
		Parameters: map[string]any{
			jsonType:     "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
	}
}

func (t *listAgentsTool) Execute(ctx context.Context, _ map[string]any) (sdk.ToolResult, error) {
	msg := brokerMessage{Type: msgTypeListAgents}

	if err := writeBrokerMessage(stdoutWriter, msg); err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil //nolint:nilerr // tool protocol
	}

	// Read from stdin until we get a list_agents_response or context is canceled.
	result, err := readListAgentsResponse(ctx, stdinReader)
	if err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil //nolint:nilerr // tool protocol
	}

	return sdk.ToolResult{Content: result}, nil
}

// writeBrokerMessage marshals a brokerMessage and writes it as a JSON line to w.
func writeBrokerMessage(w io.Writer, msg brokerMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if _, err = fmt.Fprintln(w, string(data)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

// readListAgentsResponse scans r for JSON lines and returns the content of the
// first list_agents_response message. Non-JSON lines and other message types
// are skipped. Returns an error if the context is canceled before a response
// is received.
func readListAgentsResponse(ctx context.Context, r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)

	done := make(chan struct{})

	var (
		result  string
		scanErr error
	)

	go func() {
		defer close(done)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var msg brokerMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}

			if msg.Type == msgTypeListAgentsResp {
				result = formatRoster(msg.Agents)

				return
			}
		}

		if err := scanner.Err(); err != nil {
			scanErr = fmt.Errorf("read stdin: %w", err)
		}
	}()

	select {
	case <-done:
		return result, scanErr
	case <-ctx.Done():
		return "", fmt.Errorf("context canceled: %w", ctx.Err())
	}
}
