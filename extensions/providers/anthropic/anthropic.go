package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"weave/sdk"
)

const defaultModel = "claude-sonnet-4-20250514"

type provider struct {
	client anthropic.Client
	model  string
}

func init() {
	sdk.RegisterProvider("anthropic", func(_ sdk.Config) (sdk.Provider, error) {
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("anthropic: ANTHROPIC_API_KEY environment variable is required")
		}

		model := os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = defaultModel
		}

		client := anthropic.NewClient(option.WithAPIKey(apiKey))

		return &provider{
			client: client,
			model:  model,
		}, nil
	})
}

// NewProviderWithClient creates a provider with a pre-configured client (for testing).
func NewProviderWithClient(client anthropic.Client, model string) sdk.Provider {
	if model == "" {
		model = defaultModel
	}
	return &provider{client: client, model: model}
}

func (p *provider) Stream(ctx context.Context, req sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
	ch := make(chan sdk.ProviderEvent, 64)

	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: 8192,
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
					ch <- sdk.ProviderEvent{
						Type:    sdk.ProviderEventTextDelta,
						Content: e.Delta.Text,
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- sdk.ProviderEvent{
				Type:    sdk.ProviderEventError,
				Content: err.Error(),
			}
			return
		}

		for _, block := range message.Content {
			if toolUse, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				var args map[string]any
				if raw := toolUse.JSON.Input.Raw(); raw != "" {
					if err := json.Unmarshal([]byte(raw), &args); err != nil {
						args = make(map[string]any)
					}
				} else {
					args = make(map[string]any)
				}
				ch <- sdk.ProviderEvent{
					Type: sdk.ProviderEventToolCall,
					Content: sdk.ToolCall{
						ID:        toolUse.ID,
						Name:      toolUse.Name,
						Arguments: args,
					},
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
		if params, ok := t.Parameters.(map[string]any); ok {
			if p, ok := params["properties"].(map[string]any); ok {
				properties = p
			}
		}
		result[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: properties,
				},
			},
		}
	}
	return result
}
