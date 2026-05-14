package launcher

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const maxUIExtScanSize = 10 << 20 // 10 MB

var errUIExtFound = errors.New("ui extension found")

// isUIExtension reports whether the directory at dir contains a UI extension.
// It scans .go files for RegisterUIExtension(, RegisterUI(, or RegisterTUIExtension( calls,
// respecting module boundaries (subdirectories with their own go.mod are skipped).
func isUIExtension(dir string) bool {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("launcher: skip unreadable entry", "path", path, "error", err)

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

		data, readErr := os.ReadFile(filepath.Join(dir, rel))
		if readErr != nil {
			slog.Warn("launcher: skip unreadable file", "path", path, "error", readErr)

			return nil
		}

		if int64(len(data)) > maxUIExtScanSize {
			slog.Warn("launcher: skip oversized file", "path", path)

			return nil
		}

		src := string(data)
		if strings.Contains(src, "RegisterUIExtension(") || strings.Contains(src, "RegisterUI(") || strings.Contains(src, "RegisterTUIExtension(") {
			return errUIExtFound
		}

		return nil
	})

	return errors.Is(err, errUIExtFound)
}
