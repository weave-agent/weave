package providerretry

import (
	"fmt"
	"time"

	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/retry"
)

// Config holds provider retry configuration. Duration fields are strings
// parsed by Resolve; Jitter is a string validated against retry.JitterMode.
// MaxRetries and Multiplier use pointer types so that zero values can be
// distinguished from "not set" during config merging.
type Config struct {
	MaxRetries *int     `json:"max_retries,omitempty" description:"Maximum retry attempts"`
	BaseDelay  string   `json:"base_delay,omitempty" description:"Base delay between retries"`
	MaxDelay   string   `json:"max_delay,omitempty" description:"Maximum delay between retries"`
	Multiplier *float64 `json:"multiplier,omitempty" description:"Exponential backoff multiplier"`
	Jitter     string   `json:"jitter,omitempty" description:"Jitter mode: full or none"`
}

// DefaultConfig returns the default retry configuration.
func DefaultConfig() Config {
	maxRetries := 5
	multiplier := 2.0

	return Config{
		MaxRetries: &maxRetries,
		BaseDelay:  "1s",
		MaxDelay:   "30s",
		Multiplier: &multiplier,
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

	if c.MaxRetries != nil {
		if *c.MaxRetries < 0 {
			return retry.Config{}, fmt.Errorf("provider %s: invalid max_retries: negative value %d", provider, *c.MaxRetries)
		}

		r.MaxRetries = *c.MaxRetries
	}

	if c.Multiplier != nil {
		if *c.Multiplier < 0 {
			return retry.Config{}, fmt.Errorf("provider %s: invalid multiplier: negative value %v", provider, *c.Multiplier)
		}

		r.Multiplier = *c.Multiplier
	}

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

// mergeConfig returns a new Config where non-nil/non-empty fields from override
// replace the corresponding fields in base. Nil pointers and empty strings mean
// "not set" and do not override.
func mergeConfig(base, override Config) Config {
	result := base

	if override.MaxRetries != nil {
		v := *override.MaxRetries
		result.MaxRetries = &v
	}

	if override.BaseDelay != "" {
		result.BaseDelay = override.BaseDelay
	}

	if override.MaxDelay != "" {
		result.MaxDelay = override.MaxDelay
	}

	if override.Multiplier != nil {
		v := *override.Multiplier
		result.Multiplier = &v
	}

	if override.Jitter != "" {
		result.Jitter = override.Jitter
	}

	return result
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
