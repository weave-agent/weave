package zai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	openaicompat "weave/ext/providers/openaicompat"
	"weave/sdk"
)

const (
	defaultModel   = "glm-4"
	defaultBaseURL = "https://open.bigmodel.cn/api/paas/v4"
)

type provider struct {
	client *http.Client
	config openaicompat.ProviderConfig
}

func init() {
	sdk.RegisterProvider("zai", func(cfg sdk.Config) (sdk.Provider, error) {
		apiKey, err := cfg.ResolveKey("zai", "ZAI_API_KEY")
		if err != nil {
			return nil, fmt.Errorf("zai: %w", err)
		}

		if apiKey == "" {
			return nil, errors.New("zai: API key required (set ZAI_API_KEY, add to ~/.weave/auth.json, or configure in .weave.yaml)")
		}

		model := defaultModel
		baseURL := defaultBaseURL

		if v := os.Getenv("ZAI_MODEL"); v != "" {
			model = v
		}

		if pc := cfg.ProviderConfig("zai"); pc != nil {
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
		return nil, fmt.Errorf("zai: %w", err)
	}

	return ch, nil
}
