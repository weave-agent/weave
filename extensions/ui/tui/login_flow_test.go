package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollDeviceCodeCmd_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"access_token":  "at-test",
			"refresh_token": "rt-test",
			"token_type":    "bearer",
			"expires_in":    3600,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cmd := pollDeviceCodeCmd(context.Background(), "test-provider", "dc-123", 1, server.URL, "client-id", 1)
	msg := cmd()

	result, ok := msg.(LoginFlowResultMsg)
	require.True(t, ok)
	assert.Equal(t, "test-provider", result.Provider)
	assert.Equal(t, 1, result.Gen)
	require.NoError(t, result.Error)
	assert.Equal(t, "at-test", result.Credential.AccessToken)
	assert.Equal(t, "rt-test", result.Credential.RefreshToken)
	assert.Equal(t, "bearer", result.Credential.TokenType)
	assert.False(t, result.Credential.ExpiresAt.IsZero())
}

func TestPollDeviceCodeCmd_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)

		resp := map[string]any{"error": "access_denied"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cmd := pollDeviceCodeCmd(context.Background(), "test-provider", "dc-123", 1, server.URL, "client-id", 2)
	msg := cmd()

	result, ok := msg.(LoginFlowResultMsg)
	require.True(t, ok)
	assert.Equal(t, "test-provider", result.Provider)
	assert.Equal(t, 2, result.Gen)
	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "access_denied")
}
