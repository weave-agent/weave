package providerretry

import (
	"fmt"
	"time"

	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/retry"
)

// Config holds provider retry configuration. Duration fields are strings
// parsed by Resolve; Jitter is a string validated against retry.JitterMode.
type Config struct {
	MaxRetries int     `json:"max_retries,omitempty" env:"MAX_RETRIES" description:"Maximum retry attempts"`
	BaseDelay  string  `json:"base_delay,omitempty" env:"BASE_DELAY" description:"Base delay between retries"`
	MaxDelay   string  `json:"max_delay,omitempty" env:"MAX_DELAY" description:"Maximum delay between retries"`
	Multiplier float64 `json:"multiplier,omitempty" env:"MULTIPLIER" description:"Exponential backoff multiplier"`
	Jitter     string  `json:"jitter,omitempty" env:"JITTER" description:"Jitter mode: full or none"`
}

// DefaultConfig returns the default retry configuration.
func DefaultConfig() Config {
	return Config{
		MaxRetries: 5,
		BaseDelay:  "1s",
		MaxDelay:   "30s",
		Multiplier: 2,
		Jitter:     "full",
	}
}

// Resolve parses the duration fields and validates jitter, returning a
// retry.Config suitable for use with sdk/retry.
func (c Config) Resolve(provider string) (retry.Config, error) {
	var r retry.Config

	var err error

	r.BaseDelay, err = parseDurationField("base_delay", c.BaseDelay)
	if err != nil {
		return retry.Config{}, fmt.Errorf("provider %s: %w", provider, err)
	}

	r.MaxDelay, err = parseDurationField("max_delay", c.MaxDelay)
	if err != nil {
		return retry.Config{}, fmt.Errorf("provider %s: %w", provider, err)
	}

	r.MaxRetries = c.MaxRetries

	if c.Multiplier < 0 {
		return retry.Config{}, fmt.Errorf("provider %s: invalid multiplier: negative value %v", provider, c.Multiplier)
	}

	r.Multiplier = c.Multiplier

	if c.Jitter != "" && c.Jitter != string(retry.JitterNone) && c.Jitter != string(retry.JitterFull) {
		return retry.Config{}, fmt.Errorf("provider %s: invalid jitter: %q", provider, c.Jitter)
	}

	if c.Jitter != "" {
		r.Jitter = retry.JitterMode(c.Jitter)
	}

	return r, nil
}

// ForProvider resolves retry configuration for a named provider and returns
// a sdk/retry.Config alongside the raw Config.
// Resolution order: code defaults → providers.defaults.retry → providers.<name>.retry.
// Partial overrides are supported: a provider only needs to specify the fields
// it wants to override; all other fields inherit from defaults.
func ForProvider(cfg sdk.Config, provider string) (retry.Config, Config, error) {
	result := DefaultConfig()

	var defaults struct {
		Retry Config `json:"retry"`
	}
	if err := cfg.ExtensionConfig("providers", "defaults", &defaults); err != nil {
		return retry.Config{}, Config{}, fmt.Errorf("load provider defaults: %w", err)
	}

	result = mergeConfig(result, defaults.Retry)

	var specific struct {
		Retry Config `json:"retry"`
	}
	if err := cfg.ExtensionConfig("providers", provider, &specific); err != nil {
		return retry.Config{}, Config{}, fmt.Errorf("load provider %s: %w", provider, err)
	}

	result = mergeConfig(result, specific.Retry)

	r, err := result.Resolve(provider)
	if err != nil {
		return retry.Config{}, Config{}, err
	}

	return r, result, nil
}

// mergeConfig returns a new Config where non-zero fields from override replace
// the corresponding fields in base. For strings, empty means "not set".
// For float64, zero means "not set" because multiplier=0 is not useful.
func mergeConfig(base, override Config) Config {
	if override.MaxRetries != 0 {
		base.MaxRetries = override.MaxRetries
	}

	if override.BaseDelay != "" {
		base.BaseDelay = override.BaseDelay
	}

	if override.MaxDelay != "" {
		base.MaxDelay = override.MaxDelay
	}

	if override.Multiplier != 0 {
		base.Multiplier = override.Multiplier
	}

	if override.Jitter != "" {
		base.Jitter = override.Jitter
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
