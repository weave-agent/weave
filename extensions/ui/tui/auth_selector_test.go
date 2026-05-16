package tui

import (
	"testing"

	sdkmodel "weave/sdk/model"

	"github.com/stretchr/testify/assert"
)

func TestBuildLoginProviders_EmptyWhenNoProviders(t *testing.T) {
	providers := buildLoginProviders()
	assert.NotNil(t, providers)
}

func TestBuildLoginProviders_IncludesOAuthProviders(t *testing.T) {
	// Register a test OAuth provider
	sdkmodel.RegisterModel(sdkmodel.ModelDef{
		ID:       "test-oauth-provider",
		Provider: "test-oauth-provider",
	})

	providers := buildLoginProviders()
	assert.NotNil(t, providers)
}

func TestBuildLogoutProviders_EmptyWhenNoAuth(t *testing.T) {
	providers := buildLogoutProviders()
	assert.Empty(t, providers)
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

func TestLoginProviderEntry_Struct(t *testing.T) {
	entry := LoginProviderEntry{
		Name:    "Test Provider",
		ID:      "test-provider",
		IsOAuth: true,
		HasAuth: false,
	}
	assert.Equal(t, "Test Provider", entry.Name)
	assert.Equal(t, "test-provider", entry.ID)
	assert.True(t, entry.IsOAuth)
	assert.False(t, entry.HasAuth)
}

func TestLogoutProviderEntry_Struct(t *testing.T) {
	entry := LogoutProviderEntry{
		Name: "Test Provider",
		ID:   "test-provider",
	}
	assert.Equal(t, "Test Provider", entry.Name)
	assert.Equal(t, "test-provider", entry.ID)
}
