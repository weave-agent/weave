package launcher

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var validExtName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type ExtensionInfo struct {
	Name       string
	Dir        string
	GoFiles    []string
	ModulePath string // e.g. "weave/ext/tools/bash"; populated by builder
}

// Discover resolves each named extension to its source directory and Go files.
// For each name, it checks:
//  1. Project-local: .weave/extensions/{name}/
//  2. Global: ~/.weave/extensions/{name}/
func Discover(projectDir string, names []string) ([]ExtensionInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("discover: get home dir: %w", err)
	}

	return DiscoverCustomHome(projectDir, homeDir, names)
}

// DiscoverWithBuiltins is like Discover but also checks built-in extensions
// under moduleRoot/extensions/{name}/ as a final fallback. This allows core
// extensions shipped with weave to be found without installing them.
func DiscoverWithBuiltins(projectDir, moduleRoot string, names []string) ([]ExtensionInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("discover: get home dir: %w", err)
	}

	return DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, names)
}

// DiscoverCustomHome is like Discover but accepts an explicit home directory.
func DiscoverCustomHome(projectDir, homeDir string, names []string) ([]ExtensionInfo, error) {
	var exts []ExtensionInfo

	seen := make(map[string]bool, len(names))

	for _, name := range names {
		if seen[name] {
			return nil, fmt.Errorf("discover: duplicate extension name %q", name)
		}

		seen[name] = true
		if !validExtName.MatchString(name) {
			return nil, fmt.Errorf("discover: invalid extension name %q (must match [a-zA-Z0-9_-]+)", name)
		}

		info, err := findExtension(projectDir, homeDir, name)
		if err != nil {
			return nil, err
		}

		exts = append(exts, *info)
	}

	return exts, nil
}

// DiscoverCustomHomeWithBuiltins is like DiscoverCustomHome but falls back to
// built-in extensions under moduleRoot/extensions/{name}/.
func DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot string, names []string) ([]ExtensionInfo, error) {
	var exts []ExtensionInfo

	seen := make(map[string]bool, len(names))

	for _, name := range names {
		if seen[name] {
			return nil, fmt.Errorf("discover: duplicate extension name %q", name)
		}

		seen[name] = true
		if !validExtName.MatchString(name) {
			return nil, fmt.Errorf("discover: invalid extension name %q (must match [a-zA-Z0-9_-]+)", name)
		}

		info, err := findExtensionWithBuiltins(projectDir, homeDir, moduleRoot, name)
		if err != nil {
			return nil, err
		}

		exts = append(exts, *info)
	}

	return exts, nil
}

func findExtension(projectDir, homeDir, name string) (*ExtensionInfo, error) {
	localDir := filepath.Join(projectDir, ".weave", "extensions", name)

	stat, statErr := os.Stat(localDir)
	if statErr == nil {
		if !stat.IsDir() {
			return nil, fmt.Errorf("discover: local extension path %q exists but is not a directory", localDir)
		}

		goFiles, err := collectGoFiles(localDir)
		if err != nil {
			return nil, fmt.Errorf("discover: local extension %q: %w", name, err)
		}

		return &ExtensionInfo{
			Name:    name,
			Dir:     localDir,
			GoFiles: goFiles,
		}, nil
	}

	if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("discover: local extension path %q: %w", localDir, statErr)
	}

	globalDir := filepath.Join(homeDir, ".weave", "extensions", name)

	goFiles, err := collectGoFiles(globalDir)
	if err != nil {
		return nil, fmt.Errorf("discover: extension %q not found in .weave/extensions/ (local or global): %w", name, err)
	}

	if len(goFiles) > 0 {
		return &ExtensionInfo{
			Name:    name,
			Dir:     globalDir,
			GoFiles: goFiles,
		}, nil
	}

	return nil, fmt.Errorf("discover: extension %q not found in .weave/extensions/ (local or global)", name)
}

func findExtensionWithBuiltins(projectDir, homeDir, moduleRoot, name string) (*ExtensionInfo, error) {
	// Try local then global first.
	info, err := findExtension(projectDir, homeDir, name)
	if err == nil {
		return info, nil
	}

	// If a local extension directory exists (or stat failed for a reason other
	// than "not found"), surface the original error instead of silently falling
	// back to built-in. This catches permission errors and other broken states.
	localDir := filepath.Join(projectDir, ".weave", "extensions", name)
	if _, statErr := os.Stat(localDir); statErr == nil || !os.IsNotExist(statErr) {
		return nil, err
	}

	// Same guard for global: if a user-installed global extension exists but is
	// broken, surface the error rather than silently falling back to built-in.
	globalDir := filepath.Join(homeDir, ".weave", "extensions", name)
	if _, statErr := os.Stat(globalDir); statErr == nil || !os.IsNotExist(statErr) {
		return nil, err
	}

	// Fallback: try built-in extension under moduleRoot/extensions/{name}/
	// and also one level deeper: moduleRoot/extensions/*/{name}/
	info, foundErr := findBuiltin(moduleRoot, name)
	if foundErr != nil {
		return nil, fmt.Errorf("discover: extension %q not found (local, global, or built-in): %w", name, err)
	}

	return info, nil
}

// findBuiltin searches for a built-in extension first at
// moduleRoot/extensions/{name}/ then one level deeper at
// moduleRoot/extensions/*/{name}/, then in TUI-specific extensions at
// moduleRoot/extensions/ui/tui/extensions/{name}/.
func findBuiltin(moduleRoot, name string) (*ExtensionInfo, error) {
	builtinDir := filepath.Join(moduleRoot, "extensions", name)

	if goFiles, err := collectGoFiles(builtinDir); err == nil && len(goFiles) > 0 {
		return &ExtensionInfo{Name: name, Dir: builtinDir, GoFiles: goFiles}, nil
	}

	// Search one level deeper: extensions/*/{name}/
	extRoot := filepath.Join(moduleRoot, "extensions")

	entries, err := os.ReadDir(extRoot)
	if err != nil {
		return nil, fmt.Errorf("extension %q not found in built-ins", name)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		nestedDir := filepath.Join(extRoot, e.Name(), name)

		if goFiles, goErr := collectGoFiles(nestedDir); goErr == nil && len(goFiles) > 0 {
			return &ExtensionInfo{Name: name, Dir: nestedDir, GoFiles: goFiles}, nil
		}
	}

	// Fallback: search TUI-specific extensions at extensions/ui/tui/extensions/{name}/
	tuiExtDir := filepath.Join(moduleRoot, "extensions", "ui", "tui", "extensions", name)

	if goFiles, err := collectGoFiles(tuiExtDir); err == nil && len(goFiles) > 0 {
		return &ExtensionInfo{Name: name, Dir: tuiExtDir, GoFiles: goFiles}, nil
	}

	return nil, fmt.Errorf("extension %q not found in built-ins", name)
}

func collectGoFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect go files in %s: %w", dir, err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .go files in %s", dir)
	}

	sort.Strings(files)

	return files, nil
}
