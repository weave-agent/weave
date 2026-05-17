package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OAuthCredential stores OAuth tokens for a provider.
type OAuthCredential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at,omitzero"`
	TokenType    string    `json:"token_type,omitempty"`
}

const oauthRefreshSkew = time.Minute

// IsExpired returns true if the credential has an expiry time that has passed.
func (o OAuthCredential) IsExpired() bool {
	return o.ExpiresWithin(0)
}

// ExpiresWithin returns true if the credential expires within d.
func (o OAuthCredential) ExpiresWithin(d time.Duration) bool {
	if o.ExpiresAt.IsZero() {
		return false
	}

	return time.Now().Add(d).After(o.ExpiresAt)
}

// File represents ~/.weave/auth.json.
// Provider auth is stored as raw JSON so that arbitrary provider-specific
// fields (org_id, tenant_id, etc.) can be persisted and loaded generically.
type File struct {
	Providers map[string]json.RawMessage `json:"providers"`
}

// Path returns the path to the auth file (~/.weave/auth.json).
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("auth path: %w", err)
	}

	return filepath.Join(home, ".weave", "auth.json"), nil
}

// Load reads and parses the auth file. Returns an empty File if not found.
func Load() (*File, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{Providers: make(map[string]json.RawMessage)}, nil
		}

		return nil, fmt.Errorf("read auth: %w", err)
	}

	var auth File
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("parse auth: %w", err)
	}

	if auth.Providers == nil {
		auth.Providers = make(map[string]json.RawMessage)
	}

	return &auth, nil
}

// Save writes the auth file with 0600 permissions.
func Save(auth *File) error {
	p, err := Path()
	if err != nil {
		return err
	}

	dir := filepath.Dir(p)
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return fmt.Errorf("create auth dir: %w", mkdirErr)
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}

	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("write auth: %w", err)
	}

	return nil
}

// GetProviderKey returns the stored API key for a provider, or "" if not set.
func (a *File) GetProviderKey(providerName string) string {
	raw, ok := a.Providers[providerName]
	if !ok {
		return ""
	}

	var p struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}

	return p.APIKey
}

// GetOAuthCredential returns the stored OAuth credential for a provider, or
// an empty OAuthCredential if not set.
func (a *File) GetOAuthCredential(providerName string) OAuthCredential {
	raw, ok := a.Providers[providerName]
	if !ok {
		return OAuthCredential{}
	}

	var cred OAuthCredential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return OAuthCredential{}
	}

	// If flat fields are empty, try nested oauth_token format.
	if cred.AccessToken == "" && cred.RefreshToken == "" {
		var nested struct {
			OAuthToken OAuthCredential `json:"oauth_token"`
		}
		if err := json.Unmarshal(raw, &nested); err == nil {
			cred = nested.OAuthToken
		}
	}

	return cred
}

// HasProviderAuth returns true if the provider has either an API key or OAuth
// credentials stored.
func (a *File) HasProviderAuth(providerName string) bool {
	raw, ok := a.Providers[providerName]
	if !ok || len(raw) == 0 {
		return false
	}

	var p struct {
		APIKey       string `json:"api_key"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return false
	}

	return p.APIKey != "" || p.AccessToken != ""
}

// SetProviderKey updates or adds a provider key in the auth file and saves.
// Preserves any other fields already present for the provider.
func SetProviderKey(providerName, apiKey string) error {
	auth, err := Load()
	if err != nil {
		return err
	}

	// Preserve existing fields by unmarshaling into a map.
	var providerMap map[string]any
	if raw, ok := auth.Providers[providerName]; ok && len(raw) > 0 {
		_ = json.Unmarshal(raw, &providerMap)
	}

	if providerMap == nil {
		providerMap = make(map[string]any)
	}

	providerMap["api_key"] = apiKey

	data, err := json.Marshal(providerMap)
	if err != nil {
		return fmt.Errorf("marshal provider auth: %w", err)
	}

	auth.Providers[providerName] = data

	return Save(auth)
}

// SetOAuthCredential updates or adds OAuth credentials for a provider in the
// auth file and saves. Preserves any other fields already present for the
// provider (e.g., api_key).
func SetOAuthCredential(providerName string, cred OAuthCredential) error {
	auth, err := Load()
	if err != nil {
		return err
	}

	// Preserve existing fields by unmarshaling into a map.
	var providerMap map[string]any
	if raw, ok := auth.Providers[providerName]; ok && len(raw) > 0 {
		_ = json.Unmarshal(raw, &providerMap)
	}

	if providerMap == nil {
		providerMap = make(map[string]any)
	}

	providerMap["access_token"] = cred.AccessToken

	if cred.RefreshToken != "" {
		providerMap["refresh_token"] = cred.RefreshToken
	}

	if !cred.ExpiresAt.IsZero() {
		providerMap["expires_at"] = cred.ExpiresAt.Format(time.RFC3339)
	}

	if cred.TokenType != "" {
		providerMap["token_type"] = cred.TokenType
	}

	data, err := json.Marshal(providerMap)
	if err != nil {
		return fmt.Errorf("marshal provider oauth: %w", err)
	}

	auth.Providers[providerName] = data

	return Save(auth)
}

// ClearProviderAuth removes all authentication data for a provider from the
// auth file and saves.
func ClearProviderAuth(providerName string) error {
	auth, err := Load()
	if err != nil {
		return err
	}

	delete(auth.Providers, providerName)

	return Save(auth)
}

func RefreshOAuthToken(ctx context.Context, providerName, tokenURL, clientID string) (OAuthCredential, error) {
	auth, err := Load()
	if err != nil {
		return OAuthCredential{}, err
	}

	cred := auth.GetOAuthCredential(providerName)
	if cred.RefreshToken == "" {
		return cred, fmt.Errorf("%s auth expired: refresh token missing", providerName)
	}

	tokenResp, err := RefreshToken(ctx, tokenURL, clientID, cred.RefreshToken)
	if err != nil {
		return cred, fmt.Errorf("%s auth expired: %w", providerName, err)
	}

	refreshed := OAuthCredential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
	}

	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = cred.RefreshToken
	}

	if refreshed.TokenType == "" {
		refreshed.TokenType = cred.TokenType
	}

	if tokenResp.ExpiresIn > 0 {
		refreshed.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	if refreshed.AccessToken == "" {
		return OAuthCredential{}, fmt.Errorf("%s auth expired: refresh response missing access token", providerName)
	}

	if err := SetOAuthCredential(providerName, refreshed); err != nil {
		return OAuthCredential{}, fmt.Errorf("save refreshed oauth credential: %w", err)
	}

	return refreshed, nil
}

func RefreshOAuthTokenIfNeeded(ctx context.Context, providerName, tokenURL, clientID string, cred OAuthCredential) (OAuthCredential, error) {
	if cred.AccessToken == "" {
		return cred, nil
	}

	if !cred.ExpiresWithin(oauthRefreshSkew) {
		return cred, nil
	}

	return RefreshOAuthToken(ctx, providerName, tokenURL, clientID)
}
