package providerhttp

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weave-agent/weave/sdk"
)

// stubConfig is a test double for sdk.Config that returns pre-configured
// provider data from ExtensionConfig.
type stubConfig struct {
	providers map[string]map[string]any
	sdk.NoopConfig
}

func (s *stubConfig) ExtensionConfig(scope, name string, target any) error {
	if scope != "providers" {
		return fmt.Errorf("unknown scope %q", scope)
	}

	section, ok := s.providers[name]
	if !ok {
		return nil
	}

	data, err := json.Marshal(section)
	if err != nil {
		return fmt.Errorf("marshal stub config: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal stub config: %w", err)
	}

	return nil
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "10s", cfg.DialTimeout)
	assert.Equal(t, "10s", cfg.TLSHandshakeTimeout)
	assert.Equal(t, "60s", cfg.ResponseHeaderTimeout)
	assert.Equal(t, "90s", cfg.IdleConnTimeout)
}

func TestResolve_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)

	assert.Equal(t, 10*time.Second, r.dialTimeout)
	assert.Equal(t, 10*time.Second, r.tlsHandshakeTimeout)
	assert.Equal(t, 60*time.Second, r.responseHeaderTimeout)
	assert.Equal(t, 90*time.Second, r.idleConnTimeout)
}

func TestResolve_CustomValues(t *testing.T) {
	cfg := Config{
		DialTimeout:           "5s",
		TLSHandshakeTimeout:   "15s",
		ResponseHeaderTimeout: "120s",
		IdleConnTimeout:       "180s",
	}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)

	assert.Equal(t, 5*time.Second, r.dialTimeout)
	assert.Equal(t, 15*time.Second, r.tlsHandshakeTimeout)
	assert.Equal(t, 120*time.Second, r.responseHeaderTimeout)
	assert.Equal(t, 180*time.Second, r.idleConnTimeout)
}

func TestResolve_EmptyFieldsParseToZero(t *testing.T) {
	cfg := Config{}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)

	assert.Equal(t, time.Duration(0), r.dialTimeout)
	assert.Equal(t, time.Duration(0), r.tlsHandshakeTimeout)
	assert.Equal(t, time.Duration(0), r.responseHeaderTimeout)
	assert.Equal(t, time.Duration(0), r.idleConnTimeout)
}

func TestResolve_InvalidDuration(t *testing.T) {
	cfg := Config{DialTimeout: "not-a-duration"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid dial_timeout")
}

func TestResolve_InvalidTLSHandshakeTimeout(t *testing.T) {
	cfg := Config{TLSHandshakeTimeout: "abc"}

	_, err := cfg.Resolve("anthropic")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider anthropic")
	assert.Contains(t, err.Error(), "invalid tls_handshake_timeout")
}

func TestResolve_InvalidResponseHeaderTimeout(t *testing.T) {
	cfg := Config{ResponseHeaderTimeout: "xyz"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid response_header_timeout")
}

func TestResolve_InvalidIdleConnTimeout(t *testing.T) {
	cfg := Config{IdleConnTimeout: "bad"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid idle_conn_timeout")
}

func TestResolve_NegativeDuration(t *testing.T) {
	cfg := Config{DialTimeout: "-5s"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid dial_timeout")
	assert.Contains(t, err.Error(), "negative duration")
}

func TestResolve_NegativeTLSHandshakeTimeout(t *testing.T) {
	cfg := Config{TLSHandshakeTimeout: "-1m"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative duration")
}

func TestResolve_NegativeResponseHeaderTimeout(t *testing.T) {
	cfg := Config{ResponseHeaderTimeout: "-30s"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative duration")
}

func TestResolve_NegativeIdleConnTimeout(t *testing.T) {
	cfg := Config{IdleConnTimeout: "-10s"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative duration")
}

func TestResolve_PartialConfig(t *testing.T) {
	cfg := Config{
		DialTimeout: "5s",
	}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)

	assert.Equal(t, 5*time.Second, r.dialTimeout)
	assert.Equal(t, time.Duration(0), r.tlsHandshakeTimeout)
	assert.Equal(t, time.Duration(0), r.responseHeaderTimeout)
	assert.Equal(t, time.Duration(0), r.idleConnTimeout)
}

func TestResolve_MillisecondDuration(t *testing.T) {
	cfg := Config{DialTimeout: "500ms"}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)
	assert.Equal(t, 500*time.Millisecond, r.dialTimeout)
}

func TestResolve_MinuteDuration(t *testing.T) {
	cfg := Config{IdleConnTimeout: "2m30s"}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)
	assert.Equal(t, 150*time.Second, r.idleConnTimeout)
}

func TestForProvider_UsesCodeDefaults(t *testing.T) {
	cfg := &stubConfig{providers: map[string]map[string]any{}}

	got, err := ForProvider(cfg, "openai")
	require.NoError(t, err)

	assert.Equal(t, "10s", got.DialTimeout)
	assert.Equal(t, "10s", got.TLSHandshakeTimeout)
	assert.Equal(t, "60s", got.ResponseHeaderTimeout)
	assert.Equal(t, "90s", got.IdleConnTimeout)
}

func TestForProvider_GlobalDefaultsMerge(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"defaults": {
				"http": map[string]any{
					"dial_timeout":            "20s",
					"response_header_timeout": "120s",
				},
			},
		},
	}

	got, err := ForProvider(cfg, "openai")
	require.NoError(t, err)

	assert.Equal(t, "20s", got.DialTimeout, "global default should override code default")
	assert.Equal(t, "10s", got.TLSHandshakeTimeout, "unspecified field should inherit code default")
	assert.Equal(t, "120s", got.ResponseHeaderTimeout, "global default should override code default")
	assert.Equal(t, "90s", got.IdleConnTimeout, "unspecified field should inherit code default")
}

func TestForProvider_ProviderSpecificPartialOverride(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"defaults": {
				"http": map[string]any{
					"dial_timeout":            "20s",
					"response_header_timeout": "120s",
				},
			},
			"openai": {
				"http": map[string]any{
					"response_header_timeout": "180s",
				},
			},
		},
	}

	got, err := ForProvider(cfg, "openai")
	require.NoError(t, err)

	assert.Equal(t, "20s", got.DialTimeout, "should inherit from global defaults")
	assert.Equal(t, "10s", got.TLSHandshakeTimeout, "should inherit code default")
	assert.Equal(t, "180s", got.ResponseHeaderTimeout, "provider should override global default")
	assert.Equal(t, "90s", got.IdleConnTimeout, "should inherit code default")
}

