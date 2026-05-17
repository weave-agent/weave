package extmanage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/weave-agent/weave/settings"
)

// updateExtension runs git pull --ff-only in the extension directory.
func updateExtension(name string) error {
	if !settings.ValidExtName(name) {
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
		return fmt.Errorf("extension %q is a symlink — update manually", name)
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

// RunUpdate updates git-sourced extensions. With a name argument, only that
// extension is updated. Without arguments, all git-sourced extensions are updated.
func RunUpdate(args []string) int {
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
