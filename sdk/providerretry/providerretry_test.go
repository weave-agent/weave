package providerretry

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/retry"
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

	assert.Equal(t, 5, *cfg.MaxRetries)
	assert.Equal(t, "1s", cfg.BaseDelay)
	assert.Equal(t, "30s", cfg.MaxDelay)
	assert.InDelta(t, 2.0, *cfg.Multiplier, 0.0001)
	assert.Equal(t, "full", cfg.Jitter)
}

func TestResolve_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)

	assert.Equal(t, 5, r.MaxRetries)
	assert.Equal(t, 1*time.Second, r.BaseDelay)
	assert.Equal(t, 30*time.Second, r.MaxDelay)
	assert.InDelta(t, 2.0, r.Multiplier, 0.0001)
	assert.Equal(t, retry.JitterFull, r.Jitter)
}

func TestResolve_CustomValues(t *testing.T) {
	cfg := Config{
		MaxRetries: new(3),
		BaseDelay:  "500ms",
		MaxDelay:   "10s",
		Multiplier: new(1.5),
		Jitter:     "none",
	}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)

	assert.Equal(t, 3, r.MaxRetries)
	assert.Equal(t, 500*time.Millisecond, r.BaseDelay)
	assert.Equal(t, 10*time.Second, r.MaxDelay)
	assert.InDelta(t, 1.5, r.Multiplier, 0.0001)
	assert.Equal(t, retry.JitterNone, r.Jitter)
}

func TestResolve_EmptyFields(t *testing.T) {
	cfg := Config{}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)

	assert.Equal(t, 0, r.MaxRetries)
	assert.Equal(t, time.Duration(0), r.BaseDelay)
	assert.Equal(t, time.Duration(0), r.MaxDelay)
	assert.InDelta(t, 0.0, r.Multiplier, 0.0001)
	assert.Equal(t, retry.JitterMode(""), r.Jitter)
}

func TestResolve_InvalidBaseDelay(t *testing.T) {
	cfg := Config{BaseDelay: "not-a-duration"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid base_delay")
}

func TestResolve_InvalidMaxDelay(t *testing.T) {
	cfg := Config{MaxDelay: "bad"}

	_, err := cfg.Resolve("anthropic")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider anthropic")
	assert.Contains(t, err.Error(), "invalid max_delay")
}

func TestResolve_NegativeBaseDelay(t *testing.T) {
	cfg := Config{BaseDelay: "-5s"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "negative duration")
}

func TestResolve_NegativeMaxDelay(t *testing.T) {
	cfg := Config{MaxDelay: "-1m"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative duration")
}

func TestResolve_InvalidJitter(t *testing.T) {
	cfg := Config{Jitter: "partial"}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid jitter")
	assert.Contains(t, err.Error(), "partial")
}

func TestResolve_NegativeMultiplier(t *testing.T) {
	cfg := Config{Multiplier: new(-1.0)}

	_, err := cfg.Resolve("openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid multiplier")
	assert.Contains(t, err.Error(), "negative")
}

func TestResolve_ZeroMultiplier(t *testing.T) {
	cfg := Config{
		BaseDelay:  "1s",
		Multiplier: new(0.0),
	}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)
	assert.InDelta(t, 0.0, r.Multiplier, 0.0001)
}

func TestForProvider_UsesCodeDefaults(t *testing.T) {
	cfg := &stubConfig{providers: map[string]map[string]any{}}

	r, raw, err := ForProvider(cfg, "openai")
	require.NoError(t, err)

	assert.Equal(t, 5, r.MaxRetries)
	assert.Equal(t, 1*time.Second, r.BaseDelay)
	assert.Equal(t, 30*time.Second, r.MaxDelay)
	assert.InDelta(t, 2.0, r.Multiplier, 0.0001)
	assert.Equal(t, retry.JitterFull, r.Jitter)

	assert.Equal(t, "1s", raw.BaseDelay)
	assert.Equal(t, "30s", raw.MaxDelay)
}

func TestForProvider_GlobalDefaultsMerge(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"defaults": {
				"retry": map[string]any{
					"max_retries": 10,
					"base_delay":  "2s",
				},
			},
		},
	}

	r, raw, err := ForProvider(cfg, "openai")
	require.NoError(t, err)

	assert.Equal(t, 10, r.MaxRetries, "global default should override code default")
	assert.Equal(t, 2*time.Second, r.BaseDelay, "global default should override code default")
	assert.Equal(t, 30*time.Second, r.MaxDelay, "unspecified field should inherit code default")
	assert.InDelta(t, 2.0, r.Multiplier, 0.0001, "unspecified field should inherit code default")
	assert.Equal(t, retry.JitterFull, r.Jitter, "unspecified field should inherit code default")

	assert.Equal(t, 10, *raw.MaxRetries)
	assert.Equal(t, "2s", raw.BaseDelay)
	assert.Equal(t, "30s", raw.MaxDelay)
}