func TestForProvider_ProviderSpecificFullOverride(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"openai": {
				"http": map[string]any{
					"dial_timeout":            "5s",
					"tls_handshake_timeout":   "15s",
					"response_header_timeout": "30s",
					"idle_conn_timeout":       "45s",
				},
			},
		},
	}

	got, err := ForProvider(cfg, "openai")
	require.NoError(t, err)

	assert.Equal(t, "5s", got.DialTimeout)
	assert.Equal(t, "15s", got.TLSHandshakeTimeout)
	assert.Equal(t, "30s", got.ResponseHeaderTimeout)
	assert.Equal(t, "45s", got.IdleConnTimeout)
}

func TestForProvider_InvalidDefaultDuration(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"defaults": {
				"http": map[string]any{
					"dial_timeout": "not-a-duration",
				},
			},
		},
	}

	_, err := ForProvider(cfg, "openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid dial_timeout")
}

func TestForProvider_InvalidProviderDuration(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"openai": {
				"http": map[string]any{
					"idle_conn_timeout": "bad-value",
				},
			},
		},
	}

	_, err := ForProvider(cfg, "openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid idle_conn_timeout")
}

func TestForProvider_NegativeProviderDuration(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"openai": {
				"http": map[string]any{
					"tls_handshake_timeout": "-5s",
				},
			},
		},
	}

	_, err := ForProvider(cfg, "openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "negative duration")
}

func TestMergeConfig(t *testing.T) {
	base := Config{
		DialTimeout:           "10s",
		TLSHandshakeTimeout:   "10s",
		ResponseHeaderTimeout: "60s",
		IdleConnTimeout:       "90s",
	}

	t.Run("empty override keeps base", func(t *testing.T) {
		got := mergeConfig(base, Config{})
		assert.Equal(t, base, got)
	})

	t.Run("partial override merges only set fields", func(t *testing.T) {
		override := Config{DialTimeout: "20s"}
		got := mergeConfig(base, override)
		assert.Equal(t, "20s", got.DialTimeout)
		assert.Equal(t, "10s", got.TLSHandshakeTimeout)
		assert.Equal(t, "60s", got.ResponseHeaderTimeout)
		assert.Equal(t, "90s", got.IdleConnTimeout)
	})

	t.Run("full override replaces all", func(t *testing.T) {
		override := Config{
			DialTimeout:           "1s",
			TLSHandshakeTimeout:   "2s",
			ResponseHeaderTimeout: "3s",
			IdleConnTimeout:       "4s",
		}
		got := mergeConfig(base, override)
		assert.Equal(t, override, got)
	})
}
