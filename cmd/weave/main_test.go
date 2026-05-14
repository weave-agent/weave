package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrependDebugFlag_EnvVar(t *testing.T) {
	t.Setenv("WEAVE_DEBUG", "1")
	got := prependDebugFlag([]string{"--prompt", "hello"})
	assert.Equal(t, []string{"--weave-debug=true", "--prompt", "hello"}, got)
}

func TestPrependDebugFlag_EnvVarTrue(t *testing.T) {
	t.Setenv("WEAVE_DEBUG", "true")
	got := prependDebugFlag([]string{"--prompt", "hello"})
	assert.Equal(t, []string{"--weave-debug=true", "--prompt", "hello"}, got)
}

func TestPrependDebugFlag_EnvVarUnset(t *testing.T) {
	t.Setenv("WEAVE_DEBUG", "")
	got := prependDebugFlag([]string{"--prompt", "hello"})
	assert.Equal(t, []string{"--prompt", "hello"}, got)
}

func TestPrependDebugFlag_Flag(t *testing.T) {
	got := prependDebugFlag([]string{"--debug", "--prompt", "hello"})
	assert.Equal(t, []string{"--weave-debug=true", "--prompt", "hello"}, got)
}

func TestPrependDebugFlag_FlagEquals(t *testing.T) {
	got := prependDebugFlag([]string{"--debug=true", "--prompt", "hello"})
	assert.Equal(t, []string{"--weave-debug=true", "--prompt", "hello"}, got)
}

func TestPrependDebugFlag_FlagEqualsFalse(t *testing.T) {
	got := prependDebugFlag([]string{"--debug=false", "--prompt", "hello"})
	assert.Equal(t, []string{"--prompt", "hello"}, got)
}

func TestPrependDebugFlag_FlagRemoved(t *testing.T) {
	got := prependDebugFlag([]string{"--prompt", "hello"})
	assert.Equal(t, []string{"--prompt", "hello"}, got)
}
