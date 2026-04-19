package launcher

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// Cache manages per-hash binary caching under a root directory.
type Cache struct {
	Root string
}

// NewCache creates a Cache rooted at rootDir (typically ~/.weave/bin/).
func NewCache(rootDir string) *Cache {
	return &Cache{Root: rootDir}
}

// DefaultCacheDir returns ~/.weave/bin/.
func DefaultCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cache: get home dir: %w", err)
	}

	return filepath.Join(home, ".weave", "bin"), nil
}

// Lookup checks whether a cached binary exists for the given hash.
// Returns the binary path and true if found, or ("", false) otherwise.
func (c *Cache) Lookup(hash string) (string, bool) {
	binPath := filepath.Join(c.Root, hash, "weave")

	info, err := os.Stat(binPath)
	if err != nil {
		return "", false
	}

	if info.IsDir() {
		return "", false
	}

	return binPath, true
}

// Store copies the binary at src into the cache under the given hash.
// It writes to a PID-scoped temp file first, then renames atomically to
// avoid concurrent readers seeing a partial binary.
// Falls back to copy+delete when src and dst are on different filesystems.
func (c *Cache) Store(hash, src string) error {
	dir := filepath.Join(c.Root, hash)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cache: mkdir %s: %w", dir, err)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("cache: open source: %w", err)
	}
	defer srcFile.Close()

	dst := filepath.Join(dir, "weave")
	tmp := dst + ".tmp." + strconv.Itoa(os.Getpid())

	// Remove stale temp file from a crashed previous run.
	_ = os.Remove(tmp)

	dstFile, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o750)
	if err != nil {
		return fmt.Errorf("cache: create dest: %w", err)
	}

	cleanup := true

	defer func() {
		_ = dstFile.Close()

		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("cache: copy: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("cache: sync: %w", err)
	}

	cleanup = false

	if err := os.Rename(tmp, dst); err != nil {
		// Cross-device rename: fall back to copy+delete.
		if copyErr := copyFile(tmp, dst); copyErr != nil {
			return fmt.Errorf("cache: rename failed (%w) and cross-device copy failed: %w", err, copyErr)
		}

		_ = os.Remove(tmp)
	}

	return nil
}

// copyFile copies the contents of src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy open src: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o750)
	if err != nil {
		return fmt.Errorf("copy open dst: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy data: %w", err)
	}

	if err := out.Sync(); err != nil {
		return fmt.Errorf("copy sync: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("copy close: %w", err)
	}

	return nil
}
