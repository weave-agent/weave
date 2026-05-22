package providerhttp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
