package extmanage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CoreExtensionNames lists the canonical set of core extensions that are
// auto-installed on first run when ~/.weave/extensions/ is empty.
var CoreExtensionNames = []string{
	"bash",
	"read",
	"edit",
	"write",
	"grep",
	"find",
	"ls",
	"search",
	"webfetch",
	"subagent",
	"anthropic",
	"openai",
	"zai",
	"kimi",
	"codex",
	"agent",
	"guardian",
	"sandbox",
	"jsonl",
	"tui",
	"tui-guardian",
	"tui-diffview",
	"tui-subagent",
}

var obsoleteCoreExtensionNames = []string{
	"tui-sandbox",
}

// coreExtensionRepo returns the GitHub shorthand for a core extension by name.
// The convention is github.com/weave-agent/weave-<name>.
func coreExtensionRepo(name string) string {
	return "github.com/weave-agent/weave-" + name
}

// ExtensionsDir returns the global extensions directory (~/.weave/extensions/).
// Returns the path even if it does not exist.
func ExtensionsDir(homeDir string) string {
	return filepath.Join(homeDir, ".weave", "extensions")
}

// NeedsBootstrap reports whether bootstrap should run. It returns true when
// the extensions directory does not exist, has no non-hidden entries, or is
// missing any required core extension.
func NeedsBootstrap(homeDir string) (bool, error) {
	dir := ExtensionsDir(homeDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // directory does not exist yet — needs bootstrap
		}

		return false, fmt.Errorf("read extensions dir: %w", err)
	}

	installed := make(map[string]struct{}, len(entries))

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}

		installed[e.Name()] = struct{}{}
	}

	if len(installed) == 0 {
		return true, nil
	}

	for _, name := range CoreExtensionNames {
		if _, ok := installed[name]; !ok {
			return true, nil
		}
	}

	if isStaleSandbox(filepath.Join(dir, "sandbox")) {
		return true, nil
	}

	return false, nil
}

// BootstrapResult holds the outcome of a bootstrap run.
type BootstrapResult struct {
	Installed []string // names of successfully installed extensions
	Failed    []string // names of extensions that failed to install
}

// BootstrapCoreExtensions installs all core extensions from GitHub into
// ~/.weave/extensions/. It clones each extension repo using the same logic as
// "weave install". Extensions that already exist on disk are skipped.
//
// The output function receives progress messages suitable for display to the
// user. If output is nil, no progress is reported.
func BootstrapCoreExtensions(ctx context.Context, homeDir string, output func(string)) (*BootstrapResult, error) {
	result := &BootstrapResult{}

	extDir := ExtensionsDir(homeDir)

	if err := os.MkdirAll(extDir, 0o750); err != nil {
		return nil, fmt.Errorf("create extensions dir: %w", err)
	}

	for _, name := range obsoleteCoreExtensionNames {
		if err := os.RemoveAll(filepath.Join(extDir, name)); err != nil {
			return nil, fmt.Errorf("remove obsolete core extension %s: %w", name, err)
		}
	}

	if output != nil {
		output("weave: installing core extensions...")
	}

	for _, name := range CoreExtensionNames {
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("bootstrap canceled: %w", ctx.Err())
		default:
		}

		destDir := filepath.Join(extDir, name)
		if !shouldInstallCoreExtension(name, destDir) {
			continue
		}

		if output != nil {
			output("  -> " + name)
		}

		if err := installCoreExtension(homeDir, destDir, name); err != nil {
			result.Failed = append(result.Failed, name)

			if output != nil {
				output(fmt.Sprintf("  !! %s: %v", name, err))
			}

			continue
		}

		result.Installed = append(result.Installed, name)
	}

	if len(result.Failed) > 0 {
		return result, fmt.Errorf("bootstrap failed for core extensions: %s", strings.Join(result.Failed, ", "))
	}

	return result, nil
}

func shouldInstallCoreExtension(name, destDir string) bool {
	if _, err := os.Stat(destDir); err != nil {
		return true
	}

	return name == "sandbox" && isStaleSandbox(destDir)
}

func installCoreExtension(homeDir, destDir, name string) error {
	repo := coreExtensionRepo(name)

	parsed, err := parseSource(repo)
	if err != nil {
		return fmt.Errorf("parse source: %w", err)
	}

	stagingDir, err := stagingPath(homeDir, name)
	if err != nil {
		return fmt.Errorf("staging: %w", err)
	}

	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	if err := cloneExtension(parsed.gitURL, stagingDir); err != nil {
		return fmt.Errorf("clone: %w", err)
	}

	if !hasGoFiles(stagingDir) {
		return errors.New("no .go files")
	}

	if !hasGoMod(stagingDir) {
		return errors.New("no go.mod")
	}

	if err := swapStaging(stagingDir, destDir); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	return nil
}

func isStaleSandbox(dir string) bool {
	if _, err := os.Stat(dir); err != nil {
		return false
	}

	stale := false
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if stale {
			return fs.SkipAll
		}

		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		data, readErr := os.ReadFile(path) //nolint:gosec // G304 - scanning installed extension files.
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}

		content := string(data)
		stale = strings.Contains(content, "SandboxMode") ||
			strings.Contains(content, "AllowRead(") ||
			strings.Contains(content, "AllowWrite(") ||
			strings.Contains(content, "SetMode(")

		return nil
	})

	return walkErr == nil && stale
}
