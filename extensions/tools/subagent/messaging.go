package subagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"weave/sdk"
)

// stdoutWriter and stdinReader are swappable for testing.
var (
	stdoutWriter io.Writer = os.Stdout
	stdinReader  io.Reader = os.Stdin
)

func init() {
	sdk.RegisterOutputWriterSetter(SetStdoutWriter)
}

// SetStdoutWriter sets the writer used by inter-agent messaging tools for
// stdout output. Registered via sdk.RegisterOutputWriterSetter so the
// generated main can call it generically without a named import.
func SetStdoutWriter(w io.Writer) {
	stdoutWriter = w
}

// registerMessagingTools registers the child-side inter-agent communication
// tools when running as a subagent with messaging enabled.
// Messaging is enabled when WEAVE_SUBAGENT_ID is set AND WEAVE_MESSAGING is "true".
func isMessagingEnabled() bool {
	v := os.Getenv("WEAVE_MESSAGING")

	return v == "true" || v == "1" || v == "yes"
}

func registerMessagingTools() {
	if os.Getenv("WEAVE_SUBAGENT_ID") == "" {
		return
	}

	if !isMessagingEnabled() {
		return
	}

	sdk.RegisterTool[struct{}]("send_message", func(_ sdk.Config, _ struct{}) (sdk.Tool, error) {
		return &sendMessageTool{}, nil
	})
	sdk.RegisterTool[struct{}]("broadcast_message", func(_ sdk.Config, _ struct{}) (sdk.Tool, error) {
		return &broadcastMessageTool{}, nil
	})
	sdk.RegisterTool[struct{}](msgTypeListAgents, func(_ sdk.Config, _ struct{}) (sdk.Tool, error) {
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

var listAgentsMu sync.Mutex

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
	// Serialize list_agents calls to prevent concurrent executions from
	// racing on the single shared response channel.
	listAgentsMu.Lock()
	defer listAgentsMu.Unlock()

	// Set up response channel first to avoid race with parent's response.
	var respCh chan string
	if sl := getStdinListener(); sl != nil {
		respCh = make(chan string, 1)

		sl.setResponseChannel(respCh)
		defer sl.setResponseChannel(nil)
	}

	msg := brokerMessage{Type: msgTypeListAgents}

	if err := writeBrokerMessage(stdoutWriter, msg); err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Read from stdin until we get a list_agents_response or context is canceled.
	result, err := readListAgentsResponseWithChannel(ctx, stdinReader, respCh)
	if err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil
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

// readListAgentsResponse waits for a list_agents_response message.
// If the stdin listener is running, it uses the listener's response channel.
// Otherwise it scans r directly for the response.
func readListAgentsResponse(ctx context.Context, r io.Reader) (string, error) {
	return readListAgentsResponseWithChannel(ctx, r, nil)
}

// readListAgentsResponseWithChannel waits for a list_agents_response message.
// If respCh is non-nil, it reads from that channel directly. Otherwise, if the
// stdin listener is running, it uses the listener's response channel. If neither,
// it scans r directly for the response.
func readListAgentsResponseWithChannel(ctx context.Context, r io.Reader, respCh chan string) (string, error) {
	if respCh != nil {
		select {
		case result := <-respCh:
			return result, nil
		case <-ctx.Done():
			return "", fmt.Errorf("context canceled: %w", ctx.Err())
		}
	}

	// Prefer the stdin listener if available.
	if sl := getStdinListener(); sl != nil {
		ch := make(chan string, 1)

		sl.setResponseChannel(ch)
		defer sl.setResponseChannel(nil)

		select {
		case result := <-ch:
			return result, nil
		case <-ctx.Done():
			return "", fmt.Errorf("context canceled: %w", ctx.Err())
		}
	}

	// Fallback: scan the reader directly.
	scanner := bufio.NewScanner(r)

	done := make(chan struct{})

	var (
		result  string
		scanErr error
	)

	go func() {
		defer close(done)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

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
		if result == "" && scanErr == nil {
			return "", errors.New("no list_agents_response received")
		}

		return result, scanErr
	case <-ctx.Done():
		return "", fmt.Errorf("context canceled: %w", ctx.Err())
	}
}
