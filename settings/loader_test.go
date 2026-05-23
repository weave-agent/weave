package settings

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	Timeout  int     `json:"timeout" default:"120" env:"TEST_TIMEOUT" flag:"timeout" short:"t" validate:"gt=0,lt=3600" description:"Command timeout"`
	Shell    string  `json:"shell" default:"bash" env:"TEST_SHELL" flag:"shell" description:"Shell to use"`
	Verbose  bool    `json:"verbose" default:"false" env:"TEST_VERBOSE" flag:"verbose" short:"v" description:"Enable verbose output"`
	MaxLines int     `json:"max_lines" default:"1000" validate:"min=1,max=10000"`
	Ratio    float64 `json:"ratio" default:"0.5" validate:"gt=0,lt=1"`
}

type nestedConfig struct {
	Name    string      `json:"name" default:"default-name"`
	Timeout int         `json:"timeout" default:"30"`
	Inner   innerConfig `json:"inner"`
}

type innerConfig struct {
	Key   string `json:"key" default:"inner-default"`
	Count int    `json:"count" default:"5" validate:"gt=0"`
}

type validationConfig struct {
	Required string  `json:"required" validate:"required"`
	URL      string  `json:"url" validate:"url"`
	Mode     string  `json:"mode" default:"standard" validate:"oneof=standard relaxed strict"`
	Count    int     `json:"count" default:"5" validate:"gt=0,lt=100"`
	MinMax   int     `json:"min_max" default:"10" validate:"min=5,max=20"`
	Ratio    float64 `json:"ratio" default:"0.5" validate:"gt=0,lt=1"`
	Label    string  `json:"label"`
	Optional string  `json:"optional"`
}

type customValidator struct {
	Value string `json:"value"`
}

func (c customValidator) Validate() error {
	if c.Value == "invalid" {
		return errors.New("value cannot be 'invalid'")
	}

	return nil
}

func TestLoader_Defaults(t *testing.T) {
	l := Loader{}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 120, cfg.Timeout)
	assert.Equal(t, "bash", cfg.Shell)
	assert.False(t, cfg.Verbose)
	assert.Equal(t, 1000, cfg.MaxLines)
	assert.InDelta(t, 0.5, cfg.Ratio, 0.001)
}

func TestLoader_Data(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"timeout":   60,
			"shell":     "zsh",
			"verbose":   true,
			"max_lines": 500,
			"ratio":     0.75,
		},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 60, cfg.Timeout)
	assert.Equal(t, "zsh", cfg.Shell)
	assert.True(t, cfg.Verbose)
	assert.Equal(t, 500, cfg.MaxLines)
	assert.InDelta(t, 0.75, cfg.Ratio, 0.001)
}

func TestLoader_DataPartial(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"shell": "fish",
		},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 120, cfg.Timeout, "default should be preserved")
	assert.Equal(t, "fish", cfg.Shell, "data should override")
}

func TestLoader_DataNested(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"name":    "test-name",
			"timeout": 45,
			"inner": map[string]any{
				"key":   "test-key",
				"count": 10,
			},
		},
	}

	var cfg nestedConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, "test-name", cfg.Name)
	assert.Equal(t, 45, cfg.Timeout)
	assert.Equal(t, "test-key", cfg.Inner.Key)
	assert.Equal(t, 10, cfg.Inner.Count)
}

func TestLoader_DataRejectsDeprecatedSandboxKeys(t *testing.T) {
	for _, key := range []string{"mode", "writable", "deny_read", "deny_write"} {
		t.Run(key, func(t *testing.T) {
			l := Loader{
				Data: map[string]any{
					"sandbox": map[string]any{
						key: "old-value",
					},
				},
			}

			var cfg Settings

			err := l.Load(&cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported sandbox config key "+strconv.Quote(key))
			assert.Contains(t, err.Error(), "sandbox mode API was removed")
		})
	}
}

func TestLoader_DataNestedDefaults(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"name": "only-name",
		},
	}

	var cfg nestedConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, "only-name", cfg.Name)
	assert.Equal(t, 30, cfg.Timeout)
	assert.Equal(t, "inner-default", cfg.Inner.Key)
	assert.Equal(t, 5, cfg.Inner.Count)
}

func TestLoader_Env(t *testing.T) {
	t.Setenv("WEAVE_TEST_TIMEOUT", "90")
	t.Setenv("WEAVE_TEST_SHELL", "fish")
	t.Setenv("WEAVE_TEST_VERBOSE", "true")

	l := Loader{EnvPrefix: "WEAVE"}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 90, cfg.Timeout)
	assert.Equal(t, "fish", cfg.Shell)
	assert.True(t, cfg.Verbose)
}

