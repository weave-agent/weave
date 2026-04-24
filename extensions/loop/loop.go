package agentloop

//go:generate moq -fmt goimports -stub -skip-ensure -pkg agentloop -out mock_test.go ../../sdk Provider Tool

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"weave/sdk"
)

// Bus topics
const (
	TopicPrompt    = "agent.prompt"
	TopicSteer     = "agent.steer"
	TopicFollowup  = "agent.followup"
	TopicInterrupt = "agent.interrupt"

	TopicTurnStart         = "agent.turn_start"
	TopicTurnEnd           = "agent.turn_end"
	TopicMsgStart          = "agent.message_start"
	TopicMsgUpdate         = "agent.message_update"
	TopicMsgEnd            = "agent.message_end"
	TopicToolResult        = "agent.tool_result"
	TopicEnd               = "agent.end"
	TopicModelChange       = "model.change"
	TopicModelChangeFailed = "model.change_failed"
	TopicThinkingChange    = "thinking.change"
)

// Loop is the agent-loop extension that drives the LLM conversation cycle.
type Loop struct {
	cfg          sdk.Config
	providerName string
	modelName    string
	singleTurn   bool

	thinkingLevel sdk.ThinkingLevel

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func init() { //nolint:gochecknoinits // required for extension self-registration
	sdk.RegisterExtension("loop", func(cfg sdk.Config) (sdk.Extension, error) {
		provider := os.Getenv("WEAVE_PROVIDER")
		if provider == "" {
			provider = "anthropic"
		}

		return NewLoop(cfg, provider)
	})
}

func NewLoop(cfg sdk.Config, providerName string) (*Loop, error) {
	return &Loop{
		cfg:           cfg,
		providerName:  providerName,
		singleTurn:    os.Getenv("WEAVE_SINGLE_TURN") == "1",
		thinkingLevel: sdk.ThinkingMedium,
	}, nil
}

func (l *Loop) Name() string { return "loop" }

func (l *Loop) Subscribe(bus sdk.Bus) {
	l.mu.Lock()
	if l.cancel != nil {
		l.mu.Unlock()
		panic("loop: Subscribe called twice without Close")
	}

	promptCh := bus.Subscribe(TopicPrompt)
	steerCh := bus.Subscribe(TopicSteer)
	followupCh := bus.Subscribe(TopicFollowup)
	interruptCh := bus.Subscribe(TopicInterrupt)
	modelChangeCh := bus.Subscribe(TopicModelChange)
	thinkingCh := bus.Subscribe(TopicThinkingChange)

	ctx, cancel := context.WithCancel(context.Background())

	l.cancel = cancel
	l.done = make(chan struct{})
	l.mu.Unlock()

	go l.run(ctx, bus, promptCh, steerCh, followupCh, interruptCh, modelChangeCh, thinkingCh)
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

//nolint:gocyclo // central event loop with multiple channel selects
func (l *Loop) run(ctx context.Context, bus sdk.Bus, promptCh, steerCh, followupCh, interruptCh, modelChangeCh, thinkingCh <-chan sdk.Event) {
	defer close(l.done)

	var endPayload any

	defer func() { bus.Publish(sdk.NewEvent(TopicEnd, endPayload)) }()

	provider, err := sdk.GetProvider(l.providerName, l.cfg)
	if err != nil {
		endPayload = fmt.Sprintf("provider error: %v", err)
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

	turn := 1

	// Outer loop: follow-ups. The waitForInput label is used by model/thinking
	// change handlers to skip streamTurn and go directly back to waiting.
	for {
		// Per-turn context that can be canceled by interrupt without
		// killing the entire session.
		turnCtx, turnCancel := context.WithCancel(ctx)
		turnDone := make(chan struct{})

		go func() {
			defer close(turnDone)

			select {
			case <-interruptCh:
				turnCancel()
			case <-turnCtx.Done():
			}
		}()

		// Inner loop: tool calls. Continues while the provider returns
		// tool calls that need execution.
		continueLoop := true

		for continueLoop {
			messages, _ = drainSteering(steerCh, messages)

			bus.Publish(sdk.NewEvent(TopicTurnStart, turn))

			opts := l.streamOpts()

			resp, toolCalls, err := streamTurn(turnCtx, bus, provider, messages, toolDefs, opts...)
			if err != nil {
				bus.Publish(sdk.NewEvent(TopicTurnEnd, nil))

				// If the turn was interrupted (not the main context), break to follow-up.
				if turnCtx.Err() != nil && ctx.Err() == nil {
					break
				}

				endPayload = fmt.Sprintf("stream error: %v", err)

				turnCancel()
				<-turnDone

				return
			}

			messages = append(messages, resp)

			for _, tc := range toolCalls {
				result, err := executeTool(turnCtx, l.cfg, tc)
				if err != nil {
					result = sdk.ToolResult{Content: err.Error(), IsError: true}
				}

				bus.Publish(sdk.NewEvent(TopicToolResult, map[string]any{
					"id":     tc.ID,
					"tool":   tc.Name,
					"result": result,
				}))

				messages = append(messages, sdk.NewToolResultMessage(tc.ID, tc.Name, result.Content, result.IsError))
			}

			bus.Publish(sdk.NewEvent(TopicTurnEnd, nil))

			var hasNewSteering bool

			messages, hasNewSteering = drainSteering(steerCh, messages)
			continueLoop = len(toolCalls) > 0 || hasNewSteering
		}

		turnCancel()
		<-turnDone

		drainInterrupts(interruptCh)

		turn++

		if l.singleTurn {
			return
		}

	waitForInput:
		// Wait for follow-up or new prompt. Blocking — the loop stays alive
		// between turns. A new agent.prompt resets the conversation (e.g. /new).
		select {
		case evt, ok := <-followupCh:
			if !ok {
				return
			}

			provider = l.drainChanges(modelChangeCh, thinkingCh, bus, provider)

			messages = append(messages, sdk.NewUserMessage(evt.Payload))
		case evt, ok := <-promptCh:
			if !ok {
				return
			}

			provider = l.drainChanges(modelChangeCh, thinkingCh, bus, provider)
			messages = []sdk.Message{sdk.NewUserMessage(evt.Payload)}
			turn = 1
		case evt, ok := <-modelChangeCh:
			if !ok {
				return
			}

			provider = l.applyModelChange(evt, bus, provider)
			provider = l.drainChanges(modelChangeCh, thinkingCh, bus, provider)

			goto waitForInput
		case evt, ok := <-thinkingCh:
			if !ok {
				return
			}

			l.applyThinkingChange(evt)
			provider = l.drainChanges(modelChangeCh, thinkingCh, bus, provider)

			goto waitForInput
		case <-ctx.Done():
			return
		}
	}
}

func drainInterrupts(ch <-chan sdk.Event) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func (l *Loop) applyModelChange(evt sdk.Event, bus sdk.Bus, currentProv sdk.Provider) sdk.Provider {
	m, ok := evt.Payload.(map[string]string)
	if !ok {
		return currentProv
	}

	provider := m["provider"]
	model := m["model"]

	if provider != "" && provider != l.providerName {
		newProv, err := sdk.GetProvider(provider, l.cfg)
		if err != nil {
			bus.Publish(sdk.NewEvent(TopicModelChangeFailed, map[string]any{
				"provider": l.providerName,
				"error":    err.Error(),
			}))

			return currentProv
		}

		l.providerName = provider
		currentProv = newProv
	}

	if model != "" {
		l.modelName = model
	}

	return currentProv
}

func (l *Loop) applyThinkingChange(evt sdk.Event) {
	m, ok := evt.Payload.(map[string]string)
	if !ok {
		return
	}

	if level, ok := m["level"]; ok {
		if parsed, err := sdk.ParseThinkingLevel(level); err == nil {
			l.thinkingLevel = parsed
		}
	}
}

func (l *Loop) drainChanges(modelChangeCh, thinkingCh <-chan sdk.Event, bus sdk.Bus, currentProv sdk.Provider) sdk.Provider {
	for {
		select {
		case evt, ok := <-modelChangeCh:
			if !ok {
				return currentProv
			}

			currentProv = l.applyModelChange(evt, bus, currentProv)
		case evt, ok := <-thinkingCh:
			if !ok {
				return currentProv
			}

			l.applyThinkingChange(evt)
		default:
			return currentProv
		}
	}
}

func (l *Loop) streamOpts() []sdk.StreamOption {
	level := l.thinkingLevel
	if level != sdk.ThinkingOff {
		if modelDef, ok := sdk.GetModel(l.modelName); ok {
			if !modelDef.Reasoning {
				level = sdk.ThinkingOff
			} else {
				level = sdk.ClampForModel(level, modelDef)
			}
		}
	}

	opts := []sdk.StreamOption{
		sdk.WithThinkingLevel(level),
	}

	if l.modelName != "" {
		opts = append(opts, sdk.WithModel(l.modelName))
	}

	return opts
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

func streamTurn(ctx context.Context, bus sdk.Bus, provider sdk.Provider, messages []sdk.Message, tools []sdk.ToolDef, opts ...sdk.StreamOption) (sdk.Message, []sdk.ToolCall, error) {
	req := sdk.ProviderRequest{
		Messages: messages,
		Tools:    tools,
	}

	ch, err := provider.Stream(ctx, req, opts...)
	if err != nil {
		return sdk.Message{}, nil, fmt.Errorf("provider stream: %w", err)
	}

	bus.Publish(sdk.NewEvent(TopicMsgStart, nil))

	var content strings.Builder

	var thinking strings.Builder

	var toolCalls []sdk.ToolCall

	for evt := range ch {
		switch evt.Type {
		case sdk.ProviderEventTextDelta:
			bus.Publish(sdk.NewEvent(TopicMsgUpdate, evt.Content))

			if s, ok := evt.Content.(string); ok {
				content.WriteString(s)
			}
		case sdk.ProviderEventThinking:
			if s, ok := evt.Content.(string); ok {
				thinking.WriteString(s)
			}
		case sdk.ProviderEventToolCall:
			if tc, ok := evt.Content.(sdk.ToolCall); ok {
				toolCalls = append(toolCalls, tc)
			}
		case sdk.ProviderEventError:
			bus.Publish(sdk.NewEvent(TopicMsgEnd, map[string]any{"content": content.String(), "tool_calls": toolCalls}))
			return sdk.Message{}, nil, fmt.Errorf("provider error: %v", evt.Content)
		}
	}

	msgEndPayload := map[string]any{"content": content.String(), "tool_calls": toolCalls}
	if thinking.Len() > 0 {
		msgEndPayload["thinking"] = thinking.String()
	}

	bus.Publish(sdk.NewEvent(TopicMsgEnd, msgEndPayload))

	resp := sdk.NewAssistantMessage(content.String())
	resp.ToolCalls = toolCalls

	return resp, toolCalls, nil
}

func executeTool(ctx context.Context, cfg sdk.Config, tc sdk.ToolCall) (sdk.ToolResult, error) {
	tool, err := sdk.GetTool(tc.Name, cfg)
	if err != nil {
		return sdk.ToolResult{}, fmt.Errorf("tool %q not found: %w", tc.Name, err)
	}

	result, err := tool.Execute(ctx, tc.Arguments)
	if err != nil {
		return sdk.ToolResult{}, fmt.Errorf("tool %q execute: %w", tc.Name, err)
	}

	return result, nil
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
