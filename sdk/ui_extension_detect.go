package sdk

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const maxUIExtScanSize = 10 << 20 // 10 MB

var errUIExtFound = errors.New("ui extension found")

// IsUIExtension reports whether the directory at dir contains a UI extension.
// It scans .go files for RegisterUIExtension( or RegisterUI( calls,
// respecting module boundaries (subdirectories with their own go.mod are skipped).
func IsUIExtension(dir string) bool {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("sdk: skip unreadable entry %s: %v", path, err)

			return nil
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

		info, statErr := os.Stat(filepath.Join(dir, rel))
		if statErr != nil || info.Size() > maxUIExtScanSize {
			log.Printf("sdk: skip unreadable or oversized file %s", path)

			return nil //nolint:nilerr // logged above, skip unreadable or oversized files by design
		}

		data, readErr := os.ReadFile(filepath.Join(dir, rel))
		if readErr != nil {
			log.Printf("sdk: skip unreadable file %s: %v", path, readErr)

			return nil
		}

		src := string(data)
		if strings.Contains(src, "RegisterUIExtension(") || strings.Contains(src, "RegisterUI(") {
			return errUIExtFound
		}

		return nil
	})

	return errors.Is(err, errUIExtFound)
}
