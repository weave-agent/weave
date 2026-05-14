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
	assert.Equal(t, []string{"--weave-debug=false", "--prompt", "hello"}, got)
}

func TestPrependDebugFlag_FlagBeforeValue(t *testing.T) {
	got := prependDebugFlag([]string{"--debug", "hello"})
	assert.Equal(t, []string{"--weave-debug=true", "hello"}, got)
}

func TestPrependDebugFlag_EnvVarFalse(t *testing.T) {
	t.Setenv("WEAVE_DEBUG", "false")

	got := prependDebugFlag([]string{"--prompt", "hello"})
	assert.Equal(t, []string{"--prompt", "hello"}, got)
}

func TestPrependDebugFlag_FlagSpaceSeparatedFalse(t *testing.T) {
	got := prependDebugFlag([]string{"--debug", "false", "--prompt", "hello"})
	assert.Equal(t, []string{"--weave-debug=false", "--prompt", "hello"}, got)
}

func TestPrependDebugFlag_FlagSpaceSeparatedZero(t *testing.T) {
	got := prependDebugFlag([]string{"--debug", "0", "--prompt", "hello"})
	assert.Equal(t, []string{"--weave-debug=false", "--prompt", "hello"}, got)
}

func TestPrependDebugFlag_FlagSpaceSeparatedTrue(t *testing.T) {
	got := prependDebugFlag([]string{"--debug", "true", "--prompt", "hello"})
	assert.Equal(t, []string{"--weave-debug=true", "--prompt", "hello"}, got)
}

func TestPrependDebugFlag_FlagSpaceSeparatedOne(t *testing.T) {
	got := prependDebugFlag([]string{"--debug", "1", "--prompt", "hello"})
	assert.Equal(t, []string{"--weave-debug=true", "--prompt", "hello"}, got)
}

func TestPrependDebugFlag_FlagRemoved(t *testing.T) {
	got := prependDebugFlag([]string{"--prompt", "hello"})
	assert.Equal(t, []string{"--prompt", "hello"}, got)
}