func TestLoader_EnvOverridesData(t *testing.T) {
	t.Setenv("WEAVE_TEST_TIMEOUT", "90")

	l := Loader{
		Data:      map[string]any{"timeout": 60},
		EnvPrefix: "WEAVE",
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 90, cfg.Timeout, "env should override data")
}

func TestLoader_EnvEmptyPrefix(t *testing.T) {
	t.Setenv("TEST_TIMEOUT", "90")
	t.Setenv("TEST_SHELL", "fish")

	l := Loader{EnvPrefix: ""}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 90, cfg.Timeout, "env var should be read without prefix")
	assert.Equal(t, "fish", cfg.Shell, "env var should be read without prefix")
}

func TestLoader_Flags(t *testing.T) {
	l := Loader{
		Args: []string{"--timeout", "45", "--shell", "fish", "--verbose"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 45, cfg.Timeout)
	assert.Equal(t, "fish", cfg.Shell)
	assert.True(t, cfg.Verbose)
}

func TestLoader_FlagShort(t *testing.T) {
	l := Loader{
		Args: []string{"-t", "45", "-v"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 45, cfg.Timeout)
	assert.True(t, cfg.Verbose)
}

func TestLoader_FlagsOverrideEnv(t *testing.T) {
	t.Setenv("WEAVE_TEST_TIMEOUT", "90")

	l := Loader{
		Args:      []string{"--timeout", "45"},
		EnvPrefix: "WEAVE",
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 45, cfg.Timeout, "flags should override env")
}

func TestLoader_FlagsIgnoreUnknown(t *testing.T) {
	l := Loader{
		Args: []string{"--timeout", "45", "--unknown", "value"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, 45, cfg.Timeout)
}

func TestLoader_PriorityOrder(t *testing.T) {
	// defaults → data → env → flags
	t.Setenv("WEAVE_TEST_TIMEOUT", "90")

	l := Loader{
		Data:      map[string]any{"timeout": 60, "shell": "zsh"},
		EnvPrefix: "WEAVE",
		Args:      []string{"--timeout", "45"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))

	assert.Equal(t, 45, cfg.Timeout, "flag should win")
	assert.Equal(t, "zsh", cfg.Shell, "data should be used since no env/flag")
	assert.False(t, cfg.Verbose, "default should be used")
}

func TestLoader_ValidationRequired(t *testing.T) {
	l := Loader{}

	var cfg validationConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Field, "Required")
	assert.Contains(t, errs[0].Message, "required")
}

func TestLoader_ValidationURL(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"required": "ok",
			"url":      "not-a-url",
		},
	}

	var cfg validationConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)

	found := false

	for _, e := range errs {
		if e.Field == "URL" {
			found = true

			assert.Contains(t, e.Message, "invalid URL")
		}
	}

	assert.True(t, found, "expected URL validation error")
}

func TestLoader_ValidationURLEmptyOK(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"required": "ok",
		},
	}

	var cfg validationConfig
	require.NoError(t, l.Load(&cfg))
	assert.Empty(t, cfg.URL)
}

func TestLoader_ValidationOneOf(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"required": "ok",
			"mode":     "invalid-mode",
		},
	}

	var cfg validationConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)

	found := false

	for _, e := range errs {
		if e.Field == "Mode" {
			found = true

			assert.Contains(t, e.Message, "one of")
		}
	}

	assert.True(t, found, "expected oneof validation error")
}

func TestLoader_ValidationOneOfEmptyOK(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"required": "ok",
		},
	}

	var cfg validationConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, "standard", cfg.Mode)
}

func TestLoader_ValidationGT(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"required": "ok",
			"count":    0,
		},
	}

	var cfg validationConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)

	found := false

	for _, e := range errs {
		if e.Field == "Count" {
			found = true

			assert.Contains(t, e.Message, "greater than")
		}
	}

	assert.True(t, found, "expected gt validation error")
}

func TestLoader_ValidationLT(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"required": "ok",
			"count":    100,
		},
	}

	var cfg validationConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)

	found := false

	for _, e := range errs {
		if e.Field == "Count" {
			found = true

			assert.Contains(t, e.Message, "less than")
		}
	}

	assert.True(t, found, "expected lt validation error")
}

func TestLoader_ValidationMinMax(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"required": "ok",
			"min_max":  3,
		},
	}

	var cfg validationConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)

	found := false

	for _, e := range errs {
		if e.Field == "MinMax" {
			found = true

			assert.Contains(t, e.Message, "at least")
		}
	}

	assert.True(t, found, "expected min validation error")
}

