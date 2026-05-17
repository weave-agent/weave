package tui

import (
	"errors"
	"testing"

	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildLoginProviders_EmptyWhenNoProviders(t *testing.T) {
	sdk.ResetProviderRegistry()
	sdk.ResetOAuthRegistry()
	sdkmodel.ResetModelRegistry()
	t.Cleanup(func() {
		sdk.ResetProviderRegistry()
		sdk.ResetOAuthRegistry()
		sdkmodel.ResetModelRegistry()
	})

	providers := buildLoginProviders()
	assert.Empty(t, providers)
}

func TestBuildLoginProviders_IncludesOAuthProviders(t *testing.T) {
	sdk.ResetProviderRegistry()
	sdk.ResetOAuthRegistry()
	sdkmodel.ResetModelRegistry()
	t.Cleanup(func() {
		sdk.ResetProviderRegistry()
		sdk.ResetOAuthRegistry()
		sdkmodel.ResetModelRegistry()
	})

	// Register a test OAuth provider
	sdk.RegisterOAuthProvider(sdk.OAuthProvider{
		ID:   "test-oauth-provider",
		Name: "Test OAuth Provider",
	})

	providers := buildLoginProviders()
	require.Len(t, providers, 1)
	assert.Equal(t, "Test OAuth Provider", providers[0].Name)
	assert.Equal(t, "test-oauth-provider", providers[0].ID)
	assert.True(t, providers[0].IsOAuth)
	assert.False(t, providers[0].HasAuth)
}

func TestBuildLoginProviders_IncludesRegisteredProviders(t *testing.T) {
	sdk.ResetProviderRegistry()
	sdk.ResetOAuthRegistry()
	sdkmodel.ResetModelRegistry()
	t.Cleanup(func() {
		sdk.ResetProviderRegistry()
		sdk.ResetOAuthRegistry()
		sdkmodel.ResetModelRegistry()
	})

	// Register a provider without OAuth
	sdk.RegisterProvider[struct{}, struct{}]("test-api-provider", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) {
		return nil, errors.New("test provider not implemented")
	})

	providers := buildLoginProviders()
	require.Len(t, providers, 1)
	assert.Equal(t, "Test-api-provider", providers[0].Name)
	assert.Equal(t, "test-api-provider", providers[0].ID)
	assert.False(t, providers[0].IsOAuth)
	assert.False(t, providers[0].HasAuth)
}

func TestBuildLoginProviders_SortsByName(t *testing.T) {
	sdk.ResetProviderRegistry()
	sdk.ResetOAuthRegistry()
	sdkmodel.ResetModelRegistry()
	t.Cleanup(func() {
		sdk.ResetProviderRegistry()
		sdk.ResetOAuthRegistry()
		sdkmodel.ResetModelRegistry()
	})

	sdk.RegisterOAuthProvider(sdk.OAuthProvider{ID: "zebra", Name: "Zebra"})
	sdk.RegisterOAuthProvider(sdk.OAuthProvider{ID: "alpha", Name: "Alpha"})

	providers := buildLoginProviders()
	require.Len(t, providers, 2)
	assert.Equal(t, "alpha", providers[0].ID)
	assert.Equal(t, "zebra", providers[1].ID)
}

func TestBuildLogoutProviders_EmptyWhenNoAuth(t *testing.T) {
	sdk.ResetProviderRegistry()
	sdk.ResetOAuthRegistry()
	sdkmodel.ResetModelRegistry()
	sdkmodel.ResetAuthRegistry()
	t.Cleanup(func() {
		sdk.ResetProviderRegistry()
		sdk.ResetOAuthRegistry()
		sdkmodel.ResetModelRegistry()
		sdkmodel.ResetAuthRegistry()
	})

	providers := buildLogoutProviders()
	assert.Empty(t, providers)
}

func TestBuildLogoutProviders_IncludesAuthenticatedProviders(t *testing.T) {
	sdk.ResetProviderRegistry()
	sdk.ResetOAuthRegistry()
	sdkmodel.ResetModelRegistry()
	sdkmodel.ResetAuthRegistry()
	t.Cleanup(func() {
		sdk.ResetProviderRegistry()
		sdk.ResetOAuthRegistry()
		sdkmodel.ResetModelRegistry()
		sdkmodel.ResetAuthRegistry()
	})

	// Register a provider and mark it as authenticated
	sdk.RegisterProvider[struct{}, struct{}]("authed-provider", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) {
		return nil, errors.New("test provider not implemented")
	})
	sdkmodel.SetProviderAuth("authed-provider", true)

	providers := buildLogoutProviders()
	require.Len(t, providers, 1)
	assert.Equal(t, "authed-provider", providers[0].ID)
	assert.Equal(t, "Authed-provider", providers[0].Name)
}

func TestDisplayNameForProvider(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"anthropic", "Anthropic"},
		{"openai", "Openai"},
		{"", ""},
		{"a", "A"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, displayNameForProvider(tt.input))
		})
	}
}
