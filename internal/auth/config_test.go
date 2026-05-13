package auth

import (
	"encoding/json"
	"os"
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
		Providers: map[string]json.RawMessage{
			"testprovider": json.RawMessage(`{"api_key": "sk-from-file"}`),
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
		Providers: map[string]json.RawMessage{
			"testprovider": json.RawMessage(`{"api_key": "sk-from-file"}`),
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
		Providers: map[string]json.RawMessage{
			"testprovider": json.RawMessage(`{"api_key": "sk-key"}`),
		},
	}
	require.NoError(t, Save(auth))

	t.Setenv("TEST_AUTH_BASE_URL", "https://example.com")

	var target testAuth
	require.NoError(t, LoadProviderAuth("testprovider", &target))
	assert.Equal(t, "sk-key", target.APIKey)
	assert.Equal(t, "https://example.com", target.BaseURL)
}

func TestLoadProviderAuth_ExtraFieldsFromAuthFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	type extendedAuth struct {
		APIKey string `json:"api_key"`
		OrgID  string `json:"org_id"`
	}

	auth := &File{
		Providers: map[string]json.RawMessage{
			"testprovider": json.RawMessage(`{"api_key": "sk-key", "org_id": "org-123"}`),
		},
	}
	require.NoError(t, Save(auth))

	var target extendedAuth
	require.NoError(t, LoadProviderAuth("testprovider", &target))
	assert.Equal(t, "sk-key", target.APIKey)
	assert.Equal(t, "org-123", target.OrgID)
}

func TestLoadProviderAuth_EmptyEnvDoesNotClobberAuthFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	auth := &File{
		Providers: map[string]json.RawMessage{
			"testprovider": json.RawMessage(`{"api_key": "sk-from-file"}`),
		},
	}
	require.NoError(t, Save(auth))

	// Empty env var should NOT override the auth file value.
	t.Setenv("TEST_AUTH_API_KEY", "")

	var target testAuth
	require.NoError(t, LoadProviderAuth("testprovider", &target))
	assert.Equal(t, "sk-from-file", target.APIKey)
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

func TestLoadProviderAuth_AuthFileLoadError(t *testing.T) {
	// Create a directory where auth.json should be, but make it a file instead
	// so Load() fails when trying to create the directory.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Create auth.json as a file with invalid JSON to cause unmarshal error.
	authFilePath := dir + "/.weave/auth.json"
	require.NoError(t, os.MkdirAll(dir+"/.weave", 0o755))
	require.NoError(t, os.WriteFile(authFilePath, []byte("not-json"), 0o600))

	var target testAuth

	// Malformed auth file should not block provider instantiation —
	// env vars may still provide valid auth.
	err := LoadProviderAuth("testprovider", &target)
	require.NoError(t, err)
	assert.Empty(t, target.APIKey)
	assert.Empty(t, target.BaseURL)
}

func TestLoadProviderAuth_InvalidNumericEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	type numericAuth struct {
		Timeout int `json:"timeout" env:"TEST_TIMEOUT"`
	}

	t.Setenv("TEST_TIMEOUT", "not-a-number")

	var target numericAuth

	err := LoadProviderAuth("test", &target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse int")
}

func TestLoadProviderAuth_InvalidBoolEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	type boolAuth struct {
		Enabled bool `json:"enabled" env:"TEST_ENABLED"`
	}

	t.Setenv("TEST_ENABLED", "not-a-bool")

	var target boolAuth

	err := LoadProviderAuth("test", &target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse bool")
}

func TestLoadProviderAuth_NestedStruct(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	type nestedAuth struct {
		Inner struct {
			APIKey string `json:"api_key" env:"NESTED_API_KEY"`
		}
	}

	t.Setenv("NESTED_API_KEY", "nested-key")

	var target nestedAuth
	require.NoError(t, LoadProviderAuth("test", &target))
	assert.Equal(t, "nested-key", target.Inner.APIKey)
}

func TestLoadProviderAuth_UnsupportedFieldKind(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	type sliceAuth struct {
		Scopes []string `json:"scopes" env:"TEST_SCOPES"`
	}

	t.Setenv("TEST_SCOPES", "a,b,c")

	var target sliceAuth

	err := LoadProviderAuth("test", &target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported field kind")
}