func TestForProvider_ProviderSpecificPartialOverride(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"defaults": {
				"retry": map[string]any{
					"max_retries": 10,
					"base_delay":  "2s",
				},
			},
			"openai": {
				"retry": map[string]any{
					"jitter": "none",
				},
			},
		},
	}

	r, raw, err := ForProvider(cfg, "openai")
	require.NoError(t, err)

	assert.Equal(t, 10, r.MaxRetries, "should inherit from global defaults")
	assert.Equal(t, 2*time.Second, r.BaseDelay, "should inherit from global defaults")
	assert.Equal(t, 30*time.Second, r.MaxDelay, "should inherit code default")
	assert.InDelta(t, 2.0, r.Multiplier, 0.0001, "should inherit code default")
	assert.Equal(t, retry.JitterNone, r.Jitter, "provider should override global default")

	assert.Equal(t, "none", raw.Jitter)
}

func TestForProvider_ProviderSpecificFullOverride(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"openai": {
				"retry": map[string]any{
					"max_retries": 3,
					"base_delay":  "500ms",
					"max_delay":   "5s",
					"multiplier":  1.5,
					"jitter":      "none",
				},
			},
		},
	}

	r, raw, err := ForProvider(cfg, "openai")
	require.NoError(t, err)

	assert.Equal(t, 3, r.MaxRetries)
	assert.Equal(t, 500*time.Millisecond, r.BaseDelay)
	assert.Equal(t, 5*time.Second, r.MaxDelay)
	assert.InDelta(t, 1.5, r.Multiplier, 0.0001)
	assert.Equal(t, retry.JitterNone, r.Jitter)

	assert.Equal(t, 3, *raw.MaxRetries)
	assert.Equal(t, "500ms", raw.BaseDelay)
	assert.Equal(t, "5s", raw.MaxDelay)
	assert.InDelta(t, 1.5, *raw.Multiplier, 0.0001)
	assert.Equal(t, "none", raw.Jitter)
}

func TestForProvider_InvalidDefaultDuration(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"defaults": {
				"retry": map[string]any{
					"base_delay": "not-a-duration",
				},
			},
		},
	}

	_, _, err := ForProvider(cfg, "openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid base_delay")
}

func TestForProvider_InvalidProviderDuration(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"openai": {
				"retry": map[string]any{
					"max_delay": "bad-value",
				},
			},
		},
	}

	_, _, err := ForProvider(cfg, "openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid max_delay")
}

func TestForProvider_InvalidProviderJitter(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"openai": {
				"retry": map[string]any{
					"jitter": "random",
				},
			},
		},
	}

	_, _, err := ForProvider(cfg, "openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid jitter")
}

