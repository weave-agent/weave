package wire

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const outdatedTimeout = 10 * time.Second

type extensionSource int

const (
	sourceGit extensionSource = iota
	sourceLocal
)

type extensionStatus struct {
	Name       string
	Dir        string
	Source     extensionSource
	LocalHead  string
	RemoteHead string
	Outdated   bool
}

// extensionsDir returns the user-level extensions directory (~/.weave/extensions/).
func extensionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	return filepath.Join(home, ".weave", "extensions"), nil
}

// listExtensionsDir scans ~/.weave/extensions/ and returns status for each entry.
func listExtensionsDir() ([]extensionStatus, error) {
	dir, err := extensionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read extensions dir: %w", err)
	}

	var result []extensionStatus

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		extDir := filepath.Join(dir, name)
		st := extensionStatus{
			Name:   name,
			Dir:    extDir,
			Source: sourceLocal,
		}

		if _, err := os.Stat(filepath.Join(extDir, ".git")); err == nil {
			st.Source = sourceGit
		}

		result = append(result, st)
	}

	return result, nil
}

// checkOutdated compares local HEAD to remote HEAD for a git-sourced extension.
func checkOutdated(ext *extensionStatus) error {
	if ext.Source != sourceGit {
		return nil
	}

	localHead, err := gitRevParseHEAD(ext.Dir)
	if err != nil {
		return fmt.Errorf("local head for %s: %w", ext.Name, err)
	}

	ext.LocalHead = localHead

	remoteHead, err := gitLSRemoteHEAD(ext.Dir)
	if err != nil {
		return fmt.Errorf("remote head for %s: %w", ext.Name, err)
	}

	ext.RemoteHead = remoteHead
	ext.Outdated = localHead != remoteHead

	return nil
}

// updateExtension runs git pull --ff-only in the extension directory.
func updateExtension(name string) error {
	dir, err := extensionsDir()
	if err != nil {
		return err
	}

	extDir := filepath.Join(dir, name)

	if _, err := os.Stat(extDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("extension %q not found", name)
		}

		return fmt.Errorf("stat %s: %w", name, err)
	}

	if _, err := os.Stat(filepath.Join(extDir, ".git")); err != nil {
		return fmt.Errorf("extension %q is not git-sourced (installed from local path)", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), outdatedTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "pull", "--ff-only")
	cmd.Dir = extDir
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git pull %s: timed out after %s", name, outdatedTimeout)
		}

		return fmt.Errorf("git pull %s: %w", name, err)
	}

	return nil
}

// uninstallExtension removes an extension directory from ~/.weave/extensions/.
func uninstallExtension(name string) error {
	dir, err := extensionsDir()
	if err != nil {
		return err
	}

	extDir := filepath.Join(dir, name)

	if _, err := os.Stat(extDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("extension %q not found", name)
		}

		return fmt.Errorf("stat %s: %w", name, err)
	}

	if err := os.RemoveAll(extDir); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}

	return nil
}

func gitRevParseHEAD(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), outdatedTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

func gitLSRemoteHEAD(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), outdatedTimeout)
	defer cancel()

	// Suppress stderr to hide "fatal: could not read from remote repository" on network errors.
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "origin", "HEAD")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote origin HEAD: %w", err)
	}

	// Output format: "<hash>\tHEAD\n"
	line, _ := strings.CutSuffix(strings.TrimSpace(string(out)), "\tHEAD")

	if line == "" {
		return "", fmt.Errorf("unexpected ls-remote output: %q", string(out))
	}

	return line, nil
}
