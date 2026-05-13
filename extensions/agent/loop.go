package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"weave/sdk"
	"weave/sdk/model"
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
	TopicToolCall          = "agent.tool_call"
	TopicToolResult        = "agent.tool_result"
	TopicEnd               = "agent.end"
	TopicModelChange       = "model.change"
	TopicModelChangeFailed = "model.change_failed"
	TopicThinkingChange    = "thinking.change"
)

//nolint:gocyclo // central event loop with multiple channel selects
func (a *AgentExtension) run(
	ctx context.Context,
	bus sdk.Bus,
	promptCh, steerCh, followupCh, interruptCh, modelChangeCh, thinkingCh <-chan sdk.Event,
) {
	defer close(a.done)

	var endPayload any

	defer func() { bus.Publish(sdk.NewEvent(TopicEnd, endPayload)) }()

	toolDefs := collectToolDefs(a.cfg)

	var messages []sdk.Message

	// Build the system prompt once from discovered files and skills.
	systemPrompt := a.buildSystemPrompt()

	// Wait for the first prompt before instantiating the provider.
	// This gives the TUI time to show "No providers configured" and let the
	// user set an API key via /providers before we try to connect.
	// Also drain model/thinking changes from the TUI's Init() so the
	// initial provider matches what the user sees.
	for {
		select {
		case evt, ok := <-promptCh:
			if !ok {
				return
			}

			messages = append(messages, sdk.NewUserMessage(evt.Payload))
		case evt := <-modelChangeCh:
			if m, ok := evt.Payload.(map[string]string); ok {
				if p := m["provider"]; p != "" {
					a.providerName = p
				}

				if modelName := m["model"]; modelName != "" {
					a.modelName = modelName
				}
			}

			continue
		case evt := <-thinkingCh:
			if m, ok := evt.Payload.(map[string]string); ok {
				if lvl, err := model.ParseThinkingLevel(m["level"]); err == nil {
					a.thinkingLevel = lvl
				}
			}

			continue
		case <-ctx.Done():
			return
		}

		break
	}

	// Drain initial model/thinking changes before instantiating the provider.
	provider := a.drainChanges(modelChangeCh, thinkingCh, bus, nil)
	if provider == nil {
		var err error

		provider, err = sdk.GetProvider(a.providerName, a.cfg)
		if err != nil {
			msg := err.Error()

			if !anyProviderHasKey() {
				msg = "No providers configured. Set an API key via /providers or the environment variable."
			}

			endPayload = msg

			return
		}
	}

	provider = a.drainChanges(modelChangeCh, thinkingCh, bus, provider)

	turn := 1

	// Outer loop: follow-ups.
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

			opts := a.streamOpts()

			resp, toolCalls, err := streamTurn(turnCtx, bus, provider, messages, toolDefs, systemPrompt, opts...)
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
				bus.Publish(sdk.NewEvent(TopicToolCall, map[string]any{
					"id":   tc.ID,
					"tool": tc.Name,
					"args": tc.Arguments,
				}))

				result, err := executeTool(turnCtx, a.cfg, tc)
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

		if a.singleTurn {
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

			provider = a.drainChanges(modelChangeCh, thinkingCh, bus, provider)

			messages = append(messages, sdk.NewUserMessage(evt.Payload))
		case evt, ok := <-promptCh:
			if !ok {
				return
			}

			provider = a.drainChanges(modelChangeCh, thinkingCh, bus, provider)
			messages = []sdk.Message{sdk.NewUserMessage(evt.Payload)}
			turn = 1
			// Rebuild system prompt on new conversation
			systemPrompt = a.buildSystemPrompt()
		case evt, ok := <-modelChangeCh:
			if !ok {
				return
			}

			provider = a.applyModelChange(evt, bus, provider)
			provider = a.drainChanges(modelChangeCh, thinkingCh, bus, provider)

			goto waitForInput
		case evt, ok := <-thinkingCh:
			if !ok {
				return
			}

			a.applyThinkingChange(evt)
			provider = a.drainChanges(modelChangeCh, thinkingCh, bus, provider)

			goto waitForInput
		case <-ctx.Done():
			return
		}
	}
}

