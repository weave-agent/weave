package wire

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitIsAvailable skips the test if git is not installed.
func gitIsAvailable(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// initGitRepo creates a minimal git repo with an initial commit on the master branch.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
}

// initBareRepo creates a bare git repo and seeds it with an initial commit
// (by creating a temp working copy, committing, and pushing).
func initBareRepo(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	bareDir := filepath.Join(t.TempDir(), "remote.git")
	runGit(t, tmp, "init", "--bare", bareDir)

	// Seed the bare repo with an initial commit so it has a HEAD ref.
	workDir := filepath.Join(t.TempDir(), "seed")
	runGit(t, t.TempDir(), "clone", bareDir, workDir)
	runGit(t, workDir, "config", "user.email", "test@test.com")
	runGit(t, workDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "main.go"), []byte("package main\n"), 0o600))
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "initial")
	runGit(t, workDir, "push", "-u", "origin", "HEAD")

	return bareDir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "git %s", strings.Join(args, " "))
}

func setupExtensionsDir(t *testing.T) string {
	t.Helper()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	extDir := filepath.Join(homeDir, ".weave", "extensions")
	require.NoError(t, os.MkdirAll(extDir, 0o750))

	return extDir
}

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

// --- runList tests ---

func TestRunList_NoExtensions(t *testing.T) {
	setupExtensionsDir(t)

	code := runList(nil)
	assert.Equal(t, 0, code)
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

	code := runList(nil)
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

	code := runList(nil)
	assert.Equal(t, 0, code)
}

// --- runUpdate tests ---

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

	code := runUpdate([]string{"my-tool"})
	assert.Equal(t, 0, code)
	assert.FileExists(t, filepath.Join(localDir, "feature.go"))
}

func TestRunUpdate_NotFound(t *testing.T) {
	setupExtensionsDir(t)

	code := runUpdate([]string{"nonexistent"})
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

	code := runUpdate(nil)
	assert.Equal(t, 0, code)
}

func TestRunUpdate_NoGitExtensions(t *testing.T) {
	extDir := setupExtensionsDir(t)

	// Only local extensions — nothing to update.
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "local-tool"), 0o750))

	code := runUpdate(nil)
	assert.Equal(t, 0, code)
}

// --- runUninstall tests ---

func TestRunUninstall_Success(t *testing.T) {
	extDir := setupExtensionsDir(t)

	extPath := filepath.Join(extDir, "my-tool")
	require.NoError(t, os.MkdirAll(extPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extPath, "main.go"), []byte("package main\n"), 0o600))

	code := runUninstall([]string{"my-tool"})
	assert.Equal(t, 0, code)

	_, statErr := os.Stat(extPath)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRunUninstall_MissingArg(t *testing.T) {
	code := runUninstall(nil)
	assert.Equal(t, 1, code)
}

func TestRunUninstall_NotFound(t *testing.T) {
	setupExtensionsDir(t)

	code := runUninstall([]string{"nonexistent"})
	assert.Equal(t, 1, code)
}
