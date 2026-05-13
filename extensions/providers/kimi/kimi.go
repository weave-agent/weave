package kimi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"weave/sdk"
	"weave/sdk/model"
)

const (
	defaultModel     = "kimi-for-coding"
	defaultMaxTokens = 32768
	defaultBaseURL   = "https://api.kimi.com/coding"
)

// KimiConfig holds per-provider configuration for the Kimi provider.
type KimiConfig struct {
	Model     string `json:"model" default:"kimi-for-coding" env:"KIMI_MODEL" description:"Model name"`
	MaxTokens int    `json:"max_tokens" default:"32768" env:"KIMI_MAX_TOKENS" validate:"gt=0" description:"Maximum tokens"`
	BaseURL   string `json:"base_url" default:"https://api.kimi.com/coding" env:"KIMI_BASE_URL" description:"API base URL"`
}

type provider struct {
	client    anthropic.Client
	model     string
	maxTokens int
	baseURL   string
}

func init() {
	model.RegisterProviderEnvVar("kimi", "KIMI_API_KEY")

	sdk.RegisterProvider[KimiConfig]("kimi", func(cfg sdk.Config, kc KimiConfig) (sdk.Provider, error) {
		apiKey, err := cfg.ResolveKey("kimi", "KIMI_API_KEY")
		if err != nil {
			return nil, fmt.Errorf("kimi: %w", err)
		}

		if apiKey == "" {
			return nil, errors.New("kimi: API key required (set KIMI_API_KEY, add to ~/.weave/auth.json, or configure in .weave/settings.json)")
		}

		client := anthropic.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(kc.BaseURL),
			option.WithHeader("User-Agent", "weave/0.1.0"),
		)

		return &provider{
			client:    client,
			model:     kc.Model,
			maxTokens: kc.MaxTokens,
			baseURL:   kc.BaseURL,
		}, nil
	})
}

// NewProviderWithClient creates a provider with a pre-configured client (for testing).
func NewProviderWithClient(client anthropic.Client, modelName string) sdk.Provider {
	if modelName == "" {
		modelName = defaultModel
	}

	return &provider{client: client, model: modelName, maxTokens: defaultMaxTokens, baseURL: defaultBaseURL}
}

func (p *provider) Stream(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
	ch := make(chan sdk.ProviderEvent, 64)

	so := model.NewStreamOptions(opts...)

	modelName := so.Model
	if modelName == "" {
		modelName = p.model
	}

	maxTokens := so.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}

	params := p.buildParams(req, modelName, maxTokens, so.ThinkingLevel)

	send := func(evt sdk.ProviderEvent) bool {
		select {
		case ch <- evt:
			return true
		case <-ctx.Done():
			return false
		}
	}

	go func() {
		defer close(ch)

		stream := p.client.Messages.NewStreaming(ctx, params)

		var message anthropic.Message

		for stream.Next() {
			event := stream.Current()
			_ = message.Accumulate(event)

			e, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent)
			if !ok {
				continue
			}

			if e.Delta.Text != "" {
				if !send(sdk.ProviderEvent{
					Type:    sdk.ProviderEventTextDelta,
					Content: e.Delta.Text,
				}) {
					return
				}
			}

			if e.Delta.Thinking != "" {
				if !send(sdk.ProviderEvent{
					Type:    sdk.ProviderEventThinking,
					Content: e.Delta.Thinking,
				}) {
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			send(sdk.ProviderEvent{
				Type:    sdk.ProviderEventError,
				Content: err.Error(),
			})

			return
		}

		emitContentBlocks(message.Content, send)
	}()

	return ch, nil
}

func (p *provider) buildParams(req sdk.ProviderRequest, mdl string, maxTokens int, thinkingLevel model.ThinkingLevel) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     mdl,
		MaxTokens: int64(maxTokens),
		Messages:  convertMessages(req.Messages),
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	if len(req.Tools) > 0 {
		params.Tools = convertTools(req.Tools)
	}

	thinkingLevel = resolveThinkingLevel(mdl, thinkingLevel)

	if thinkingLevel != model.ThinkingOff {
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		}

		effortMap := map[model.ThinkingLevel]anthropic.OutputConfigEffort{
			model.ThinkingMinimal: anthropic.OutputConfigEffortLow,
			model.ThinkingLow:     anthropic.OutputConfigEffortLow,
			model.ThinkingMedium:  anthropic.OutputConfigEffortMedium,
			model.ThinkingHigh:    anthropic.OutputConfigEffortHigh,
			model.ThinkingXHigh:   anthropic.OutputConfigEffortXhigh,
		}

		if effort, ok := effortMap[thinkingLevel]; ok {
			params.OutputConfig = anthropic.OutputConfigParam{Effort: effort}
		}
	}

	return params
}

