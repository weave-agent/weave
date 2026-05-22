package providerhttp

import (
	"fmt"
	"time"
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
