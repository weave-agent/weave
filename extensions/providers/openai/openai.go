package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"weave/sdk"
	"weave/sdk/model"
	"weave/utils/openaicompat"
)

// OpenAIConfig holds per-provider configuration for the OpenAI provider.
type OpenAIConfig struct {
	Model   string `json:"model" default:"gpt-5.5" env:"OPENAI_MODEL" description:"Model name"`
	BaseURL string `json:"base_url" default:"https://api.openai.com/v1" env:"OPENAI_BASE_URL" description:"API base URL"`
}

// AuthConfig holds authentication credentials for the OpenAI provider.
type AuthConfig struct {
	APIKey     string              `json:"api_key" env:"OPENAI_API_KEY" description:"API key"`
	OAuthToken sdk.OAuthCredential `json:"oauth_token"`
}

const openAITokenURL = "https://auth.openai.com/oauth/token" // #nosec G101 -- OAuth endpoint URL, not a credential.

type provider struct {
	client      *http.Client
	config      openaicompat.ProviderConfig
	oauthToken  sdk.OAuthCredential
	oauthClient string
	tokenURL    string
}

func init() {
	// Register OAuth provider so /login can discover OpenAI's OAuth flow.
	sdk.RegisterOAuthProvider(sdk.OAuthProvider{
		ID:          "openai",
		Name:        "OpenAI",
		ClientID:    "app_EMoamEEZ73f0CkXaXp7hrann",
		AuthURL:     "https://auth.openai.com/oauth/authorize",
		TokenURL:    openAITokenURL,
		RedirectURI: "http://localhost:1455/auth/callback",
		ExtraAuthParams: map[string]string{
			"codex_cli_simplified_flow": "true",
			"originator":                "weave",
		},
		Scopes:   []string{"openid", "profile", "email", "offline_access"},
		FlowType: sdk.AuthorizationCode,
	})
	sdk.MarkProviderOAuthSupported("openai")

	sdk.RegisterProvider[OpenAIConfig, AuthConfig]("openai", func(cfg sdk.Config, oc OpenAIConfig, a AuthConfig) (sdk.Provider, error) {
		apiKey := a.APIKey
		if apiKey == "" {
			apiKey = a.OAuthToken.AccessToken
		}

		if apiKey == "" {
			return nil, errors.New("openai: API key or OAuth token required (set OPENAI_API_KEY, use /login, or add to ~/.weave/auth.json)")
		}

		return &provider{
			client: &http.Client{},
			config: openaicompat.ProviderConfig{
				BaseURL:       oc.BaseURL,
				APIKey:        apiKey,
				Model:         oc.Model,
				ModifyRequest: modifyRequest(oc.Model),
			},
			oauthToken: a.OAuthToken,
			tokenURL:   openAITokenURL,
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
	cfg, err := p.configWithFreshToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	ch, err := openaicompat.Stream(ctx, p.client, cfg, req, opts...)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	return ch, nil
}

func (p *provider) configWithFreshToken(ctx context.Context) (openaicompat.ProviderConfig, error) {
	if p.oauthToken.AccessToken == "" {
		return p.config, nil
	}

	cred, err := sdk.RefreshOAuthTokenIfNeeded(ctx, "openai", p.tokenURL, p.oauthClient, p.oauthToken)
	if err != nil {
		return openaicompat.ProviderConfig{}, fmt.Errorf("refresh oauth token: %w", err)
	}

	p.oauthToken = cred
	p.config.APIKey = cred.AccessToken

	return p.config, nil
}
