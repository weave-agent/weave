package launcher

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/weave-agent/weave/settings"
)

type ExtensionInfo struct {
	Name       string
	Dir        string
	GoFiles    []string
	ModulePath string // e.g. "github.com/weave-agent/weave/ext/tools/bash"; populated by builder
	IsUIExt    bool   // true if the extension registers UI elements (RegisterUI, RegisterUIExtension, or RegisterTUIExtension)
}

// AutoDiscover recursively scans extension directories to find all Go modules.
// It checks three roots in order of precedence: project-local, global, built-in.
// Within each root, it walks the directory tree looking for directories that
// contain both a go.mod file and at least one non-test .go file.
// Deduplication: earlier roots take precedence (local > global > built-in).
// The exclude list filters out extensions by name after discovery.
func AutoDiscover(projectDir, homeDir, moduleRoot string, exclude []string) ([]ExtensionInfo, error) {
	var exts []ExtensionInfo

	seen := make(map[string]bool)

	roots := []string{
		filepath.Join(projectDir, ".weave", "extensions"),
		filepath.Join(homeDir, ".weave", "extensions"),
		filepath.Join(moduleRoot, "extensions"),
	}

	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			if !os.IsNotExist(err) {
				slog.Warn("auto-discover: stat failed", "root", root, "error", err)
			}

			continue
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			return tryAddExtension(path, root, d, err, seen, &exts)
		})
		if err != nil {
			return nil, fmt.Errorf("auto-discover %s: %w", root, err)
		}
	}

	// Apply exclude list
	if len(exclude) > 0 {
		excludeSet := make(map[string]bool, len(exclude))
		for _, e := range exclude {
			excludeSet[e] = true
		}

		var filtered []ExtensionInfo

		for _, ext := range exts {
			if !excludeSet[ext.Name] {
				filtered = append(filtered, ext)
			}
		}

		exts = filtered
	}

	// Sort by name for deterministic output
	sort.Slice(exts, func(i, j int) bool {
		return exts[i].Name < exts[j].Name
	})

	return exts, nil
}

func tryAddExtension(path, root string, d fs.DirEntry, err error, seen map[string]bool, exts *[]ExtensionInfo) error {
	if err != nil {
		slog.Warn("auto-discover: walk error", "path", path, "error", err)

		if d != nil && d.IsDir() {
			return fs.SkipDir
		}

		return nil
	}

	if !d.IsDir() {
		return nil
	}

	if path == root {
		return nil // skip the root directory itself
	}

	// Skip hidden/VCS directories (e.g. .git, .hg).
	if strings.HasPrefix(d.Name(), ".") {
		return fs.SkipDir
	}

	// Check if this directory has a go.mod
	goModPath := filepath.Join(path, "go.mod")
	_, statErr := os.Stat(goModPath)

	hasGoMod := statErr == nil
	if !hasGoMod {
		return nil
	}

	// Collect .go files within module boundary
	goFiles, fileErr := collectGoFiles(path)
	if fileErr != nil {
		slog.Warn("auto-discover: file collection error", "error", fileErr)
		return nil
	}

	if len(goFiles) == 0 {
		return nil // no .go files in this module
	}

	name := filepath.Base(path)
	if !settings.ValidExtName(name) {
		slog.Warn("auto-discover: skipping invalid extension name", "name", name)
		return nil
	}

	if seen[name] {
		return nil // already found at higher precedence
	}

	seen[name] = true

	isUI := isUIExtension(path)

	*exts = append(*exts, ExtensionInfo{
		Name:    name,
		Dir:     path,
		GoFiles: goFiles,
		IsUIExt: isUI,
	})

	return nil
}

func collectGoFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			// Skip subdirectories that have their own go.mod (module boundary)
			if path != dir {
				if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
					return fs.SkipDir
				}
			}

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
		return nil, nil
	}

	sort.Strings(files)

	return files, nil
}
