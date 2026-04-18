package launcher

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
func (c *Cache) Store(hash, src string) error {
	dir := filepath.Join(c.Root, hash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cache: mkdir %s: %w", dir, err)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("cache: open source: %w", err)
	}
	defer srcFile.Close()

	dst := filepath.Join(dir, "weave")

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("cache: create dest: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("cache: copy: %w", err)
	}

	return nil
}
