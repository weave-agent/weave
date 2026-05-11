package sdk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var errUIExtFound = errors.New("ui extension found")

// IsUIExtension reports whether the directory at dir contains a UI extension.
// It scans .go files for RegisterUIExtension( or RegisterUI( calls,
// respecting module boundaries (subdirectories with their own go.mod are skipped).
func IsUIExtension(dir string) bool {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries
		}

		if d.IsDir() {
			if path != dir {
				if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
					return fs.SkipDir
				}
			}

			return nil
		}

		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		// Build root-scoped path to avoid symlink TOCTOU (gosec G122).
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}

		data, readErr := os.ReadFile(filepath.Join(dir, rel))
		if readErr != nil {
			return nil //nolint:nilerr // skip unreadable files
		}

		src := string(data)
		if strings.Contains(src, "RegisterUIExtension(") || strings.Contains(src, "RegisterUI(") {
			return errUIExtFound
		}

		return nil
	})

	return errors.Is(err, errUIExtFound)
}