func TestForProvider_NegativeProviderMultiplier(t *testing.T) {
	cfg := &stubConfig{
		providers: map[string]map[string]any{
			"openai": {
				"retry": map[string]any{
					"multiplier": -1.0,
				},
			},
		},
	}

	_, _, err := ForProvider(cfg, "openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider openai")
	assert.Contains(t, err.Error(), "invalid multiplier")
}

func TestMergeConfig(t *testing.T) {
	base := Config{
		MaxRetries: new(5),
		BaseDelay:  "1s",
		MaxDelay:   "30s",
		Multiplier: new(2.0),
		Jitter:     "full",
	}

	t.Run("empty override keeps base", func(t *testing.T) {
		got := mergeConfig(base, Config{})
		assert.Equal(t, base, got)
	})

	t.Run("partial override merges only set fields", func(t *testing.T) {
		override := Config{MaxRetries: new(10), Jitter: "none"}
		got := mergeConfig(base, override)
		assert.Equal(t, 10, *got.MaxRetries)
		assert.Equal(t, "1s", got.BaseDelay)
		assert.Equal(t, "30s", got.MaxDelay)
		assert.InDelta(t, 2.0, *got.Multiplier, 0.0001)
		assert.Equal(t, "none", got.Jitter)
	})

	t.Run("full override replaces all", func(t *testing.T) {
		override := Config{
			MaxRetries: new(3),
			BaseDelay:  "500ms",
			MaxDelay:   "5s",
			Multiplier: new(1.5),
			Jitter:     "none",
		}
		got := mergeConfig(base, override)
		assert.Equal(t, override, got)
	})

	t.Run("nil max_retries does not override", func(t *testing.T) {
		override := Config{BaseDelay: "2s"}
		got := mergeConfig(base, override)
		assert.Equal(t, 5, *got.MaxRetries)
		assert.Equal(t, "2s", got.BaseDelay)
	})

	t.Run("explicit zero max_retries overrides", func(t *testing.T) {
		override := Config{MaxRetries: new(0), BaseDelay: "2s"}
		got := mergeConfig(base, override)
		assert.Equal(t, 0, *got.MaxRetries)
		assert.Equal(t, "2s", got.BaseDelay)
	})

	t.Run("nil multiplier does not override", func(t *testing.T) {
		override := Config{Jitter: "none"}
		got := mergeConfig(base, override)
		assert.InDelta(t, 2.0, *got.Multiplier, 0.0001)
		assert.Equal(t, "none", got.Jitter)
	})

	t.Run("explicit zero multiplier overrides", func(t *testing.T) {
		override := Config{Multiplier: new(0.0), Jitter: "none"}
		got := mergeConfig(base, override)
		assert.InDelta(t, 0.0, *got.Multiplier, 0.0001)
		assert.Equal(t, "none", got.Jitter)
	})
}

func TestResolve_PartialConfig(t *testing.T) {
	cfg := Config{
		BaseDelay: "2s",
	}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)

	assert.Equal(t, 0, r.MaxRetries)
	assert.Equal(t, 2*time.Second, r.BaseDelay)
	assert.Equal(t, time.Duration(0), r.MaxDelay)
	assert.InDelta(t, 0.0, r.Multiplier, 0.0001)
	assert.Equal(t, retry.JitterMode(""), r.Jitter)
}

func TestResolve_MillisecondDuration(t *testing.T) {
	cfg := Config{BaseDelay: "500ms"}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)
	assert.Equal(t, 500*time.Millisecond, r.BaseDelay)
}

func TestResolve_MinuteDuration(t *testing.T) {
	cfg := Config{MaxDelay: "2m30s"}

	r, err := cfg.Resolve("openai")
	require.NoError(t, err)
	assert.Equal(t, 150*time.Second, r.MaxDelay)
}

func TestConfig_NoEnvTags(t *testing.T) {
	for field := range reflect.TypeFor[Config]().Fields() {
		assert.Empty(t, field.Tag.Get("env"), "field %s should not have an env tag", field.Name)
	}
}
