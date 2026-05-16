package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartCallbackServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cs, err := StartCallbackServer(ctx, "")
	require.NoError(t, err)
	require.NotNil(t, cs)

	redirectURI := cs.RedirectURI()
	assert.True(t, strings.HasPrefix(redirectURI, "http://127.0.0.1:"), "redirect URI should be localhost: %s", redirectURI)
	assert.True(t, strings.HasSuffix(redirectURI, "/callback"), "redirect URI should end with /callback: %s", redirectURI)
}

func TestCallbackServer_ReceivesAuthorizationCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cs, err := StartCallbackServer(ctx, "")
	require.NoError(t, err)

	// Simulate the OAuth provider redirecting to the callback with a code.
	resp, err := http.Get(cs.RedirectURI() + "?code=auth-code-123&state=xyz")
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Authentication successful")

	result := <-cs.Result()
	require.NoError(t, result.Error)
	assert.Equal(t, "auth-code-123", result.Code)
	assert.Equal(t, "xyz", result.State)
}

func TestCallbackServer_OAuthError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cs, err := StartCallbackServer(ctx, "")
	require.NoError(t, err)

	resp, err := http.Get(cs.RedirectURI() + "?error=access_denied&error_description=user+denied")
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	result := <-cs.Result()
	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "access_denied")
	assert.Contains(t, result.Error.Error(), "user denied")
}

func TestCallbackServer_MissingCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cs, err := StartCallbackServer(ctx, "")
	require.NoError(t, err)

	resp, err := http.Get(cs.RedirectURI() + "?state=abc")
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	result := <-cs.Result()
	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "missing authorization code")
}

func TestCallbackServer_IgnoresDuplicateRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cs, err := StartCallbackServer(ctx, "")
	require.NoError(t, err)

	// First request succeeds.
	resp1, err := http.Get(cs.RedirectURI() + "?code=first-code")
	require.NoError(t, err)

	_ = resp1.Body.Close()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	result := <-cs.Result()
	require.NoError(t, result.Error)
	assert.Equal(t, "first-code", result.Code)

	// Second request should be rejected since server is shutting down.
	// Give server a moment to start shutdown.
	time.Sleep(50 * time.Millisecond)

	resp2, err := http.Get(cs.RedirectURI() + "?code=second-code")
	if err == nil {
		_ = resp2.Body.Close()
	}

	// Result channel should be closed, no second value.
	select {
	case _, ok := <-cs.Result():
		assert.False(t, ok, "result channel should be closed, not receive another value")
	case <-time.After(100 * time.Millisecond):
		// Channel is closed, which is correct.
	}
}

func TestCallbackServer_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cs, err := StartCallbackServer(ctx, "")
	require.NoError(t, err)

	// Cancel the context to trigger auto-shutdown.
	cancel()

	// Wait a bit for shutdown.
	time.Sleep(100 * time.Millisecond)

	// Server should no longer accept connections.
	resp, err := http.Get(cs.RedirectURI())
	if err == nil {
		_ = resp.Body.Close()
	}

	require.Error(t, err)
}

func TestCallbackServer_ContextTimeout_NoCallback(t *testing.T) {
	// Short timeout so the context cancels before any callback.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cs, err := StartCallbackServer(ctx, "")
	require.NoError(t, err)

	// Wait for context to expire.
	select {
	case result, ok := <-cs.Result():
		if ok {
			// Should not receive a result, only channel close.
			t.Fatalf("expected no result, got: %+v", result)
		}
		// Channel closed without result — correct behavior on timeout.
	case <-time.After(2 * time.Second):
		t.Fatal("result channel should have been closed after context timeout")
	}
}
