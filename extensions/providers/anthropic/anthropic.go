package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"weave/sdk"
)

const (
	defaultModel     = "claude-sonnet-4-20250514"
	defaultMaxTokens = 8192
)

type provider struct {
	client    anthropic.Client
	model     string
	maxTokens int64
}

func init() {
	sdk.RegisterProvider("anthropic", func(cfg sdk.Config) (sdk.Provider, error) {
		apiKey, err := cfg.ResolveKey("anthropic", "ANTHROPIC_API_KEY")
		if err != nil {
			return nil, fmt.Errorf("anthropic: %w", err)
		}

		if apiKey == "" {
			return nil, fmt.Errorf("anthropic: API key required (set ANTHROPIC_API_KEY, add to ~/.weave/auth.json, or configure in .weave.yaml)")
		}

		model := defaultModel
		maxTokens := int64(defaultMaxTokens)

		if v := os.Getenv("ANTHROPIC_MODEL"); v != "" {
			model = v
		}

		if v := os.Getenv("ANTHROPIC_MAX_TOKENS"); v != "" {
			if n, parseErr := strconv.ParseInt(v, 10, 64); parseErr == nil && n > 0 {
				maxTokens = n
			}
		}

		if pc := cfg.ProviderConfig("anthropic"); pc != nil {
			if pc.Model != "" {
				model = pc.Model
			}

			if pc.MaxTokens > 0 {
				maxTokens = pc.MaxTokens
			}
		}

		client := anthropic.NewClient(option.WithAPIKey(apiKey))

		return &provider{
			client:    client,
			model:     model,
			maxTokens: maxTokens,
		}, nil
	})
}

// NewProviderWithClient creates a provider with a pre-configured client (for testing).
func NewProviderWithClient(client anthropic.Client, model string) sdk.Provider {
	if model == "" {
		model = defaultModel
	}

	return &provider{client: client, model: model, maxTokens: defaultMaxTokens}
}

func (p *provider) Stream(ctx context.Context, req sdk.ProviderRequest, opts ...sdk.StreamOption) (<-chan sdk.ProviderEvent, error) {
	ch := make(chan sdk.ProviderEvent, 64)

	so := sdk.NewStreamOptions(opts...)
	model := so.Model
	if model == "" {
		model = p.model
	}
	maxTokens := so.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}

	params := anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
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

	thinkingLevel := so.ThinkingLevel
	if thinkingLevel == sdk.ThinkingXHigh {
		if m, ok := sdk.GetModel(model); ok && !m.SupportsXHigh {
			thinkingLevel = sdk.ThinkingHigh
		}
	}

	if thinkingLevel != sdk.ThinkingOff {
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		}
	}

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

			switch e := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
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
		}

		if err := stream.Err(); err != nil {
			send(sdk.ProviderEvent{
				Type:    sdk.ProviderEventError,
				Content: err.Error(),
			})

			return
		}

		for _, block := range message.Content {
			switch b := block.AsAny().(type) {
			case anthropic.ToolUseBlock:
				var args map[string]any

				if raw := b.JSON.Input.Raw(); raw != "" {
					if err := json.Unmarshal([]byte(raw), &args); err != nil {
						send(sdk.ProviderEvent{
							Type:    sdk.ProviderEventError,
							Content: fmt.Sprintf("anthropic: parse tool call arguments for %s: %v", b.Name, err),
						})

						return
					}
				} else {
					args = make(map[string]any)
				}

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
	}()

	return ch, nil
}

func convertMessages(msgs []sdk.Message) []anthropic.MessageParam {
	var params []anthropic.MessageParam
	var pendingToolResults []anthropic.ContentBlockParamUnion

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
		var properties map[string]any
		var required []string

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
