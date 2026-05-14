package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopConfigIsHeadless(t *testing.T) {
	cfg := NoopConfig{}
	assert.True(t, cfg.IsHeadless(), "NoopConfig should report headless")
}

func TestFilePathConfigIsHeadless(t *testing.T) {
	cfg := FilePathConfig("/some/path")
	assert.True(t, cfg.IsHeadless(), "FilePathConfig should report headless")
}

func TestHeadlessConfig_OverridesToHeadless(t *testing.T) {
	inner := NoopConfig{}
	cfg := HeadlessConfig{Config: inner, Headless: true}
	assert.True(t, cfg.IsHeadless(), "HeadlessConfig with Headless=true should report headless")
}

func TestHeadlessConfig_OverridesToNotHeadless(t *testing.T) {
	inner := NoopConfig{}
	cfg := HeadlessConfig{Config: inner, Headless: false}
	assert.False(t, cfg.IsHeadless(), "HeadlessConfig with Headless=false should report not headless")
}

func TestHeadlessConfig_DelegatesOtherMethods(t *testing.T) {
	inner := FilePathConfig("/test/path")
	cfg := HeadlessConfig{Config: inner, Headless: false}

	assert.Equal(t, "/test/path", cfg.FilePath(), "HeadlessConfig should delegate FilePath to inner")
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", nil, ""), "HeadlessConfig should delegate ExtensionConfig to inner")
}

func TestNoopConfig_ExtensionConfig(t *testing.T) {
	cfg := NoopConfig{}

	var target struct{ Timeout int }
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", &target, "WEAVE_BASH"))
	assert.Zero(t, target.Timeout)
}

func TestFilePathConfig_ExtensionConfig(t *testing.T) {
	cfg := FilePathConfig("/test/path")

	var target struct{ Timeout int }
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", &target, "WEAVE_BASH"))
	assert.Zero(t, target.Timeout)
}

func TestConfigMock_ExtensionConfig(t *testing.T) {
	var called bool

	mock := &ConfigMock{
		ExtensionConfigFunc: func(scope, name string, target any, envPrefix string) error {
			called = true
			return nil
		},
	}

	var target struct{ Timeout int }
	require.NoError(t, mock.ExtensionConfig("tools", "bash", &target, "WEAVE_BASH"))
	assert.True(t, called)
}

func TestEnvPrefixFor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "bash", "WEAVE_BASH"},
		{"with hyphens", "my-extension", "WEAVE_MY_EXTENSION"},
		{"all lowercase", "read", "WEAVE_READ"},
		{"mixed case", "My-Tool", "WEAVE_MY_TOOL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, envPrefixFor(tt.input))
		})
	}
}