func TestLoader_ValidationFloatRange(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"required": "ok",
			"ratio":    1.5,
		},
	}

	var cfg validationConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)

	found := false

	for _, e := range errs {
		if e.Field == "Ratio" {
			found = true

			assert.Contains(t, e.Message, "less than")
		}
	}

	assert.True(t, found, "expected float lt validation error")
}

func TestLoader_ValidationStringMinMax(t *testing.T) {
	type stringMinMaxConfig struct {
		Label string `json:"label" validate:"min=2,max=10"`
	}

	l := Loader{
		Data: map[string]any{
			"label": "a",
		},
	}

	var cfg stringMinMaxConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "at least")
}

func TestLoader_ValidationMultipleErrors(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"url":   "bad-url",
			"mode":  "bad-mode",
			"count": 0,
		},
	}

	var cfg validationConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.GreaterOrEqual(t, len(errs), 3, "expected at least 3 validation errors")
}

func TestLoader_ValidationNested(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"inner": map[string]any{
				"count": 0,
			},
		},
	}

	var cfg nestedConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Field, "Inner.Count")
	assert.Contains(t, errs[0].Message, "greater than")
}

func TestLoader_CustomValidator(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"value": "invalid",
		},
	}

	var cfg customValidator

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "cannot be 'invalid'")
}

func TestLoader_CustomValidatorPass(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"value": "valid",
		},
	}

	var cfg customValidator
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, "valid", cfg.Value)
}

func TestLoader_NilTarget(t *testing.T) {
	l := Loader{}
	err := l.Load(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestLoader_NonPointer(t *testing.T) {
	l := Loader{}

	var cfg testConfig

	err := l.Load(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pointer")
}

func TestLoader_NonStruct(t *testing.T) {
	l := Loader{}

	var s string

	err := l.Load(&s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "struct")
}

func TestLoader_SliceField(t *testing.T) {
	type sliceConfig struct {
		Paths []string `json:"paths"`
	}

	l := Loader{
		Data: map[string]any{
			"paths": []any{"/tmp", "/var"},
		},
	}

	var cfg sliceConfig
	require.NoError(t, l.Load(&cfg))
	require.Len(t, cfg.Paths, 2)
	assert.Equal(t, "/tmp", cfg.Paths[0])
	assert.Equal(t, "/var", cfg.Paths[1])
}

func TestLoader_SliceFieldDefaults(t *testing.T) {
	type sliceConfig struct {
		Paths []string `json:"paths" default:"a,b"`
	}

	l := Loader{}

	var cfg sliceConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, []string{"a", "b"}, cfg.Paths)
}

func TestLoader_SliceValidationMinMax(t *testing.T) {
	type sliceValConfig struct {
		Items []string `json:"items" validate:"min=1,max=3"`
	}

	l := Loader{
		Data: map[string]any{
			"items": []any{},
		},
	}

	var cfg sliceValConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "at least")
}

func TestLoader_SliceValidationMax(t *testing.T) {
	type sliceValConfig struct {
		Items []string `json:"items" validate:"max=2"`
	}

	l := Loader{
		Data: map[string]any{
			"items": []any{"a", "b", "c"},
		},
	}

	var cfg sliceValConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "at most")
}

func TestLoader_SliceEnv(t *testing.T) {
	type sliceConfig struct {
		Paths []string `json:"paths" env:"PATHS"`
	}

	t.Setenv("WEAVE_PATHS", "/tmp,/var")

	l := Loader{EnvPrefix: "WEAVE"}

	var cfg sliceConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, []string{"/tmp", "/var"}, cfg.Paths)
}

func TestLoader_SliceEnvSingleValue(t *testing.T) {
	type sliceConfig struct {
		Paths []string `json:"paths" env:"PATHS"`
	}

	t.Setenv("WEAVE_PATHS", "/tmp")

	l := Loader{EnvPrefix: "WEAVE"}

	var cfg sliceConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, []string{"/tmp"}, cfg.Paths)
}

func TestLoader_SliceFlags(t *testing.T) {
	type sliceConfig struct {
		Paths []string `json:"paths" flag:"paths"`
	}

	l := Loader{
		Args: []string{"--paths", "/tmp,/var"},
	}

	var cfg sliceConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, []string{"/tmp", "/var"}, cfg.Paths)
}

func TestLoader_SliceFlagsRepeated(t *testing.T) {
	type sliceConfig struct {
		Paths []string `json:"paths" flag:"paths"`
	}

	l := Loader{
		Args: []string{"--paths", "/tmp", "--paths", "/var"},
	}

	var cfg sliceConfig
	require.NoError(t, l.Load(&cfg))
	// Repeated flags replace; last value wins (same as scalar flags).
	assert.Equal(t, []string{"/var"}, cfg.Paths)
}

