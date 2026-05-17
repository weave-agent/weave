package sdk

import (
	"context"
	"fmt"

	"weave/internal/auth"
)

// OAuthCredential stores OAuth tokens for a provider.
type OAuthCredential = auth.OAuthCredential

// DeviceCodeResponse holds the result from the device authorization endpoint.
type DeviceCodeResponse = auth.DeviceCodeResponse

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

func RefreshOAuthTokenIfNeeded(ctx context.Context, providerName, tokenURL, clientID string, cred OAuthCredential) (OAuthCredential, error) {
	refreshed, err := auth.RefreshOAuthTokenIfNeeded(ctx, providerName, tokenURL, clientID, cred)
	if err != nil {
		return OAuthCredential{}, fmt.Errorf("refresh oauth token: %w", err)
	}

	return refreshed, nil
}

// RunAuthorizationCodeFlow executes the full OAuth authorization code flow.
// It starts a callback server, opens the browser, waits for the callback,
// exchanges the code for tokens, and returns the credential.
func RunAuthorizationCodeFlow(ctx context.Context, authURL, tokenURL, clientID, redirectURI string, scopes []string, extraParams map[string]string) (OAuthCredential, error) {
	cred, err := auth.RunAuthorizationCodeFlow(ctx, authURL, tokenURL, clientID, redirectURI, scopes, extraParams)
	if err != nil {
		return OAuthCredential{}, fmt.Errorf("authorization code flow: %w", err)
	}

	return cred, nil
}

// AuthorizationFlowHandle is an opaque handle for an in-flight authorization
// code flow. It is returned by StartAuthorizationCodeFlow and must be passed
// to CompleteAuthorizationCodeFlow to finish the flow.
type AuthorizationFlowHandle = auth.AuthorizationFlowHandle

// StartAuthorizationCodeFlow begins the authorization code flow by generating
// PKCE parameters, starting a callback server, building the authorization URL,
// and opening the browser. Returns the full authorization URL and a handle to
// complete the flow.
func StartAuthorizationCodeFlow(ctx context.Context, authURL, tokenURL, clientID, redirectURI string, scopes []string, extraParams map[string]string) (string, *AuthorizationFlowHandle, error) {
	url, handle, err := auth.StartAuthorizationCodeFlow(ctx, authURL, tokenURL, clientID, redirectURI, scopes, extraParams)
	if err != nil {
		return "", nil, fmt.Errorf("start authorization code flow: %w", err)
	}

	return url, handle, nil
}

// CompleteAuthorizationCodeFlow finishes an authorization code flow that was
// started with StartAuthorizationCodeFlow. It waits for the callback, verifies
// state, exchanges the code for tokens, and returns the credential.
func CompleteAuthorizationCodeFlow(ctx context.Context, handle *AuthorizationFlowHandle) (OAuthCredential, error) {
	cred, err := auth.CompleteAuthorizationCodeFlow(ctx, handle)
	if err != nil {
		return OAuthCredential{}, fmt.Errorf("complete authorization code flow: %w", err)
	}

	return cred, nil
}

// RequestDeviceCode requests a device code from the device authorization
// endpoint (RFC 8628).
func RequestDeviceCode(ctx context.Context, deviceCodeURL, clientID string, scopes []string) (DeviceCodeResponse, error) {
	resp, err := auth.RequestDeviceCode(ctx, deviceCodeURL, clientID, scopes)
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("request device code: %w", err)
	}

	return resp, nil
}

// PollDeviceToken polls the token endpoint for a device code flow (RFC 8628)
// until the user authorizes the device, an error occurs, or the context is
// canceled.
func PollDeviceToken(ctx context.Context, tokenURL, clientID, deviceCode string, intervalSecs int) (TokenResponse, error) {
	resp, err := auth.PollDeviceToken(ctx, tokenURL, clientID, deviceCode, intervalSecs)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("poll device token: %w", err)
	}

	return resp, nil
}

// TokenResponse is the parsed JSON response from the token endpoint.
type TokenResponse = auth.TokenResponse
