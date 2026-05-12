package extmanage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateExtension_Success(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	bareDir := initBareRepo(t)

	// Clone from the bare repo.
	localDir := filepath.Join(extDir, "my-tool")
	runGit(t, t.TempDir(), "clone", bareDir, localDir)

	// Advance the remote.
	tmpClone := filepath.Join(t.TempDir(), "advance-clone")
	runGit(t, t.TempDir(), "clone", bareDir, tmpClone)
	runGit(t, tmpClone, "config", "user.email", "test@test.com")
	runGit(t, tmpClone, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(tmpClone, "feature.go"), []byte("package main\n"), 0o600))
	runGit(t, tmpClone, "add", ".")
	runGit(t, tmpClone, "commit", "-m", "advance remote")
	runGit(t, tmpClone, "push", "origin", "HEAD")

	// Update should pull the new commit.
	err := updateExtension("my-tool")
	require.NoError(t, err)

	// Verify feature.go exists after update.
	assert.FileExists(t, filepath.Join(localDir, "feature.go"))
}

func TestUpdateExtension_NonGit(t *testing.T) {
	extDir := setupExtensionsDir(t)

	localDir := filepath.Join(extDir, "local-tool")
	require.NoError(t, os.MkdirAll(localDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(localDir, "main.go"), []byte("package main\n"), 0o600))

	err := updateExtension("local-tool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not git-sourced")
}

func TestUpdateExtension_NotFound(t *testing.T) {
	setupExtensionsDir(t)

	err := updateExtension("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateExtension_PathTraversal(t *testing.T) {
	setupExtensionsDir(t)

	err := updateExtension("../../etc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid extension name")
}

// --- RunUpdate tests ---

func TestRunUpdate_Single(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	bareDir := initBareRepo(t)

	localDir := filepath.Join(extDir, "my-tool")
	runGit(t, t.TempDir(), "clone", bareDir, localDir)

	// Advance the remote.
	tmpClone := filepath.Join(t.TempDir(), "advance-clone")
	runGit(t, t.TempDir(), "clone", bareDir, tmpClone)
	runGit(t, tmpClone, "config", "user.email", "test@test.com")
	runGit(t, tmpClone, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(tmpClone, "feature.go"), []byte("package main\n"), 0o600))
	runGit(t, tmpClone, "add", ".")
	runGit(t, tmpClone, "commit", "-m", "advance remote")
	runGit(t, tmpClone, "push", "origin", "HEAD")

	code := RunUpdate([]string{"my-tool"})
	assert.Equal(t, 0, code)
	assert.FileExists(t, filepath.Join(localDir, "feature.go"))
}

func TestRunUpdate_NotFound(t *testing.T) {
	setupExtensionsDir(t)

	code := RunUpdate([]string{"nonexistent"})
	assert.Equal(t, 1, code)
}

func TestRunUpdate_TooManyArgs(t *testing.T) {
	code := RunUpdate([]string{"a", "b"})
	assert.Equal(t, 1, code)
}

func TestRunUpdate_All(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	bareDir := initBareRepo(t)

	// Create two git-sourced extensions.
	for _, name := range []string{"tool-a", "tool-b"} {
		localDir := filepath.Join(extDir, name)
		runGit(t, t.TempDir(), "clone", bareDir, localDir)
	}

	// Create a local extension (should be skipped).
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "local-tool"), 0o750))

	code := RunUpdate(nil)
	assert.Equal(t, 0, code)
}

func TestRunUpdate_NoGitExtensions(t *testing.T) {
	extDir := setupExtensionsDir(t)

	// Only local extensions — nothing to update.
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "local-tool"), 0o750))

	code := RunUpdate(nil)
	assert.Equal(t, 0, code)
}
