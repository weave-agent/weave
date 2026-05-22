package providerhttp

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/weave-agent/weave/sdk"
)

// Config holds provider HTTP transport configuration as duration strings.
// These values are parsed and converted into time.Duration by Resolve.
type Config struct {
	DialTimeout           string `json:"dial_timeout,omitempty" env:"DIAL_TIMEOUT" description:"Dial context timeout"`
	TLSHandshakeTimeout   string `json:"tls_handshake_timeout,omitempty" env:"TLS_HANDSHAKE_TIMEOUT" description:"TLS handshake timeout"`
	ResponseHeaderTimeout string `json:"response_header_timeout,omitempty" env:"RESPONSE_HEADER_TIMEOUT" description:"Response header timeout"`
	IdleConnTimeout       string `json:"idle_conn_timeout,omitempty" env:"IDLE_CONN_TIMEOUT" description:"Idle connection timeout"`
}

// resolved holds parsed time.Duration values from Config.
type resolved struct {
	dialTimeout           time.Duration
	tlsHandshakeTimeout   time.Duration
	responseHeaderTimeout time.Duration
	idleConnTimeout       time.Duration
}

// DefaultConfig returns the default HTTP transport configuration with
// sensible timeout values.
func DefaultConfig() Config {
	return Config{
		DialTimeout:           "10s",
		TLSHandshakeTimeout:   "10s",
		ResponseHeaderTimeout: "60s",
		IdleConnTimeout:       "90s",
	}
}

// Resolve parses the duration string fields in Config into time.Duration values.
// It returns an error if any field contains an invalid or negative duration.
func (c Config) Resolve(provider string) (resolved, error) {
	var r resolved

	var err error

	r.dialTimeout, err = parseDurationField("dial_timeout", c.DialTimeout)
	if err != nil {
		return resolved{}, fmt.Errorf("provider %s: %w", provider, err)
	}

	r.tlsHandshakeTimeout, err = parseDurationField("tls_handshake_timeout", c.TLSHandshakeTimeout)
	if err != nil {
		return resolved{}, fmt.Errorf("provider %s: %w", provider, err)
	}

	r.responseHeaderTimeout, err = parseDurationField("response_header_timeout", c.ResponseHeaderTimeout)
	if err != nil {
		return resolved{}, fmt.Errorf("provider %s: %w", provider, err)
	}

	r.idleConnTimeout, err = parseDurationField("idle_conn_timeout", c.IdleConnTimeout)
	if err != nil {
		return resolved{}, fmt.Errorf("provider %s: %w", provider, err)
	}

	return r, nil
}

// NewClient creates an *http.Client with an http.Transport configured from
// the provided Config. The client-level Timeout is intentionally left at zero
// to preserve streaming compatibility. All timeouts are set on the transport
// only.
func NewClient(cfg Config) (*http.Client, error) {
	r, err := cfg.Resolve("")
	if err != nil {
		return nil, err
	}

	return newClientFromResolved(r), nil
}

func newClientFromResolved(r resolved) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: r.dialTimeout,
		}).DialContext,
		TLSHandshakeTimeout:   r.tlsHandshakeTimeout,
		ResponseHeaderTimeout: r.responseHeaderTimeout,
		IdleConnTimeout:       r.idleConnTimeout,
	}

	return &http.Client{
		Transport: transport,
	}
}

// ForProvider resolves HTTP transport configuration for a named provider and
// returns a configured *http.Client alongside the raw Config.
// Resolution order: code defaults → providers.defaults.http → providers.<name>.http.
// Partial overrides are supported: a provider only needs to specify the fields
// it wants to override; all other fields inherit from defaults.
func ForProvider(cfg sdk.Config, provider string) (*http.Client, Config, error) {
	result := DefaultConfig()

	var defaults struct {
		HTTP Config `json:"http"`
	}
	if err := cfg.ExtensionConfig("providers", "defaults", &defaults); err != nil {
		return nil, Config{}, fmt.Errorf("load provider defaults: %w", err)
	}

	result = mergeConfig(result, defaults.HTTP)

	var specific struct {
		HTTP Config `json:"http"`
	}
	if err := cfg.ExtensionConfig("providers", provider, &specific); err != nil {
		return nil, Config{}, fmt.Errorf("load provider %s: %w", provider, err)
	}

	result = mergeConfig(result, specific.HTTP)

	r, err := result.Resolve(provider)
	if err != nil {
		return nil, Config{}, err
	}

	return newClientFromResolved(r), result, nil
}

// mergeConfig returns a new Config where non-empty fields from override
// replace the corresponding fields in base. Empty string means "not set"
// and does not override.
func mergeConfig(base, override Config) Config {
	if override.DialTimeout != "" {
		base.DialTimeout = override.DialTimeout
	}

	if override.TLSHandshakeTimeout != "" {
		base.TLSHandshakeTimeout = override.TLSHandshakeTimeout
	}

	if override.ResponseHeaderTimeout != "" {
		base.ResponseHeaderTimeout = override.ResponseHeaderTimeout
	}

	if override.IdleConnTimeout != "" {
		base.IdleConnTimeout = override.IdleConnTimeout
	}

	return base
}

func parseDurationField(field, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}

	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", field, err)
	}

	if d < 0 {
		return 0, fmt.Errorf("invalid %s: negative duration %s", field, value)
	}

	return d, nil
}
