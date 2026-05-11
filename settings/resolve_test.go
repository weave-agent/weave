package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave/internal/auth"
)

func TestResolveValue_Literal(t *testing.T) {
	got, err := ResolveValue("sk-ant-test")
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-test", got)
}

func TestResolveValue_Empty(t *testing.T) {
	got, err := ResolveValue("")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestResolveValue_Command(t *testing.T) {
	got, err := ResolveValue("!echo hello-world")
	require.NoError(t, err)
	assert.Equal(t, "hello-world", got)
}

func TestResolveValue_CommandTrimmed(t *testing.T) {
	got, err := ResolveValue("!printf '  padded  '")
	require.NoError(t, err)
	assert.Equal(t, "padded", got)
}

func TestResolveValue_CommandCached(t *testing.T) {
	// Same command should return same result from cache
	got1, err := ResolveValue("!echo cached-value")
	require.NoError(t, err)

	got2, err := ResolveValue("!echo cached-value")
	require.NoError(t, err)

	assert.Equal(t, got1, got2)
	assert.Equal(t, "cached-value", got2)
}

func TestResolveValue_CommandFailure(t *testing.T) {
	_, err := ResolveValue("!false")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve command")
}

func TestResolveProviderKey_EnvVar(t *testing.T) {
	t.Setenv("TEST_API_KEY", "from-env")

	got, err := ResolveProviderKey("test", "TEST_API_KEY", nil)
	require.NoError(t, err)
	assert.Equal(t, "from-env", got)
}

func TestResolveProviderKey_AuthFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.NoError(t, auth.SetProviderKey("test", "from-auth"))

	got, err := ResolveProviderKey("test", "UNSET_TEST_VAR", nil)
	require.NoError(t, err)
	assert.Equal(t, "from-auth", got)
}

func TestResolveProviderKey_ConfigEntry(t *testing.T) {
	entry := &ProviderEntry{APIKey: "from-config"}

	got, err := ResolveProviderKey("test", "UNSET_TEST_VAR", entry)
	require.NoError(t, err)
	assert.Equal(t, "from-config", got)
}

func TestResolveProviderKey_ConfigEntryCommand(t *testing.T) {
	entry := &ProviderEntry{APIKey: "!echo from-command"}

	got, err := ResolveProviderKey("test", "UNSET_TEST_VAR", entry)
	require.NoError(t, err)
	assert.Equal(t, "from-command", got)
}

func TestResolveProviderKey_Priority(t *testing.T) {
	t.Setenv("TEST_PRIORITY_KEY", "from-env")

	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, auth.SetProviderKey("test", "from-auth"))

	entry := &ProviderEntry{APIKey: "from-config"}

	// Env var should win over auth and config
	got, err := ResolveProviderKey("test", "TEST_PRIORITY_KEY", entry)
	require.NoError(t, err)
	assert.Equal(t, "from-env", got)
}

func TestResolveProviderKey_AuthOverConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, auth.SetProviderKey("test", "from-auth"))

	entry := &ProviderEntry{APIKey: "from-config"}

	// Auth should win over config when no env var
	got, err := ResolveProviderKey("test", "UNSET_TEST_VAR", entry)
	require.NoError(t, err)
	assert.Equal(t, "from-auth", got)
}

func TestResolveProviderKey_NotFound(t *testing.T) {
	got, err := ResolveProviderKey("test", "UNSET_TEST_VAR", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
