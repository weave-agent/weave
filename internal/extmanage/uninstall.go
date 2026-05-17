package extmanage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/weave-agent/weave/settings"
)

// uninstallExtension removes an extension directory from ~/.weave/extensions/.
func uninstallExtension(name string) error {
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
		return fmt.Errorf("extension %q is a symlink — remove manually", name)
	}

	if err := os.RemoveAll(extDir); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}

	return nil
}

// RunUninstall removes an extension. It requires exactly one argument (the extension name).
func RunUninstall(args []string) int {
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
	name = resolveExtName(name)

	if err := uninstallExtension(name); err != nil {
		fmt.Fprintf(os.Stderr, "weave uninstall: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(os.Stdout, "uninstalled %q\n", name)

	return 0
}
