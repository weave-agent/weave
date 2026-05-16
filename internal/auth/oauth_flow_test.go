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
		nil,
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
		nil,
	)

	assert.NotContains(t, url, "scope=")
}

func TestAuthorizationCodeURL_InvalidURL(t *testing.T) {
	pkce := PKCE{Verifier: "v", Challenge: "c", Method: "S256"}
	url := AuthorizationCodeURL("://invalid", "id", "http://localhost/callback", "state", pkce, nil, nil)
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

func TestRefreshToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			return
		}

		assert.Equal(t, "refresh_token", r.FormValue("grant_type"))
		assert.Equal(t, "client-id", r.FormValue("client_id"))
		assert.Equal(t, "rt-old", r.FormValue("refresh_token"))

		resp := map[string]any{
			"access_token":  "at-new",
			"refresh_token": "rt-new",
			"token_type":    "bearer",
			"expires_in":    3600,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := RefreshToken(ctx, server.URL, "client-id", "rt-old")
	require.NoError(t, err)
	assert.Equal(t, "at-new", result.AccessToken)
	assert.Equal(t, "rt-new", result.RefreshToken)
	assert.Equal(t, "bearer", result.TokenType)
	assert.Equal(t, 3600, result.ExpiresIn)
}

func TestRefreshToken_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "invalid_grant",
			"error_description": "Refresh token expired",
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RefreshToken(ctx, server.URL, "client-id", "rt-old")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token refresh failed")
	assert.Contains(t, err.Error(), "invalid_grant")
	assert.Contains(t, err.Error(), "Refresh token expired")
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
	_, err := RunAuthorizationCodeFlow(ctx, "https://example.com/authorize", "https://example.com/token", "client-id", "", nil, nil)
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

func TestRequestDeviceCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		err := r.ParseForm()
		assert.NoError(t, err)
		assert.Equal(t, "my-client-id", r.FormValue("client_id"))
		assert.Equal(t, "read write", r.FormValue("scope"))

		resp := map[string]any{
			"device_code":      "dc-123",
			"user_code":        "ABCD-EFGH",
			"verification_uri": "https://example.com/verify",
			"expires_in":       900,
			"interval":         5,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := RequestDeviceCode(ctx, server.URL, "my-client-id", []string{"read", "write"})
	require.NoError(t, err)
	assert.Equal(t, "dc-123", result.DeviceCode)
	assert.Equal(t, "ABCD-EFGH", result.UserCode)
	assert.Equal(t, "https://example.com/verify", result.VerificationURI)
	assert.Equal(t, 900, result.ExpiresIn)
	assert.Equal(t, 5, result.Interval)
}

func TestRequestDeviceCode_Success_VerificationURL(t *testing.T) {
	// Some providers use "verification_url" instead of "verification_uri".
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"device_code":      "dc-456",
			"user_code":        "WXYZ-1234",
			"verification_url": "https://example.com/device",
			"expires_in":       600,
			"interval":         5,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := RequestDeviceCode(ctx, server.URL, "client-id", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/device", result.VerificationURLOrURI())
}

func TestRequestDeviceCode_Success_VerificationURIComplete(t *testing.T) {
	// verification_uri_complete should be preferred over verification_uri.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"device_code":               "dc-789",
			"user_code":                 "TEST-1234",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=TEST-1234",
			"expires_in":                600,
			"interval":                  5,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := RequestDeviceCode(ctx, server.URL, "client-id", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/verify?code=TEST-1234", result.VerificationURLOrURI())
}

func TestRequestDeviceCode_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)

		resp := map[string]any{
			"error":             "invalid_client",
			"error_description": "Client authentication failed",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RequestDeviceCode(ctx, server.URL, "bad-client", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_client")
	assert.Contains(t, err.Error(), "Client authentication failed")
}

func TestRequestDeviceCode_HTTPError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := RequestDeviceCode(ctx, "http://127.0.0.1:1/device", "id", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "device code request failed")
}

func TestPollDeviceToken_Success(t *testing.T) {
	pollCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		err := r.ParseForm()
		assert.NoError(t, err)
		assert.Equal(t, "urn:ietf:params:oauth:grant-type:device_code", r.FormValue("grant_type"))
		assert.Equal(t, "my-client-id", r.FormValue("client_id"))
		assert.Equal(t, "dc-123", r.FormValue("device_code"))

		pollCount++
		if pollCount < 3 {
			w.WriteHeader(http.StatusBadRequest)

			resp := map[string]any{"error": "authorization_pending"}
			_ = json.NewEncoder(w).Encode(resp)

			return
		}

		resp := map[string]any{
			"access_token":  "at-789",
			"refresh_token": "rt-012",
			"token_type":    "bearer",
			"expires_in":    3600,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := PollDeviceToken(ctx, server.URL, "my-client-id", "dc-123", 1)
	require.NoError(t, err)
	assert.Equal(t, "at-789", result.AccessToken)
	assert.Equal(t, "rt-012", result.RefreshToken)
	assert.Equal(t, "bearer", result.TokenType)
	assert.Equal(t, 3600, result.ExpiresIn)
	assert.Equal(t, 3, pollCount)
}

func TestPollDeviceToken_SlowDown(t *testing.T) {
	pollCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pollCount++
		if pollCount < 2 {
			w.WriteHeader(http.StatusBadRequest)

			resp := map[string]any{"error": "slow_down"}
			_ = json.NewEncoder(w).Encode(resp)

			return
		}

		resp := map[string]any{
			"access_token": "at-slow",
			"token_type":   "bearer",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start with 1-second interval; slow_down adds 5 seconds each time.
	result, err := PollDeviceToken(ctx, server.URL, "client", "dc", 1)
	require.NoError(t, err)
	assert.Equal(t, "at-slow", result.AccessToken)
	assert.Equal(t, 2, pollCount)
}

func TestPollDeviceToken_AccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)

		resp := map[string]any{
			"error":             "access_denied",
			"error_description": "User denied the request",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := PollDeviceToken(ctx, server.URL, "client", "dc", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_denied")
	assert.Contains(t, err.Error(), "User denied the request")
}

func TestPollDeviceToken_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)

		resp := map[string]any{"error": "authorization_pending"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := PollDeviceToken(ctx, server.URL, "client", "dc", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestPollDeviceToken_ExpiredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)

		resp := map[string]any{"error": "expired_token"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := PollDeviceToken(ctx, server.URL, "client", "dc", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired_token")
}
