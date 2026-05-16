package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	require.NoError(t, err)

	// Verifier should be non-empty
	assert.NotEmpty(t, pkce.Verifier)
	assert.NotEmpty(t, pkce.Challenge)
	assert.Equal(t, "S256", pkce.Method)

	// Verifier should be at least 43 chars (base64 of 64 random bytes is ~86 chars)
	assert.GreaterOrEqual(t, len(pkce.Verifier), 43)

	// Challenge should match S256 of verifier
	hash := sha256.Sum256([]byte(pkce.Verifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
	assert.Equal(t, expectedChallenge, pkce.Challenge)
}

func TestGeneratePKCE_Uniqueness(t *testing.T) {
	pkce1, err := GeneratePKCE()
	require.NoError(t, err)

	pkce2, err := GeneratePKCE()
	require.NoError(t, err)

	assert.NotEqual(t, pkce1.Verifier, pkce2.Verifier)
	assert.NotEqual(t, pkce1.Challenge, pkce2.Challenge)
}

func TestGenerateState(t *testing.T) {
	state, err := GenerateState()
	require.NoError(t, err)
	assert.NotEmpty(t, state)

	// Should be base64url encoded (no padding, no +/)
	assert.NotContains(t, state, "+")
	assert.NotContains(t, state, "/")
	assert.NotContains(t, state, "=")
}

func TestGenerateState_Uniqueness(t *testing.T) {
	state1, err := GenerateState()
	require.NoError(t, err)

	state2, err := GenerateState()
	require.NoError(t, err)

	assert.NotEqual(t, state1, state2)
}

func TestAuthorizationCodeURL(t *testing.T) {
	pkce := PKCE{
		Verifier:  "test-verifier",
		Challenge: "test-challenge",
		Method:    "S256",
	}

	url := AuthorizationCodeURL(
		"https://example.com/authorize",
		"my-client-id",
		"http://localhost:8080/callback",
		"test-state",
		pkce,
		[]string{"read", "write"},
	)

	assert.True(t, strings.HasPrefix(url, "https://example.com/authorize?"))
	assert.Contains(t, url, "response_type=code")
	assert.Contains(t, url, "client_id=my-client-id")
	assert.Contains(t, url, "redirect_uri=http%3A%2F%2Flocalhost%3A8080%2Fcallback")
	assert.Contains(t, url, "state=test-state")
	assert.Contains(t, url, "code_challenge=test-challenge")
	assert.Contains(t, url, "code_challenge_method=S256")
	assert.Contains(t, url, "scope=read+write")
}

func TestAuthorizationCodeURL_NoScopes(t *testing.T) {
	pkce := PKCE{
		Verifier:  "test-verifier",
		Challenge: "test-challenge",
		Method:    "S256",
	}

	url := AuthorizationCodeURL(
		"https://example.com/authorize",
		"my-client-id",
		"http://localhost:8080/callback",
		"test-state",
		pkce,
		nil,
	)

	assert.NotContains(t, url, "scope=")
}

func TestAuthorizationCodeURL_InvalidURL(t *testing.T) {
	pkce := PKCE{Verifier: "v", Challenge: "c", Method: "S256"}
	url := AuthorizationCodeURL("://invalid", "id", "http://localhost/callback", "state", pkce, nil)
	// Should return the raw URL on parse failure
	assert.Equal(t, "://invalid", url)
}

func TestExchangeAuthorizationCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		err := r.ParseForm()
		assert.NoError(t, err)

		assert.Equal(t, "authorization_code", r.FormValue("grant_type"))
		assert.Equal(t, "my-client-id", r.FormValue("client_id"))
		assert.Equal(t, "auth-code-123", r.FormValue("code"))
		assert.Equal(t, "http://localhost:8080/callback", r.FormValue("redirect_uri"))
		assert.Equal(t, "test-verifier", r.FormValue("code_verifier"))

		resp := map[string]any{
			"access_token":  "at-123",
			"refresh_token": "rt-456",
			"token_type":    "bearer",
			"expires_in":    3600,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	pkce := PKCE{Verifier: "test-verifier", Challenge: "test-challenge", Method: "S256"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ExchangeAuthorizationCode(ctx, server.URL, "my-client-id", "auth-code-123", "http://localhost:8080/callback", pkce)
	require.NoError(t, err)
	assert.Equal(t, "at-123", result.AccessToken)
	assert.Equal(t, "rt-456", result.RefreshToken)
	assert.Equal(t, "bearer", result.TokenType)
	assert.Equal(t, 3600, result.ExpiresIn)
}

func TestExchangeAuthorizationCode_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)

		resp := map[string]any{
			"error":             "invalid_grant",
			"error_description": "The provided authorization grant is invalid",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	pkce := PKCE{Verifier: "v", Challenge: "c", Method: "S256"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ExchangeAuthorizationCode(ctx, server.URL, "id", "code", "http://localhost/callback", pkce)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_grant")
	assert.Contains(t, err.Error(), "The provided authorization grant is invalid")
}

func TestExchangeAuthorizationCode_HTTPError(t *testing.T) {
	pkce := PKCE{Verifier: "v", Challenge: "c", Method: "S256"}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Use a non-routable address to force a connection error
	_, err := ExchangeAuthorizationCode(ctx, "http://127.0.0.1:1/token", "id", "code", "http://localhost/callback", pkce)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token request failed")
}

func TestRunAuthorizationCodeFlow_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// No callback server will receive a request, so it should time out
	_, err := RunAuthorizationCodeFlow(ctx, "https://example.com/authorize", "https://example.com/token", "client-id", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestOpenBrowser_NonExistentURL(t *testing.T) {
	// This test verifies OpenBrowser doesn't panic on an invalid URL.
	// The browser command will fail but we just need to ensure it returns an error.
	err := OpenBrowser("http://127.0.0.1:1/this-will-fail")
	// On some systems the command might start successfully even if the URL is bad,
	// so we don't assert on the error.
	_ = err
}
