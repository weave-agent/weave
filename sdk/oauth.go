package sdk

import (
	"cmp"
	"log/slog"
	"slices"
	"sync"

	"weave/sdk/registry"
)

// OAuthFlowType defines the type of OAuth flow a provider supports.
type OAuthFlowType string

const (
	// AuthorizationCode flow redirects the user to a browser, then a
	// callback server receives the authorization code.
	AuthorizationCode OAuthFlowType = "authorization_code"
	// DeviceCode flow displays a user code and verification URL, then
	// polls the token endpoint until the user authorizes the device.
	DeviceCode OAuthFlowType = "device_code"
)

// OAuthProvider describes an OAuth identity provider and its endpoints.
type OAuthProvider struct {
	ID            string
	Name          string
	AuthURL       string
	TokenURL      string
	DeviceCodeURL string
	Scopes        []string
	ClientID      string
	FlowType      OAuthFlowType
}

var oauthReg = registry.New[OAuthProvider](
	registry.WithWarn[OAuthProvider](func(name string) {
		slog.Warn("duplicate registration", "name", name, "kind", "oauth_provider")
	}),
)

// RegisterOAuthProvider adds an OAuth provider definition to the global
// registry. Duplicate registrations log a warning and keep the first entry.
func RegisterOAuthProvider(provider OAuthProvider) {
	oauthReg.Register(provider.ID, provider)
}

// GetOAuthProvider returns an OAuth provider definition by ID.
func GetOAuthProvider(id string) (OAuthProvider, bool) {
	return oauthReg.Get(id)
}

// ListOAuthProviders returns all registered OAuth providers sorted by ID.
func ListOAuthProviders() []OAuthProvider {
	all := oauthReg.All()

	slices.SortFunc(all, func(a, b OAuthProvider) int {
		return cmp.Compare(a.ID, b.ID)
	})

	return all
}

// ResetOAuthRegistry clears all registered OAuth providers. For testing only.
func ResetOAuthRegistry() {
	oauthReg.Reset()
}

// oauthProviderAuthMu protects the map of which providers are known to
// support OAuth (set by RegisterBuiltinOAuthProviders).
var (
	oauthProviderAuth   = make(map[string]bool)
	oauthProviderAuthMu sync.RWMutex
)

// MarkProviderOAuthSupported records that the named provider supports OAuth
// authentication. This is used by CheckProviderAuth as an additional signal
// that a provider may have OAuth credentials even when its static auth struct
// does not yet declare an OAuthToken field.
func MarkProviderOAuthSupported(provider string) {
	oauthProviderAuthMu.Lock()
	defer oauthProviderAuthMu.Unlock()

	oauthProviderAuth[provider] = true
}

// ProviderSupportsOAuth returns true if the provider has been marked as
// supporting OAuth authentication.
func ProviderSupportsOAuth(provider string) bool {
	oauthProviderAuthMu.RLock()
	defer oauthProviderAuthMu.RUnlock()

	return oauthProviderAuth[provider]
}

// RegisterBuiltinOAuthProviders registers the built-in OAuth providers
// (GitHub Copilot and OpenAI).
//
//nolint:gosec // OAuth endpoint URLs are not credentials
func RegisterBuiltinOAuthProviders() {
	RegisterOAuthProvider(OAuthProvider{
		ID:            "github-copilot",
		Name:          "GitHub Copilot",
		AuthURL:       "https://github.com/login/oauth/authorize",
		TokenURL:      "https://github.com/login/oauth/access_token",
		DeviceCodeURL: "https://github.com/login/device/code",
		Scopes:        []string{"read:user", "read:org"},
		FlowType:      DeviceCode,
	})
	MarkProviderOAuthSupported("github-copilot")

	RegisterOAuthProvider(OAuthProvider{
		ID:       "openai",
		Name:     "OpenAI",
		AuthURL:  "https://platform.openai.com/authorize",
		TokenURL: "https://api.openai.com/v1/oauth/token",
		Scopes:   []string{"api"},
		FlowType: AuthorizationCode,
	})
	MarkProviderOAuthSupported("openai")
}
