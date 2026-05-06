package wire

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"weave/config"
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
	if !config.ValidExtName(name) {
		return fmt.Errorf("invalid extension name %q", name)
	}

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

	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git pull %s: timed out after %s", name, outdatedTimeout)
		}

		return fmt.Errorf("git pull %s: %w\n%s", name, err, out)
	}

	return nil
}

// uninstallExtension removes an extension directory from ~/.weave/extensions/.
func uninstallExtension(name string) error {
	if !config.ValidExtName(name) {
		return fmt.Errorf("invalid extension name %q", name)
	}

	dir, err := extensionsDir()
	if err != nil {
		return err
	}

	extDir := filepath.Join(dir, name)

	info, err := os.Lstat(extDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("extension %q not found", name)
		}

		return fmt.Errorf("lstat %s: %w", name, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("extension %q is a symlink — remove manually", name)
	}

	if err := os.RemoveAll(extDir); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}

	return nil
}

// runList prints a formatted table of installed extensions to stdout.
// It checks git-sourced extensions for available updates.
func runList(_ []string) int {
	exts, err := listExtensionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave list: %v\n", err)
		return 1
	}

	if len(exts) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "no extensions installed")
		return 0
	}

	// Check outdated for git-sourced extensions.
	for i := range exts {
		if exts[i].Source == sourceGit {
			if err := checkOutdated(&exts[i]); err != nil {
				fmt.Fprintf(os.Stderr, "weave list: warning: %v\n", err)
			}
		}
	}

	_, _ = fmt.Fprintf(os.Stdout, "%-20s %-10s %s\n", "NAME", "SOURCE", "STATUS")

	for _, ext := range exts {
		sourceLabel := "local"
		status := "ok"

		if ext.Source == sourceGit {
			sourceLabel = "git"

			if ext.Outdated {
				status = "outdated"
			}
		} else {
			status = "static"
		}

		_, _ = fmt.Fprintf(os.Stdout, "%-20s %-10s %s\n", ext.Name, sourceLabel, status)
	}

	return 0
}

// runUpdate updates git-sourced extensions. With a name argument, only that
// extension is updated. Without arguments, all git-sourced extensions are updated.
func runUpdate(args []string) int {
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "usage: weave update [name]")

		return 1
	}

	if len(args) > 0 {
		name := args[0]
		if err := updateExtension(name); err != nil {
			fmt.Fprintf(os.Stderr, "weave update: %v\n", err)
			return 1
		}

		_, _ = fmt.Fprintf(os.Stdout, "updated %q\n", name)

		return 0
	}

	// Update all git-sourced extensions.
	exts, err := listExtensionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave update: %v\n", err)
		return 1
	}

	if len(exts) == 0 {
		fmt.Fprintln(os.Stderr, "no extensions installed")
		return 0
	}

	updated := 0
	failed := 0

	for _, ext := range exts {
		if ext.Source != sourceGit {
			continue
		}

		if err := updateExtension(ext.Name); err != nil {
			fmt.Fprintf(os.Stderr, "weave update: %v\n", err)

			failed++
		} else {
			_, _ = fmt.Fprintf(os.Stdout, "updated %q\n", ext.Name)

			updated++
		}
	}

	if updated == 0 && failed == 0 {
		fmt.Fprintln(os.Stderr, "no git-sourced extensions to update")
	}

	if failed > 0 {
		return 1
	}

	return 0
}

// runUninstall removes an extension. It requires exactly one argument (the extension name).
func runUninstall(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "weave uninstall: missing extension name")
		fmt.Fprintln(os.Stderr, "usage: weave uninstall <name>")

		return 1
	}

	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "weave uninstall: too many arguments")
		fmt.Fprintln(os.Stderr, "usage: weave uninstall <name>")

		return 1
	}

	name := args[0]

	if err := uninstallExtension(name); err != nil {
		fmt.Fprintf(os.Stderr, "weave uninstall: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(os.Stdout, "uninstalled %q\n", name)

	return 0
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