func resolveThinkingLevel(mdl string, level model.ThinkingLevel) model.ThinkingLevel {
	if level == model.ThinkingOff {
		return model.ThinkingOff
	}

	m, ok := model.GetModel(mdl)
	if !ok {
		return level
	}

	if !m.Reasoning {
		return model.ThinkingOff
	}

	if level == model.ThinkingXHigh && !m.SupportsXHigh {
		return model.ThinkingHigh
	}

	return level
}

func emitContentBlocks(blocks []anthropic.ContentBlockUnion, send func(sdk.ProviderEvent) bool) {
	for _, block := range blocks {
		switch b := block.AsAny().(type) {
		case anthropic.ThinkingBlock:
			if !send(sdk.ProviderEvent{
				Type: sdk.ProviderEventThinkingDone,
				Content: sdk.SignedThinking{
					Signature: b.Signature,
					Thinking:  b.Thinking,
				},
			}) {
				return
			}
		case anthropic.RedactedThinkingBlock:
			if !send(sdk.ProviderEvent{
				Type: sdk.ProviderEventRedactedThinkingDone,
				Content: sdk.RedactedThinking{
					Data: b.Data,
				},
			}) {
				return
			}
		case anthropic.ToolUseBlock:
			args := parseToolArgs(b.Name, b.JSON.Input.Raw(), send)

			if !send(sdk.ProviderEvent{
				Type: sdk.ProviderEventToolCall,
				Content: sdk.ToolCall{
					ID:        b.ID,
					Name:      b.Name,
					Arguments: args,
				},
			}) {
				return
			}
		}
	}
}

func parseToolArgs(toolName, raw string, send func(sdk.ProviderEvent) bool) map[string]any {
	if raw == "" {
		return make(map[string]any)
	}

	var args map[string]any

	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		send(sdk.ProviderEvent{
			Type:    sdk.ProviderEventError,
			Content: fmt.Sprintf("kimi: parse tool call arguments for %s: %v", toolName, err),
		})

		return make(map[string]any)
	}

	return args
}

func convertMessages(msgs []sdk.Message) []anthropic.MessageParam {
	var (
		params             []anthropic.MessageParam
		pendingToolResults []anthropic.ContentBlockParamUnion
	)

	flush := func() {
		if len(pendingToolResults) > 0 {
			params = append(params, anthropic.NewUserMessage(pendingToolResults...))
			pendingToolResults = nil
		}
	}

	for _, msg := range msgs {
		switch msg.Role {
		case sdk.RoleUser:
			flush()

			params = append(params, anthropic.NewUserMessage(
				anthropic.NewTextBlock(fmt.Sprint(msg.Content)),
			))
		case sdk.RoleAssistant:
			flush()

			var blocks []anthropic.ContentBlockParamUnion

			for _, st := range msg.Thinking {
				blocks = append(blocks, anthropic.NewThinkingBlock(st.Signature, st.Thinking))
			}

			for _, rt := range msg.RedactedThinking {
				blocks = append(blocks, anthropic.NewRedactedThinkingBlock(rt.Data))
			}

			if text, ok := msg.Content.(string); ok && text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(text))
			}

			for _, tc := range msg.ToolCalls {
				inputJSON, _ := json.Marshal(tc.Arguments)
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    tc.ID,
						Name:  tc.Name,
						Input: inputJSON,
					},
				})
			}

			if len(blocks) > 0 {
				params = append(params, anthropic.NewAssistantMessage(blocks...))
			}
		case sdk.RoleToolResult:
			content := fmt.Sprint(msg.Content)
			pendingToolResults = append(pendingToolResults,
				anthropic.NewToolResultBlock(msg.ToolCallID, content, msg.IsError))
		}
	}

	flush()

	return params
}

func convertTools(tools []sdk.ToolDef) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, len(tools))

	for i, t := range tools {
		var (
			properties map[string]any
			required   []string
		)

		if params, ok := t.Parameters.(map[string]any); ok {
			if p, ok := params["properties"].(map[string]any); ok {
				properties = p
			}

			switch r := params["required"].(type) {
			case []string:
				required = r
			case []any:
				for _, v := range r {
					if s, ok := v.(string); ok {
						required = append(required, s)
					}
				}
			}
		}

		result[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: properties,
					Required:   required,
				},
			},
		}
	}

	return result
}
