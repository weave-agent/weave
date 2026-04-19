package openai

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	openaicompat "weave/ext/providers/openaicompat"
	"weave/sdk"
)

const defaultModel = "gpt-4o"

type provider struct {
	client *http.Client
	config openaicompat.ProviderConfig
}

func init() {
	sdk.RegisterProvider("openai", func(_ sdk.Config) (sdk.Provider, error) {
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("openai: OPENAI_API_KEY environment variable is required")
		}

		model := os.Getenv("OPENAI_MODEL")
		if model == "" {
			model = defaultModel
		}

		return &provider{
			client: &http.Client{
				Timeout: 5 * time.Minute,
			},
			config: openaicompat.ProviderConfig{
				BaseURL: "https://api.openai.com/v1",
				APIKey:  apiKey,
				Model:   model,
			},
		}, nil
	})
}

func (p *provider) Stream(ctx context.Context, req sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
	return openaicompat.Stream(ctx, p.client, p.config, req)
}
