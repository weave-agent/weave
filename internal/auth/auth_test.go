package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	auth, err := Load()
	require.NoError(t, err)
	assert.Empty(t, auth.GetProviderKey("anthropic"))
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	testKey1 := "sk-ant-" + t.Name()
	testKey2 := "sk-" + t.Name()

	auth := &File{
		Providers: map[string]json.RawMessage{
			"anthropic": json.RawMessage(`{"api_key": "` + testKey1 + `"}`),
			"openai":    json.RawMessage(`{"api_key": "` + testKey2 + `"}`),
		},
	}

	require.NoError(t, Save(auth))

	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, testKey1, loaded.GetProviderKey("anthropic"))
	assert.Equal(t, testKey2, loaded.GetProviderKey("openai"))
	assert.Empty(t, loaded.GetProviderKey("unknown"))
}

func TestSave_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// ~/.weave/ doesn't exist yet
	auth := &File{Providers: map[string]json.RawMessage{}}
	require.NoError(t, Save(auth))

	_, err := os.Stat(filepath.Join(dir, ".weave", "auth.json"))
	require.NoError(t, err)
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	require.NoError(t, Save(&File{Providers: map[string]json.RawMessage{}}))

	info, err := os.Stat(filepath.Join(dir, ".weave", "auth.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestSetProviderKey_NewProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	require.NoError(t, SetProviderKey("anthropic", "sk-new"))

	auth, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "sk-new", auth.GetProviderKey("anthropic"))
}

func TestSetProviderKey_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	require.NoError(t, SetProviderKey("anthropic", "sk-old"))
	require.NoError(t, SetProviderKey("anthropic", "sk-updated"))

	auth, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "sk-updated", auth.GetProviderKey("anthropic"))
}

func TestGetOAuthCredential_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	auth, err := Load()
	require.NoError(t, err)

	cred := auth.GetOAuthCredential("openai")
	assert.Empty(t, cred.AccessToken)
	assert.Empty(t, cred.RefreshToken)
	assert.True(t, cred.ExpiresAt.IsZero())
}

func TestGetOAuthCredential_FromAuthFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	auth := &File{
		Providers: map[string]json.RawMessage{
			"openai": json.RawMessage(`{"access_token":"sk-oauth","refresh_token":"rt-123","expires_at":"2026-05-16T12:00:00Z","token_type":"bearer"}`),
		},
	}
	require.NoError(t, Save(auth))

	loaded, err := Load()
	require.NoError(t, err)

	cred := loaded.GetOAuthCredential("openai")
	assert.Equal(t, "sk-oauth", cred.AccessToken)
	assert.Equal(t, "rt-123", cred.RefreshToken)
	assert.Equal(t, "bearer", cred.TokenType)
	assert.False(t, cred.ExpiresAt.IsZero())
}

func TestGetOAuthCredential_NestedFormat(t *testing.T) {
	auth := &File{
		Providers: map[string]json.RawMessage{
			"openai": json.RawMessage(`{"oauth_token":{"access_token":"sk-nested","refresh_token":"rt-nested","expires_at":"2026-05-16T12:00:00Z","token_type":"bearer"}}`),
		},
	}

	cred := auth.GetOAuthCredential("openai")
	assert.Equal(t, "sk-nested", cred.AccessToken)
	assert.Equal(t, "rt-nested", cred.RefreshToken)
	assert.Equal(t, "bearer", cred.TokenType)
	assert.False(t, cred.ExpiresAt.IsZero())
}

func TestSetOAuthCredential_NewProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cred := OAuthCredential{
		AccessToken:  "at-new",
		RefreshToken: "rt-new",
		TokenType:    "bearer",
	}
	require.NoError(t, SetOAuthCredential("github-copilot", cred))

	auth, err := Load()
	require.NoError(t, err)

	loaded := auth.GetOAuthCredential("github-copilot")
	assert.Equal(t, "at-new", loaded.AccessToken)
	assert.Equal(t, "rt-new", loaded.RefreshToken)
	assert.Equal(t, "bearer", loaded.TokenType)
}

func TestSetOAuthCredential_PreservesAPIKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	require.NoError(t, SetProviderKey("openai", "sk-existing"))

	cred := OAuthCredential{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
	}
	require.NoError(t, SetOAuthCredential("openai", cred))

	auth, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "sk-existing", auth.GetProviderKey("openai"))
	assert.Equal(t, "test-access-token", auth.GetOAuthCredential("openai").AccessToken)
}

func TestSetOAuthCredential_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	oldCred := OAuthCredential{AccessToken: "at-old", RefreshToken: "rt-old"}
	require.NoError(t, SetOAuthCredential("openai", oldCred))

	newCred := OAuthCredential{AccessToken: "at-new", RefreshToken: "rt-new"}
	require.NoError(t, SetOAuthCredential("openai", newCred))

	auth, err := Load()
	require.NoError(t, err)

	loaded := auth.GetOAuthCredential("openai")
	assert.Equal(t, "at-new", loaded.AccessToken)
	assert.Equal(t, "rt-new", loaded.RefreshToken)
}

func TestClearProviderAuth_RemovesProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	require.NoError(t, SetProviderKey("anthropic", "sk-key"))
	require.NoError(t, ClearProviderAuth("anthropic"))

	auth, err := Load()
	require.NoError(t, err)
	assert.Empty(t, auth.GetProviderKey("anthropic"))
	assert.False(t, auth.HasProviderAuth("anthropic"))
}

