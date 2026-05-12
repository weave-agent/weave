package extmanage

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
