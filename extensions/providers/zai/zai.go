package zai

import (
	"context"
	"fmt"
	"net/http"
	"os"

	openaicompat "weave/ext/providers/openaicompat"
	"weave/sdk"
)

const defaultModel = "glm-4"

type provider struct {
	client *http.Client
	config openaicompat.ProviderConfig
}

func init() {
	sdk.RegisterProvider("zai", func(_ sdk.Config) (sdk.Provider, error) {
		apiKey := os.Getenv("ZAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("zai: ZAI_API_KEY environment variable is required")
		}

		model := os.Getenv("ZAI_MODEL")
		if model == "" {
			model = defaultModel
		}

		return &provider{
			client: &http.Client{},
			config: openaicompat.ProviderConfig{
				BaseURL: "https://open.bigmodel.cn/api/paas/v4",
				APIKey:  apiKey,
				Model:   model,
			},
		}, nil
	})
}

func (p *provider) Stream(ctx context.Context, req sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
	return openaicompat.Stream(ctx, p.client, p.config, req)
}
