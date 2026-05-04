package launcher

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"weave/config"
)

// isPath reports whether an extension entry is a filesystem path rather than a
// bare extension name. Delegates to config.IsPathEntry.
func isPath(s string) bool { return config.IsPathEntry(s) }

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
func Discover(projectDir string, names []string) ([]ExtensionInfo, []string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("discover: get home dir: %w", err)
	}

	return DiscoverCustomHome(projectDir, homeDir, names)
}

// DiscoverWithBuiltins is like Discover but also checks built-in extensions
// under moduleRoot/extensions/{name}/ as a final fallback. This allows core
// extensions shipped with weave to be found without installing them.
// configDir is used to resolve relative path entries; when empty, projectDir is used.
func DiscoverWithBuiltins(projectDir, moduleRoot string, names []string, configDir ...string) ([]ExtensionInfo, []string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("discover: get home dir: %w", err)
	}

	return DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, names, configDir...)
}

// DiscoverCustomHome is like Discover but accepts an explicit home directory.
func DiscoverCustomHome(projectDir, homeDir string, names []string) ([]ExtensionInfo, []string, error) {
	var exts []ExtensionInfo

	seen := make(map[string]bool, len(names))

	for _, name := range names {
		if seen[name] {
			return nil, nil, fmt.Errorf("discover: duplicate extension name %q", name)
		}

		seen[name] = true
		if !config.ValidExtName(name) {
			return nil, nil, fmt.Errorf("discover: invalid extension name %q (must match [a-zA-Z0-9_-]+)", name)
		}

		info, err := findExtension(projectDir, homeDir, name)
		if err != nil {
			return nil, nil, err
		}

		exts = append(exts, *info)
	}

	return exts, nil, nil
}

// DiscoverCustomHomeWithBuiltins is like DiscoverCustomHome but falls back to
// built-in extensions under moduleRoot/extensions/{name}/. Extension entries
// that look like filesystem paths (prefixed with ./, ../, /, or ~/) are
// resolved directly instead of going through the name-based discovery hierarchy.
// configDir is used to resolve relative path entries; when empty, projectDir is used.
// Returns extension infos, shadow warnings, and any error.
func DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot string, names []string, configDir ...string) ([]ExtensionInfo, []string, error) {
	var (
		exts     []ExtensionInfo
		warnings []string
	)

	resolveDir := projectDir
	if len(configDir) > 0 && configDir[0] != "" {
		resolveDir = configDir[0]
	}

	seen := make(map[string]bool, len(names))

	for _, entry := range names {
		if isPath(entry) {
			info, err := resolvePathExtension(entry, resolveDir)
			if err != nil {
				return nil, nil, err
			}

			if seen[info.Name] {
				return nil, nil, fmt.Errorf("discover: duplicate extension name %q", info.Name)
			}

			seen[info.Name] = true
			exts = append(exts, *info)

			continue
		}

		if seen[entry] {
			return nil, nil, fmt.Errorf("discover: duplicate extension name %q", entry)
		}

		seen[entry] = true
		if !config.ValidExtName(entry) {
			return nil, nil, fmt.Errorf("discover: invalid extension name %q (must match [a-zA-Z0-9_-]+)", entry)
		}

		info, err := findExtensionWithBuiltins(projectDir, homeDir, moduleRoot, entry)
		if err != nil {
			return nil, nil, err
		}

		if w := checkBuiltinShadow(moduleRoot, entry, info.Dir); w != "" {
			warnings = append(warnings, w)
		}

		exts = append(exts, *info)
	}

	return exts, warnings, nil
}

// resolvePathExtension resolves a path-like extension entry to its ExtensionInfo.
func resolvePathExtension(entry, configDir string) (*ExtensionInfo, error) {
	dir, err := config.ResolveExtPath(entry, configDir)
	if err != nil {
		return nil, fmt.Errorf("discover: resolve path %q: %w", entry, err)
	}

	stat, statErr := os.Stat(dir)
	if statErr != nil {
		return nil, fmt.Errorf("discover: extension path %q: %w", entry, statErr)
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("discover: extension path %q (%s) is not a directory", entry, dir)
	}

	goFiles, err := collectGoFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("discover: extension path %q (%s): %w", entry, dir, err)
	}

	return &ExtensionInfo{
		Name:    filepath.Base(dir),
		Dir:     dir,
		GoFiles: goFiles,
	}, nil
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

// checkBuiltinShadow returns a warning if the resolved extension directory is
// not a built-in path but a built-in with the same name also exists.
func checkBuiltinShadow(moduleRoot, name, resolvedDir string) string {
	// If resolved from built-in paths, no shadow.
	builtinRoot := filepath.Join(moduleRoot, "extensions") + string(filepath.Separator)
	if strings.HasPrefix(resolvedDir+string(filepath.Separator), builtinRoot) {
		return ""
	}

	// Check if a built-in exists.
	if _, err := findBuiltin(moduleRoot, name); err != nil {
		return ""
	}

	return fmt.Sprintf("extension %q resolved from %s but also exists as built-in; local/global takes precedence", name, resolvedDir)
}

var warnLog = log.New(os.Stderr, "weave: ", 0)
