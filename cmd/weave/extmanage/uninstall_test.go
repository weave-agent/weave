package extmanage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUninstallExtension_Success(t *testing.T) {
	extDir := setupExtensionsDir(t)

	extPath := filepath.Join(extDir, "my-tool")
	require.NoError(t, os.MkdirAll(extPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extPath, "main.go"), []byte("package main\n"), 0o600))

	err := uninstallExtension("my-tool")
	require.NoError(t, err)

	_, statErr := os.Stat(extPath)
	assert.True(t, os.IsNotExist(statErr))
}

func TestUninstallExtension_NotFound(t *testing.T) {
	setupExtensionsDir(t)

	err := uninstallExtension("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUninstallExtension_PathTraversal(t *testing.T) {
	setupExtensionsDir(t)

	err := uninstallExtension("../other-dir")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid extension name")
}

// --- RunUninstall tests ---

func TestRunUninstall_Success(t *testing.T) {
	extDir := setupExtensionsDir(t)

	extPath := filepath.Join(extDir, "my-tool")
	require.NoError(t, os.MkdirAll(extPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extPath, "main.go"), []byte("package main\n"), 0o600))

	code := RunUninstall([]string{"my-tool"})
	assert.Equal(t, 0, code)

	_, statErr := os.Stat(extPath)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRunUninstall_MissingArg(t *testing.T) {
	code := RunUninstall(nil)
	assert.Equal(t, 1, code)
}

func TestRunUninstall_TooManyArgs(t *testing.T) {
	code := RunUninstall([]string{"a", "b"})
	assert.Equal(t, 1, code)
}

func TestRunUninstall_NotFound(t *testing.T) {
	setupExtensionsDir(t)

	code := RunUninstall([]string{"nonexistent"})
	assert.Equal(t, 1, code)
}
