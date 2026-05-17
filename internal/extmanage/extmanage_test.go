package extmanage

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

func TestReadModulePath_ValidGoMod(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module github.com/weave-agent/weave-bash\n\ngo 1.22\n"), 0o600))

	assert.Equal(t, "github.com/weave-agent/weave-bash", readModulePath(dir))
}

func TestReadModulePath_NoGoMod(t *testing.T) {
	dir := t.TempDir()

	assert.Empty(t, readModulePath(dir))
}

func TestReadModulePath_EmptyGoMod(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(""), 0o600))

	assert.Empty(t, readModulePath(dir))
}

func TestReadModulePath_ModuleWithWhitespace(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("  module   github.com/weave-agent/weave-bash  \n\ngo 1.22\n"), 0o600))

	assert.Equal(t, "github.com/weave-agent/weave-bash", readModulePath(dir))
}

func TestResolveExtName_FullPath(t *testing.T) {
	assert.Equal(t, "bash", resolveExtName("github.com/weave-agent/weave-bash"))
	assert.Equal(t, "tui-diffview", resolveExtName("github.com/weave-agent/weave-tui-diffview"))
	assert.Equal(t, "sandbox-ui", resolveExtName("github.com/weave-agent/weave-sandbox-ui"))
}

func TestResolveExtName_PlainName(t *testing.T) {
	assert.Equal(t, "bash", resolveExtName("bash"))
	assert.Equal(t, "my-custom-ext", resolveExtName("my-custom-ext"))
}

func TestResolveExtName_EmptySuffix(t *testing.T) {
	// "github.com/weave-agent/weave-" with nothing after — return unchanged.
	assert.Equal(t, "github.com/weave-agent/weave-", resolveExtName("github.com/weave-agent/weave-"))
}

func TestResolveExtName_OtherOrg(t *testing.T) {
	// Different org path — return unchanged.
	assert.Equal(t, "github.com/other/repo", resolveExtName("github.com/other/repo"))
}
