package zai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"weave/sdk"
	"weave/sdk/model"
	openaicompat "weave/utils/openaicompat"
)

const (
	defaultModel   = "glm-5.1"
	defaultBaseURL = "https://api.z.ai/api/coding/paas/v4"
)

type provider struct {
	client *http.Client
	config openaicompat.ProviderConfig
}

func init() {
	model.RegisterProviderEnvVar("zai", "ZAI_API_KEY")

	sdk.RegisterProvider("zai", func(cfg sdk.Config) (sdk.Provider, error) {
		apiKey, err := cfg.ResolveKey("zai", "ZAI_API_KEY")
		if err != nil {
			return nil, fmt.Errorf("zai: %w", err)
		}

		if apiKey == "" {
			return nil, errors.New("zai: API key required (set ZAI_API_KEY, add to ~/.weave/auth.json, or configure in .weave/settings.json)")
		}

		modelName := defaultModel
		baseURL := defaultBaseURL

		if v := os.Getenv("ZAI_MODEL"); v != "" {
			modelName = v
		}

		if pc := cfg.ProviderConfig("zai"); pc != nil {
			if pc.Model != "" {
				modelName = pc.Model
			}

			if pc.BaseURL != "" {
				baseURL = pc.BaseURL
			}
		}

		return &provider{
			client: &http.Client{},
			config: openaicompat.ProviderConfig{
				BaseURL: baseURL,
				APIKey:  apiKey,
				Model:   modelName,
				ExtraBody: map[string]any{
					"tool_stream": true,
				},
				ModifyRequest: func(body map[string]any, so *model.StreamOptions) {
					if so.ThinkingLevel != model.ThinkingOff {
						body["enable_thinking"] = true
						delete(body, "reasoning_effort")
					}
				},
			},
		}, nil
	})
}

func (p *provider) Stream(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
	ch, err := openaicompat.Stream(ctx, p.client, p.config, req, opts...)
	if err != nil {
		return nil, fmt.Errorf("zai: %w", err)
	}

	return ch, nil
}