// buildSystemPrompt discovers context files, skills, and system prompts from disk
// and assembles the full system prompt using the prompt builder.
func (a *AgentExtension) buildSystemPrompt() string {
	projectDir := a.projectDir()
	globalDir := globalConfigDir()

	// Discover context files
	contextFiles := discoverContextFiles(projectDir, globalDir)

	// Load system prompts
	systemBase, systemAppend := loadSystemPrompt(projectDir, globalDir)

	skills, _ := discoverSkills(a.resolveSkillPaths()...)

	pb := newPromptBuilder(a.cfg)

	return pb.Build(buildInput{
		contextFiles: contextFiles,
		systemBase:   systemBase,
		systemAppend: systemAppend,
		skills:       skills,
	})
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

func (a *AgentExtension) applyModelChange(evt sdk.Event, bus sdk.Bus, currentProv sdk.Provider) sdk.Provider {
	m, ok := evt.Payload.(map[string]string)
	if !ok {
		return currentProv
	}

	provider := m["provider"]
	modelName := m["model"]

	if provider != "" && provider != a.providerName {
		newProv, err := sdk.GetProvider(provider, a.cfg)
		if err != nil {
			bus.Publish(sdk.NewEvent(TopicModelChangeFailed, map[string]any{
				"provider": provider,
				"error":    err.Error(),
			}))

			return currentProv
		}

		a.providerName = provider
		currentProv = newProv
	}

	if modelName != "" {
		a.modelName = modelName
	}

	return currentProv
}

func (a *AgentExtension) applyThinkingChange(evt sdk.Event) {
	m, ok := evt.Payload.(map[string]string)
	if !ok {
		return
	}

	if level, ok := m["level"]; ok {
		if parsed, err := model.ParseThinkingLevel(level); err == nil {
			a.thinkingLevel = parsed
		}
	}
}

func (a *AgentExtension) drainChanges(modelChangeCh, thinkingCh <-chan sdk.Event, bus sdk.Bus, currentProv sdk.Provider) sdk.Provider {
	for {
		select {
		case evt, ok := <-modelChangeCh:
			if !ok {
				return currentProv
			}

			currentProv = a.applyModelChange(evt, bus, currentProv)
		case evt, ok := <-thinkingCh:
			if !ok {
				return currentProv
			}

			a.applyThinkingChange(evt)
		default:
			return currentProv
		}
	}
}

func (a *AgentExtension) streamOpts() []model.StreamOption {
	level := a.thinkingLevel

	if level != model.ThinkingOff && a.modelName != "" {
		if modelDef, ok := model.GetModel(a.modelName); ok {
			if !modelDef.Reasoning {
				level = model.ThinkingOff
			} else {
				level = model.ClampForModel(level, modelDef)
			}
		}
	}

	opts := []model.StreamOption{
		model.WithThinkingLevel(level),
	}

	if a.modelName != "" {
		opts = append(opts, model.WithModel(a.modelName))
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

func streamTurn(ctx context.Context, bus sdk.Bus, provider sdk.Provider, messages []sdk.Message, tools []sdk.ToolDef, systemPrompt string, opts ...model.StreamOption) (sdk.Message, []sdk.ToolCall, error) {
	req := sdk.ProviderRequest{
		SystemPrompt: systemPrompt,
		Messages:     messages,
		Tools:        tools,
	}

	ch, err := provider.Stream(ctx, req, opts...)
	if err != nil {
		return sdk.Message{}, nil, fmt.Errorf("provider stream: %w", err)
	}

	bus.Publish(sdk.NewEvent(TopicMsgStart, nil))

	var content strings.Builder

	var thinking strings.Builder

	var signedThinking []sdk.SignedThinking

	var redactedThinking []sdk.RedactedThinking

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
		case sdk.ProviderEventThinkingDone:
			if st, ok := evt.Content.(sdk.SignedThinking); ok {
				signedThinking = append(signedThinking, st)
			}
		case sdk.ProviderEventRedactedThinkingDone:
			if rt, ok := evt.Content.(sdk.RedactedThinking); ok {
				redactedThinking = append(redactedThinking, rt)
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
	resp.Thinking = signedThinking
	resp.RedactedThinking = redactedThinking

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

// anyProviderHasKey returns true if at least one registered provider has
// valid auth credentials available.
func anyProviderHasKey() bool {
	return slices.ContainsFunc(sdk.ListProviders(), model.ProviderHasAuth)
}
