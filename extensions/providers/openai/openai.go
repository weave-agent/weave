package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"weave/sdk"
	openaicompat "weave/utils/openaicompat"
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
	sdk.RegisterProviderEnvVar("openai", "OPENAI_API_KEY")

	sdk.RegisterProvider("openai", func(cfg sdk.Config) (sdk.Provider, error) {
		apiKey, err := cfg.ResolveKey("openai", "OPENAI_API_KEY")
		if err != nil {
			return nil, fmt.Errorf("openai: %w", err)
		}

		if apiKey == "" {
			return nil, errors.New("openai: API key required (set OPENAI_API_KEY, add to ~/.weave/auth.json, or configure in .weave.yaml)")
		}

		model := defaultModel
		baseURL := defaultBaseURL

		if v := os.Getenv("OPENAI_MODEL"); v != "" {
			model = v
		}

		if pc := cfg.ProviderConfig("openai"); pc != nil {
			if pc.Model != "" {
				model = pc.Model
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
				Model:   model,
			},
		}, nil
	})
}

func (p *provider) Stream(ctx context.Context, req sdk.ProviderRequest, opts ...sdk.StreamOption) (<-chan sdk.ProviderEvent, error) {
	ch, err := openaicompat.Stream(ctx, p.client, p.config, req, opts...)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	return ch, nil
}
