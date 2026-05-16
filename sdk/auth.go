package sdk

import (
	"context"
	"fmt"

	"weave/internal/auth"
)

// OAuthCredential stores OAuth tokens for a provider.
type OAuthCredential = auth.OAuthCredential

// ClearProviderAuth removes all authentication data for a provider from the
// auth file.
func ClearProviderAuth(providerName string) error {
	if err := auth.ClearProviderAuth(providerName); err != nil {
		return fmt.Errorf("clear provider auth: %w", err)
	}

	return nil
}

// SetOAuthCredential updates or adds OAuth credentials for a provider in the
// auth file and saves. Preserves any other fields already present for the
// provider (e.g., api_key).
func SetOAuthCredential(providerName string, cred OAuthCredential) error {
	if err := auth.SetOAuthCredential(providerName, cred); err != nil {
		return fmt.Errorf("set oauth credential: %w", err)
	}

	return nil
}

// RunAuthorizationCodeFlow executes the full OAuth authorization code flow.
// It starts a callback server, opens the browser, waits for the callback,
// exchanges the code for tokens, and returns the credential.
func RunAuthorizationCodeFlow(ctx context.Context, authURL, tokenURL, clientID string, scopes []string) (OAuthCredential, error) {
	cred, err := auth.RunAuthorizationCodeFlow(ctx, authURL, tokenURL, clientID, scopes)
	if err != nil {
		return OAuthCredential{}, fmt.Errorf("authorization code flow: %w", err)
	}

	return cred, nil
}
