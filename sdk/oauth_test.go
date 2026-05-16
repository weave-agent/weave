package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:gosec // test URLs are not credentials
func TestRegisterAndRetrieveOAuthProvider(t *testing.T) {
	ResetOAuthRegistry()
	defer ResetOAuthRegistry()

	RegisterOAuthProvider(OAuthProvider{
		ID:       "test-provider",
		Name:     "Test Provider",
		AuthURL:  "https://test.com/auth",
		TokenURL: "https://test.com/token",
		FlowType: AuthorizationCode,
	})

	p, ok := GetOAuthProvider("test-provider")
	require.True(t, ok)
	assert.Equal(t, "Test Provider", p.Name)
	assert.Equal(t, "https://test.com/auth", p.AuthURL)
	assert.Equal(t, AuthorizationCode, p.FlowType)
}

func TestGetOAuthProvider_Missing(t *testing.T) {
	ResetOAuthRegistry()
	defer ResetOAuthRegistry()

	_, ok := GetOAuthProvider("nonexistent")
	assert.False(t, ok)
}

func TestDuplicateOAuthProviderRegistration(t *testing.T) {
	ResetOAuthRegistry()
	defer ResetOAuthRegistry()

	RegisterOAuthProvider(OAuthProvider{
		ID:   "dup",
		Name: "First",
	})

	// Second registration should be a no-op with a warning (no panic).
	RegisterOAuthProvider(OAuthProvider{
		ID:   "dup",
		Name: "Second",
	})

	// First registration wins.
	p, ok := GetOAuthProvider("dup")
	require.True(t, ok)
	assert.Equal(t, "First", p.Name)
}

func TestListOAuthProviders(t *testing.T) {
	ResetOAuthRegistry()
	defer ResetOAuthRegistry()

	RegisterOAuthProvider(OAuthProvider{ID: "z-provider", Name: "Z"})
	RegisterOAuthProvider(OAuthProvider{ID: "a-provider", Name: "A"})
	RegisterOAuthProvider(OAuthProvider{ID: "m-provider", Name: "M"})

	providers := ListOAuthProviders()
	require.Len(t, providers, 3)
	assert.Equal(t, "a-provider", providers[0].ID)
	assert.Equal(t, "m-provider", providers[1].ID)
	assert.Equal(t, "z-provider", providers[2].ID)
}

func TestListOAuthProviders_Empty(t *testing.T) {
	ResetOAuthRegistry()
	defer ResetOAuthRegistry()

	assert.Empty(t, ListOAuthProviders())
}

//nolint:gosec // test URLs and client IDs are not credentials
func TestOpenAIOAuthProviderConfig(t *testing.T) {
	ResetOAuthRegistry()
	defer ResetOAuthRegistry()

	// Register with the same config the openai provider uses in init().
	RegisterOAuthProvider(OAuthProvider{
		ID:       "openai",
		Name:     "OpenAI",
		ClientID: "app_EMoamEEZ73f0CkXaXp7hrann",
		AuthURL:  "https://auth.openai.com/oauth/authorize",
		TokenURL: "https://auth.openai.com/oauth/token",
		Scopes:   []string{"openid", "profile", "email", "offline_access"},
		FlowType: AuthorizationCode,
	})
	MarkProviderOAuthSupported("openai")

	openai, ok := GetOAuthProvider("openai")
	require.True(t, ok)
	assert.Equal(t, "OpenAI", openai.Name)
	assert.Equal(t, AuthorizationCode, openai.FlowType)
	assert.Equal(t, "app_EMoamEEZ73f0CkXaXp7hrann", openai.ClientID)
	assert.Equal(t, "https://auth.openai.com/oauth/authorize", openai.AuthURL)
	assert.Equal(t, "https://auth.openai.com/oauth/token", openai.TokenURL)
	assert.Equal(t, []string{"openid", "profile", "email", "offline_access"}, openai.Scopes)
	assert.True(t, ProviderSupportsOAuth("openai"))
}

func TestProviderSupportsOAuth_NeverMarked(t *testing.T) {
	assert.False(t, ProviderSupportsOAuth("unknown-provider"))
}

func TestResetOAuthRegistry(t *testing.T) {
	ResetOAuthRegistry()
	defer ResetOAuthRegistry()

	RegisterOAuthProvider(OAuthProvider{ID: "temp", Name: "Temp"})
	require.NotEmpty(t, ListOAuthProviders())

	ResetOAuthRegistry()
	assert.Empty(t, ListOAuthProviders())
}
