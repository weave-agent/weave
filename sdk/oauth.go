package sdk

import (
	"cmp"
	"log/slog"
	"slices"

	"github.com/weave-agent/weave/sdk/registry"
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
	ID              string
	Name            string
	AuthURL         string
	TokenURL        string
	DeviceCodeURL   string
	RedirectURI     string            // Fixed callback URI registered with the provider (e.g. "http://localhost:1455/auth/callback").
	ExtraAuthParams map[string]string // Additional query params for the authorization URL.
	Scopes          []string
	ClientID        string
	FlowType        OAuthFlowType
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
