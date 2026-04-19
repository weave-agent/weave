package agentloop

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"weave/sdk"
)

// Bus topics
const (
	TopicPrompt   = "agent.prompt"
	TopicSteer    = "agent.steer"
	TopicFollowup = "agent.followup"

	TopicTurnStart  = "agent.turn_start"
	TopicTurnEnd    = "agent.turn_end"
	TopicMsgStart   = "agent.message_start"
	TopicMsgUpdate  = "agent.message_update"
	TopicMsgEnd     = "agent.message_end"
	TopicToolResult = "agent.tool_result"
	TopicEnd        = "agent.end"
)

// ToolCall represents a parsed tool call from the provider response.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// Loop is the agent-loop extension that drives the LLM conversation cycle.
type Loop struct {
	cfg          sdk.Config
	providerName string

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func init() {
	sdk.RegisterExtension("loop", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewLoop(cfg, "anthropic")
	})
}

func NewLoop(cfg sdk.Config, providerName string) (*Loop, error) {
	return &Loop{
		cfg:          cfg,
		providerName: providerName,
	}, nil
}

func (l *Loop) Name() string { return "loop" }

func (l *Loop) Subscribe(bus sdk.Bus) {
	promptCh := bus.Subscribe(TopicPrompt)
	steerCh := bus.Subscribe(TopicSteer)
	followupCh := bus.Subscribe(TopicFollowup)

	ctx, cancel := context.WithCancel(context.Background())

	l.mu.Lock()
	l.cancel = cancel
	l.done = make(chan struct{})
	l.mu.Unlock()

	go l.run(ctx, bus, promptCh, steerCh, followupCh)
}

func (l *Loop) Close() error {
	l.mu.Lock()
	cancel := l.cancel
	done := l.done
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}

	return nil
}

func (l *Loop) run(ctx context.Context, bus sdk.Bus, promptCh, steerCh, followupCh <-chan sdk.Event) {
	defer close(l.done)
	defer bus.Publish(sdk.NewEvent(TopicEnd, nil))

	provider, err := sdk.GetProvider(l.providerName, l.cfg)
	if err != nil {
		bus.Publish(sdk.NewEvent(TopicEnd, fmt.Sprintf("provider error: %v", err)))
		return
	}

	toolDefs := collectToolDefs(l.cfg)

	var messages []sdk.Message

	select {
	case evt, ok := <-promptCh:
		if !ok {
			return
		}
		messages = append(messages, sdk.NewUserMessage(evt.Payload))
	case <-ctx.Done():
		return
	}

	// Outer loop: follow-ups
	for {
		// Inner loop: tool calls + steering. Continues while there are
		// tool calls or pending steering messages that require a new turn.
		continueLoop := true

		for continueLoop {
			messages, continueLoop = drainSteering(steerCh, messages)

			bus.Publish(sdk.NewEvent(TopicTurnStart, len(messages)))

			resp, toolCalls, err := streamTurn(ctx, bus, provider, messages, toolDefs)
			if err != nil {
				bus.Publish(sdk.NewEvent(TopicEnd, fmt.Sprintf("stream error: %v", err)))
				return
			}

			messages = append(messages, resp)
			bus.Publish(sdk.NewEvent(TopicTurnEnd, nil))

			if len(toolCalls) > 0 {
				continueLoop = true
			}

			for _, tc := range toolCalls {
				result, err := executeTool(ctx, l.cfg, tc)
				if err != nil {
					result = sdk.ToolResult{Content: err.Error(), IsError: true}
				}

				bus.Publish(sdk.NewEvent(TopicToolResult, map[string]any{
					"tool":   tc.Name,
					"result": result,
				}))

				messages = append(messages, sdk.NewToolResultMessage(tc.ID, tc.Name, result.Content))
			}
		}

		// Check for follow-up — non-blocking drain. If a follow-up was
		// published while the inner loop was running, continue the outer loop.
		select {
		case evt, ok := <-followupCh:
			if !ok {
				return
			}
			messages = append(messages, sdk.NewUserMessage(evt.Payload))
		case <-ctx.Done():
			return
		default:
			return
		}
	}
}

func drainSteering(steerCh <-chan sdk.Event, messages []sdk.Message) ([]sdk.Message, bool) {
	hasSteering := false
	for {
		select {
		case evt, ok := <-steerCh:
			if !ok {
				return messages, hasSteering
			}
			messages = append(messages, sdk.NewUserMessage(evt.Payload))
			hasSteering = true
		default:
			return messages, hasSteering
		}
	}
}

func streamTurn(ctx context.Context, bus sdk.Bus, provider sdk.Provider, messages []sdk.Message, tools []sdk.ToolDef) (sdk.Message, []ToolCall, error) {
	req := sdk.ProviderRequest{
		Messages: messages,
		Tools:    tools,
	}

	ch, err := provider.Stream(ctx, req)
	if err != nil {
		return sdk.Message{}, nil, fmt.Errorf("provider stream: %w", err)
	}

	bus.Publish(sdk.NewEvent(TopicMsgStart, nil))

	var content strings.Builder
	var toolCalls []ToolCall

	for evt := range ch {
		switch evt.Type {
		case sdk.ProviderEventTextDelta:
			bus.Publish(sdk.NewEvent(TopicMsgUpdate, evt.Content))
			if s, ok := evt.Content.(string); ok {
				content.WriteString(s)
			}
		case sdk.ProviderEventToolCall:
			if tc, ok := evt.Content.(ToolCall); ok {
				toolCalls = append(toolCalls, tc)
			}
		case sdk.ProviderEventError:
			return sdk.Message{}, nil, fmt.Errorf("provider error: %v", evt.Content)
		}
	}

	bus.Publish(sdk.NewEvent(TopicMsgEnd, content.String()))

	return sdk.NewAssistantMessage(content.String()), toolCalls, nil
}

func executeTool(ctx context.Context, cfg sdk.Config, tc ToolCall) (sdk.ToolResult, error) {
	tool, err := sdk.GetTool(tc.Name, cfg)
	if err != nil {
		return sdk.ToolResult{}, fmt.Errorf("tool %q not found: %w", tc.Name, err)
	}

	return tool.Execute(ctx, tc.Arguments)
}

func collectToolDefs(cfg sdk.Config) []sdk.ToolDef {
	names := sdk.ListTools()
	defs := make([]sdk.ToolDef, 0, len(names))
	for _, name := range names {
		tool, err := sdk.GetTool(name, cfg)
		if err != nil {
			continue
		}
		defs = append(defs, tool.Definition())
	}
	return defs
}
