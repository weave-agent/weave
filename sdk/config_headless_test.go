package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopConfigIsHeadless(t *testing.T) {
	cfg := noopConfig{}
	assert.True(t, cfg.IsHeadless(), "noopConfig should report headless")
}

func TestFilePathConfigIsHeadless(t *testing.T) {
	cfg := FilePathConfig("/some/path")
	assert.True(t, cfg.IsHeadless(), "FilePathConfig should report headless")
}

func TestHeadlessConfig_OverridesToHeadless(t *testing.T) {
	inner := noopConfig{}
	cfg := HeadlessConfig{Config: inner, Headless: true}
	assert.True(t, cfg.IsHeadless(), "HeadlessConfig with Headless=true should report headless")
}

func TestHeadlessConfig_OverridesToNotHeadless(t *testing.T) {
	inner := noopConfig{}
	cfg := HeadlessConfig{Config: inner, Headless: false}
	assert.False(t, cfg.IsHeadless(), "HeadlessConfig with Headless=false should report not headless")
}

func TestHeadlessConfig_DelegatesOtherMethods(t *testing.T) {
	inner := FilePathConfig("/test/path")
	cfg := HeadlessConfig{Config: inner, Headless: false}

	assert.Equal(t, "/test/path", cfg.FilePath(), "HeadlessConfig should delegate FilePath to inner")
	assert.Nil(t, cfg.ProviderConfig("any"), "HeadlessConfig should delegate ProviderConfig to inner")
}