func TestLoader_SliceFlagsOverrideEnv(t *testing.T) {
	type sliceConfig struct {
		Paths []string `json:"paths" env:"PATHS" flag:"paths"`
	}

	t.Setenv("WEAVE_PATHS", "/env1,/env2")

	l := Loader{
		EnvPrefix: "WEAVE",
		Args:      []string{"--paths", "/flag1,/flag2"},
	}

	var cfg sliceConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, []string{"/flag1", "/flag2"}, cfg.Paths)
}

func TestLoader_ValidValidConfig(t *testing.T) {
	l := Loader{
		Data: map[string]any{
			"required": "ok",
			"url":      "https://example.com",
			"mode":     "standard",
			"count":    50,
			"min_max":  10,
			"ratio":    0.5,
			"label":    "hello",
		},
	}

	var cfg validationConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, "ok", cfg.Required)
	assert.Equal(t, "https://example.com", cfg.URL)
	assert.Equal(t, "standard", cfg.Mode)
	assert.Equal(t, 50, cfg.Count)
	assert.Equal(t, 10, cfg.MinMax)
	assert.InDelta(t, 0.5, cfg.Ratio, 0.001)
	assert.Equal(t, "hello", cfg.Label)
}

func TestLoader_UnknownValidationRule(t *testing.T) {
	type badRuleConfig struct {
		Field string `json:"field" validate:"unknown_rule"`
	}

	l := Loader{
		Data: map[string]any{
			"field": "value",
		},
	}

	var cfg badRuleConfig

	err := l.Load(&cfg)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "unknown validation rule")
}

func TestLoader_FlagBool(t *testing.T) {
	l := Loader{
		Args: []string{"--verbose=true"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.True(t, cfg.Verbose)
}

func TestLoader_FlagBoolNoValue(t *testing.T) {
	l := Loader{
		Args: []string{"--verbose"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.True(t, cfg.Verbose)
}

func TestLoader_FlagBoolSplitFalse(t *testing.T) {
	l := Loader{
		Args: []string{"--verbose", "false"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.False(t, cfg.Verbose, "split bool false should be parsed correctly")
}

func TestLoader_FlagBoolSplitTrue(t *testing.T) {
	l := Loader{
		Args: []string{"--verbose", "true"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.True(t, cfg.Verbose, "split bool true should be parsed correctly")
}

func TestLoader_FlagBoolShortSplit(t *testing.T) {
	l := Loader{
		Args: []string{"-v", "false"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.False(t, cfg.Verbose, "short split bool false should be parsed correctly")
}

func TestLoader_FlagBoolSplitZero(t *testing.T) {
	l := Loader{
		Args: []string{"--verbose", "0"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.False(t, cfg.Verbose, "split bool 0 should be parsed as false")
}

func TestLoader_FlagBoolSplitOne(t *testing.T) {
	l := Loader{
		Args: []string{"--verbose", "1"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.True(t, cfg.Verbose, "split bool 1 should be parsed as true")
}

func TestLoader_FlagBoolSplitUppercase(t *testing.T) {
	l := Loader{
		Args: []string{"--verbose", "FALSE"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.False(t, cfg.Verbose, "split bool FALSE should be parsed as false")
}

func TestLoader_FlagBoolSplitMixedCase(t *testing.T) {
	l := Loader{
		Args: []string{"--verbose", "True"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.True(t, cfg.Verbose, "split bool True should be parsed as true")
}

func TestLoader_SliceEnvFiltersEmpty(t *testing.T) {
	type sliceConfig struct {
		Paths []string `json:"paths" env:"PATHS"`
	}

	t.Setenv("WEAVE_PATHS", "/tmp,,/var")

	l := Loader{EnvPrefix: "WEAVE"}

	var cfg sliceConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, []string{"/tmp", "/var"}, cfg.Paths)
}

func TestLoader_SliceFlagsFiltersEmpty(t *testing.T) {
	type sliceConfig struct {
		Paths []string `json:"paths" flag:"paths"`
	}

	l := Loader{
		Args: []string{"--paths", "/tmp,,/var"},
	}

	var cfg sliceConfig
	require.NoError(t, l.Load(&cfg))
	assert.Equal(t, []string{"/tmp", "/var"}, cfg.Paths)
}

func TestLoader_FlagBoolSplitFalseWithRemaining(t *testing.T) {
	l := Loader{
		Args: []string{"--verbose", "false", "positional"},
	}

	var cfg testConfig
	require.NoError(t, l.Load(&cfg))
	assert.False(t, cfg.Verbose)
}
