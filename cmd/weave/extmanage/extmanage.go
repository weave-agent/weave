package extmanage

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
	CheckErr   string
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
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
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

	// stderr is captured into the error by cmd.Output, so "fatal: could not read
	// from remote repository" won't leak to the terminal on network errors.
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "origin", "HEAD")
	cmd.Dir = dir
	cmd.Stderr = nil

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
