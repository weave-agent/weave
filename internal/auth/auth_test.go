package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	auth, err := Load()
	require.NoError(t, err)
	assert.Empty(t, auth.GetProviderKey("anthropic"))
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	testKey1 := "sk-ant-" + t.Name()
	testKey2 := "sk-" + t.Name()

	auth := &File{
		Providers: map[string]ProviderAuth{
			"anthropic": {APIKey: testKey1},
			"openai":    {APIKey: testKey2},
		},
	}

	require.NoError(t, Save(auth))

	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, testKey1, loaded.GetProviderKey("anthropic"))
	assert.Equal(t, testKey2, loaded.GetProviderKey("openai"))
	assert.Empty(t, loaded.GetProviderKey("unknown"))
}

func TestSave_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// ~/.weave/ doesn't exist yet
	auth := &File{Providers: map[string]ProviderAuth{}}
	require.NoError(t, Save(auth))

	_, err := os.Stat(filepath.Join(dir, ".weave", "auth.json"))
	require.NoError(t, err)
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	require.NoError(t, Save(&File{Providers: map[string]ProviderAuth{}}))

	info, err := os.Stat(filepath.Join(dir, ".weave", "auth.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestSetProviderKey_NewProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	require.NoError(t, SetProviderKey("anthropic", "sk-new"))

	auth, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "sk-new", auth.GetProviderKey("anthropic"))
}

func TestSetProviderKey_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	require.NoError(t, SetProviderKey("anthropic", "sk-old"))
	require.NoError(t, SetProviderKey("anthropic", "sk-updated"))

	auth, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "sk-updated", auth.GetProviderKey("anthropic"))
}
