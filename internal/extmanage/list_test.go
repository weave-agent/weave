package extmanage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListExtensionsDir_Empty(t *testing.T) {
	setupExtensionsDir(t)

	exts, err := listExtensionsDir()
	require.NoError(t, err)
	assert.Empty(t, exts)
}

func TestListExtensionsDir_NoDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	exts, err := listExtensionsDir()
	require.NoError(t, err)
	assert.Empty(t, exts)
}

func TestListExtensionsDir_PlainDirs(t *testing.T) {
	extDir := setupExtensionsDir(t)

	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "tool-a"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "tool-b"), 0o750))

	exts, err := listExtensionsDir()
	require.NoError(t, err)
	require.Len(t, exts, 2)

	names := map[string]bool{}
	for _, e := range exts {
		names[e.Name] = true
		assert.Equal(t, sourceLocal, e.Source)
		assert.NotEmpty(t, e.Dir)
	}

	assert.True(t, names["tool-a"])
	assert.True(t, names["tool-b"])
}

func TestListExtensionsDir_GitDirs(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	gitExt := filepath.Join(extDir, "git-tool")
	require.NoError(t, os.MkdirAll(gitExt, 0o750))
	initGitRepo(t, gitExt)

	localExt := filepath.Join(extDir, "local-tool")
	require.NoError(t, os.MkdirAll(localExt, 0o750))

	exts, err := listExtensionsDir()
	require.NoError(t, err)
	require.Len(t, exts, 2)

	extMap := map[string]extensionStatus{}
	for _, e := range exts {
		extMap[e.Name] = e
	}

	assert.Equal(t, sourceGit, extMap["git-tool"].Source)
	assert.Equal(t, sourceLocal, extMap["local-tool"].Source)
}

func TestListExtensionsDir_IgnoresHiddenDirs(t *testing.T) {
	extDir := setupExtensionsDir(t)

	require.NoError(t, os.MkdirAll(filepath.Join(extDir, ".hidden"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "visible"), 0o750))

	exts, err := listExtensionsDir()
	require.NoError(t, err)
	require.Len(t, exts, 1)
	assert.Equal(t, "visible", exts[0].Name)
}

func TestListExtensionsDir_IgnoresFiles(t *testing.T) {
	extDir := setupExtensionsDir(t)

	require.NoError(t, os.WriteFile(filepath.Join(extDir, "not-a-dir.txt"), []byte("hi"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "real-ext"), 0o750))

	exts, err := listExtensionsDir()
	require.NoError(t, err)
	require.Len(t, exts, 1)
	assert.Equal(t, "real-ext", exts[0].Name)
}

func TestCheckOutdated_UpToDate(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	gitExt := filepath.Join(extDir, "my-tool")
	require.NoError(t, os.MkdirAll(gitExt, 0o750))
	initGitRepo(t, gitExt)

	// Set a fake remote so ls-remote works (point to the same repo via file path).
	runGit(t, gitExt, "remote", "add", "origin", gitExt)

	ext := extensionStatus{
		Name:   "my-tool",
		Dir:    gitExt,
		Source: sourceGit,
	}

	err := checkOutdated(&ext)
	require.NoError(t, err)
	assert.False(t, ext.Outdated)
	assert.NotEmpty(t, ext.LocalHead)
	assert.NotEmpty(t, ext.RemoteHead)
	assert.Equal(t, ext.LocalHead, ext.RemoteHead)
}

func TestCheckOutdated_Outdated(t *testing.T) {
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

	ext := extensionStatus{
		Name:   "my-tool",
		Dir:    localDir,
		Source: sourceGit,
	}

	err := checkOutdated(&ext)
	require.NoError(t, err)
	assert.True(t, ext.Outdated)
	assert.NotEqual(t, ext.LocalHead, ext.RemoteHead)
}

func TestCheckOutdated_NonGit(t *testing.T) {
	ext := extensionStatus{
		Name:   "local-tool",
		Dir:    "/tmp/whatever",
		Source: sourceLocal,
	}

	err := checkOutdated(&ext)
	require.NoError(t, err)
	assert.False(t, ext.Outdated)
}

func TestCheckOutdated_NetworkError(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	gitExt := filepath.Join(extDir, "my-tool")
	require.NoError(t, os.MkdirAll(gitExt, 0o750))
	initGitRepo(t, gitExt)

	// Add a fake remote that doesn't exist.
	runGit(t, gitExt, "remote", "add", "origin", "https://nonexistent.example.invalid/repo.git")

	ext := extensionStatus{
		Name:   "my-tool",
		Dir:    gitExt,
		Source: sourceGit,
	}

	err := checkOutdated(&ext)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote head")
}

// --- RunList tests ---

func TestRunList_NoExtensions(t *testing.T) {
	setupExtensionsDir(t)

	code := RunList(nil)
	assert.Equal(t, 0, code)
}

func TestRunList_RejectsExtraArgs(t *testing.T) {
	setupExtensionsDir(t)

	code := RunList([]string{"unexpected"})
	assert.Equal(t, 1, code)
}

func TestRunList_MixedSources(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	// Create a local extension.
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "local-tool"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "local-tool", "main.go"), []byte("package main\n"), 0o600))

	// Create a git-sourced extension (up-to-date: remote points to itself).
	gitExt := filepath.Join(extDir, "git-tool")
	require.NoError(t, os.MkdirAll(gitExt, 0o750))
	initGitRepo(t, gitExt)
	runGit(t, gitExt, "remote", "add", "origin", gitExt)

	code := RunList(nil)
	assert.Equal(t, 0, code)
}

func TestRunList_OutdatedExtension(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	bareDir := initBareRepo(t)

	// Clone from bare repo.
	localDir := filepath.Join(extDir, "outdated-tool")
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

	code := RunList(nil)
	assert.Equal(t, 0, code)
}
