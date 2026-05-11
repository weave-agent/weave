package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"weave/sdk"
	"weave/sdk/model"
	"weave/utils/openaicompat"
)

const (
	defaultModel   = "gpt-5.5"
	defaultBaseURL = "https://api.openai.com/v1"
)

type provider struct {
	client *http.Client
	config openaicompat.ProviderConfig
}

func init() {
	model.RegisterProviderEnvVar("openai", "OPENAI_API_KEY")

	sdk.RegisterProvider("openai", func(cfg sdk.Config) (sdk.Provider, error) {
		apiKey, err := cfg.ResolveKey("openai", "OPENAI_API_KEY")
		if err != nil {
			return nil, fmt.Errorf("openai: %w", err)
		}

		if apiKey == "" {
			return nil, errors.New("openai: API key required (set OPENAI_API_KEY, add to ~/.weave/auth.json, or configure in .weave/settings.json)")
		}

		modelName := defaultModel
		baseURL := defaultBaseURL

		if v := os.Getenv("OPENAI_MODEL"); v != "" {
			modelName = v
		}

		if pc := cfg.ProviderConfig("openai"); pc != nil {
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
				BaseURL:       baseURL,
				APIKey:        apiKey,
				Model:         modelName,
				ModifyRequest: modifyRequest(modelName),
			},
		}, nil
	})
}

// modifyRequest sets OpenAI-specific fields on the request body map.
func modifyRequest(modelName string) func(body map[string]any, so *model.StreamOptions) {
	return func(body map[string]any, so *model.StreamOptions) {
		mdl := so.Model
		if mdl == "" {
			mdl = modelName
		}

		if so.ThinkingLevel == model.ThinkingOff {
			return
		}

		if m, ok := model.GetModel(mdl); ok && !m.Reasoning {
			return
		}

		effortMap := map[model.ThinkingLevel]string{
			model.ThinkingMinimal: "low",
			model.ThinkingLow:     "low",
			model.ThinkingMedium:  "medium",
			model.ThinkingHigh:    "high",
			model.ThinkingXHigh:   "high",
		}

		if effort, ok := effortMap[so.ThinkingLevel]; ok {
			body["reasoning_effort"] = effort
		}
	}
}

func (p *provider) Stream(ctx context.Context, req sdk.ProviderRequest, opts ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
	ch, err := openaicompat.Stream(ctx, p.client, p.config, req, opts...)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	return ch, nil
}
