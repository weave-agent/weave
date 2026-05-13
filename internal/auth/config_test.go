package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testAuth struct {
	APIKey  string `json:"api_key" env:"TEST_AUTH_API_KEY"`
	BaseURL string `json:"base_url" env:"TEST_AUTH_BASE_URL"`
}

func TestLoadProviderAuth_FromAuthFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	auth := &File{
		Providers: map[string]ProviderAuth{
			//nolint:gosec // G101 — test credential
			"testprovider": {APIKey: "sk-from-file"},
		},
	}
	require.NoError(t, Save(auth))

	var target testAuth
	require.NoError(t, LoadProviderAuth("testprovider", &target))
	assert.Equal(t, "sk-from-file", target.APIKey)
}

func TestLoadProviderAuth_FromEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	t.Setenv("TEST_AUTH_API_KEY", "sk-from-env")

	var target testAuth
	require.NoError(t, LoadProviderAuth("testprovider", &target))
	assert.Equal(t, "sk-from-env", target.APIKey)
}

func TestLoadProviderAuth_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	auth := &File{
		Providers: map[string]ProviderAuth{
			//nolint:gosec // G101 — test credential
			"testprovider": {APIKey: "sk-from-file"},
		},
	}
	require.NoError(t, Save(auth))

	t.Setenv("TEST_AUTH_API_KEY", "sk-from-env")

	var target testAuth
	require.NoError(t, LoadProviderAuth("testprovider", &target))
	assert.Equal(t, "sk-from-env", target.APIKey)
}

func TestLoadProviderAuth_MissingProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	var target testAuth
	require.NoError(t, LoadProviderAuth("nonexistent", &target))
	assert.Empty(t, target.APIKey)
	assert.Empty(t, target.BaseURL)
}

func TestLoadProviderAuth_MultipleFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	auth := &File{
		Providers: map[string]ProviderAuth{
			"testprovider": {APIKey: "sk-key"},
		},
	}
	require.NoError(t, Save(auth))

	t.Setenv("TEST_AUTH_BASE_URL", "https://example.com")

	var target testAuth
	require.NoError(t, LoadProviderAuth("testprovider", &target))
	assert.Equal(t, "sk-key", target.APIKey)
	assert.Equal(t, "https://example.com", target.BaseURL)
}

func TestLoadProviderAuth_NilTarget(t *testing.T) {
	err := LoadProviderAuth("test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target is nil")
}

func TestLoadProviderAuth_NonPointer(t *testing.T) {
	var target testAuth

	err := LoadProviderAuth("test", target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-nil pointer")
}

func TestLoadProviderAuth_NonStruct(t *testing.T) {
	var target string

	err := LoadProviderAuth("test", &target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "struct")
}

func TestLoadProviderAuth_NumericAndBoolFields(t *testing.T) {
	type complexAuth struct {
		APIKey      string  `json:"api_key" env:"COMPLEX_API_KEY"`
		Timeout     int     `json:"timeout" env:"COMPLEX_TIMEOUT"`
		Enabled     bool    `json:"enabled" env:"COMPLEX_ENABLED"`
		MaxTokens   uint    `json:"max_tokens" env:"COMPLEX_MAX_TOKENS"`
		Temperature float64 `json:"temperature" env:"COMPLEX_TEMPERATURE"`
	}

	dir := t.TempDir()
	t.Setenv("HOME", dir)

	t.Setenv("COMPLEX_API_KEY", "sk-key")
	t.Setenv("COMPLEX_TIMEOUT", "30")
	t.Setenv("COMPLEX_ENABLED", "true")
	t.Setenv("COMPLEX_MAX_TOKENS", "4096")
	t.Setenv("COMPLEX_TEMPERATURE", "0.7")

	var target complexAuth
	require.NoError(t, LoadProviderAuth("test", &target))
	assert.Equal(t, "sk-key", target.APIKey)
	assert.Equal(t, 30, target.Timeout)
	assert.True(t, target.Enabled)
	assert.Equal(t, uint(4096), target.MaxTokens)
	assert.InDelta(t, 0.7, target.Temperature, 0.001)
}

func TestFile_GetProviderConfig_Exists(t *testing.T) {
	auth := &File{
		Providers: map[string]ProviderAuth{
			"anthropic": {APIKey: "sk-ant-123"},
		},
	}

	cfg, err := auth.GetProviderConfig("anthropic")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "sk-ant-123", cfg["api_key"])
}

func TestFile_GetProviderConfig_Missing(t *testing.T) {
	auth := &File{Providers: map[string]ProviderAuth{}}

	cfg, err := auth.GetProviderConfig("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}