func TestClearProviderAuth_LeavesOthers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	require.NoError(t, SetProviderKey("anthropic", "sk-ant"))
	require.NoError(t, SetProviderKey("openai", "sk-openai"))
	require.NoError(t, ClearProviderAuth("anthropic"))

	auth, err := Load()
	require.NoError(t, err)
	assert.Empty(t, auth.GetProviderKey("anthropic"))
	assert.Equal(t, "sk-openai", auth.GetProviderKey("openai"))
}

func TestHasProviderAuth_APIKey(t *testing.T) {
	auth := &File{
		Providers: map[string]json.RawMessage{
			"anthropic": json.RawMessage(`{"api_key":"sk-ant"}`),
		},
	}
	assert.True(t, auth.HasProviderAuth("anthropic"))
	assert.False(t, auth.HasProviderAuth("openai"))
}

func TestHasProviderAuth_OAuth(t *testing.T) {
	auth := &File{
		Providers: map[string]json.RawMessage{
			"openai": json.RawMessage(`{"access_token":"sk-oauth"}`),
		},
	}
	assert.True(t, auth.HasProviderAuth("openai"))
}

func TestHasProviderAuth_RefreshTokenOnly(t *testing.T) {
	auth := &File{
		Providers: map[string]json.RawMessage{
			"github-copilot": json.RawMessage(`{"refresh_token":"rt-only"}`),
		},
	}
	assert.False(t, auth.HasProviderAuth("github-copilot"))
}

func TestHasProviderAuth_EmptyProvider(t *testing.T) {
	auth := &File{
		Providers: map[string]json.RawMessage{
			"empty": json.RawMessage(`{}`),
		},
	}
	assert.False(t, auth.HasProviderAuth("empty"))
}

func TestOAuthCredential_IsExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	assert.True(t, OAuthCredential{ExpiresAt: past}.IsExpired())
	assert.False(t, OAuthCredential{ExpiresAt: future}.IsExpired())
	assert.False(t, OAuthCredential{}.IsExpired())
}

func TestOAuthCredential_ExpiresWithin(t *testing.T) {
	soon := time.Now().Add(30 * time.Second)
	later := time.Now().Add(2 * time.Minute)

	assert.True(t, OAuthCredential{ExpiresAt: soon}.ExpiresWithin(time.Minute))
	assert.False(t, OAuthCredential{ExpiresAt: later}.ExpiresWithin(time.Minute))
	assert.False(t, OAuthCredential{}.ExpiresWithin(time.Minute))
}

func TestRefreshOAuthTokenIfNeeded_NotNearExpiry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cred := OAuthCredential{AccessToken: "at-current", RefreshToken: "rt-current", ExpiresAt: time.Now().Add(time.Hour)}
	refreshed, err := RefreshOAuthTokenIfNeeded(t.Context(), "openai", "http://127.0.0.1:1/token", "client", cred)
	require.NoError(t, err)
	assert.Equal(t, cred.AccessToken, refreshed.AccessToken)
}

func TestRefreshOAuthTokenIfNeeded_RefreshesNearExpiry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			return
		}

		assert.Equal(t, "rt-old", r.FormValue("refresh_token"))

		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-new",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	oldCred := OAuthCredential{
		AccessToken:  "at-old",
		RefreshToken: "rt-old",
		ExpiresAt:    time.Now().Add(30 * time.Second),
		TokenType:    "bearer",
	}
	require.NoError(t, SetOAuthCredential("openai", oldCred))

	refreshed, err := RefreshOAuthTokenIfNeeded(t.Context(), "openai", server.URL, "client-id", oldCred)
	require.NoError(t, err)
	assert.Equal(t, "at-new", refreshed.AccessToken)
	assert.Equal(t, "rt-old", refreshed.RefreshToken)
	assert.Equal(t, "bearer", refreshed.TokenType)
	assert.True(t, refreshed.ExpiresAt.After(time.Now().Add(30*time.Minute)))

	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "at-new", loaded.GetOAuthCredential("openai").AccessToken)
}

func TestRefreshOAuthTokenIfNeeded_MissingRefreshToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cred := OAuthCredential{AccessToken: "at-old", ExpiresAt: time.Now().Add(-time.Minute)}
	_, err := RefreshOAuthTokenIfNeeded(t.Context(), "openai", "http://127.0.0.1:1/token", "client", cred)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth expired")
	assert.Contains(t, err.Error(), "refresh token missing")
}

func TestRefreshOAuthTokenIfNeeded_RefreshFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant"})
	}))
	defer server.Close()

	cred := OAuthCredential{AccessToken: "at-old", RefreshToken: "rt-old", ExpiresAt: time.Now().Add(-time.Minute)}
	require.NoError(t, SetOAuthCredential("openai", cred))

	_, err := RefreshOAuthTokenIfNeeded(t.Context(), "openai", server.URL, "client", cred)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth expired")
	assert.Contains(t, err.Error(), "invalid_grant")
}

func TestBackwardCompatibility_OldAuthFileWithoutOAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Simulate an old auth file that only has api_key fields.
	auth := &File{
		Providers: map[string]json.RawMessage{
			"anthropic": json.RawMessage(`{"api_key":"sk-ant-old"}`),
			"openai":    json.RawMessage(`{"api_key":"sk-openai-old"}`),
		},
	}
	require.NoError(t, Save(auth))

	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-old", loaded.GetProviderKey("anthropic"))
	assert.Equal(t, "sk-openai-old", loaded.GetProviderKey("openai"))
	assert.Empty(t, loaded.GetOAuthCredential("anthropic").AccessToken)
	assert.Empty(t, loaded.GetOAuthCredential("openai").AccessToken)
	assert.True(t, loaded.HasProviderAuth("anthropic"))
	assert.True(t, loaded.HasProviderAuth("openai"))
}
