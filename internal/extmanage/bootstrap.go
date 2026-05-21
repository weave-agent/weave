package extmanage

import (
	"context"
	"fmt"
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
	"sandbox",
	"sandbox-ui",
	"jsonl",
	"tui",
	"tui-diffview",
	"tui-subagent",
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
// the extensions directory does not exist or exists but is empty (no non-hidden entries).
func NeedsBootstrap(homeDir string) (bool, error) {
	dir := ExtensionsDir(homeDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // directory does not exist yet — needs bootstrap
		}

		return false, fmt.Errorf("read extensions dir: %w", err)
	}

	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			return false, nil // has at least one real extension
		}
	}

	return true, nil
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

		// Skip if already installed.
		if _, err := os.Stat(destDir); err == nil {
			continue
		}

		if output != nil {
			output("  -> " + name)
		}

		repo := coreExtensionRepo(name)

		parsed, err := parseSource(repo)
		if err != nil {
			result.Failed = append(result.Failed, name)

			if output != nil {
				output(fmt.Sprintf("  !! %s: parse source: %v", name, err))
			}

			continue
		}

		stagingDir, err := stagingPath(homeDir, name)
		if err != nil {
			result.Failed = append(result.Failed, name)

			if output != nil {
				output(fmt.Sprintf("  !! %s: staging: %v", name, err))
			}

			continue
		}

		if err := cloneExtension(parsed.gitURL, stagingDir); err != nil {
			_ = os.RemoveAll(stagingDir)

			result.Failed = append(result.Failed, name)

			if output != nil {
				output(fmt.Sprintf("  !! %s: clone: %v", name, err))
			}

			continue
		}

		if !hasGoFiles(stagingDir) {
			_ = os.RemoveAll(stagingDir)

			result.Failed = append(result.Failed, name)

			if output != nil {
				output(fmt.Sprintf("  !! %s: no .go files", name))
			}

			continue
		}

		if !hasGoMod(stagingDir) {
			_ = os.RemoveAll(stagingDir)

			result.Failed = append(result.Failed, name)

			if output != nil {
				output(fmt.Sprintf("  !! %s: no go.mod", name))
			}

			continue
		}

		if err := swapStaging(stagingDir, destDir); err != nil {
			_ = os.RemoveAll(stagingDir)

			result.Failed = append(result.Failed, name)

			if output != nil {
				output(fmt.Sprintf("  !! %s: install: %v", name, err))
			}

			continue
		}

		result.Installed = append(result.Installed, name)
	}

	return result, nil
}
