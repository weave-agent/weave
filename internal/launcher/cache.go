package launcher

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	cacheBinaryName = "weave"
	cacheHashLength = 64
)

// DefaultMaxCacheSizeBytes is the default launcher binary cache size cap.
const DefaultMaxCacheSizeBytes int64 = 1 << 30

// DefaultAccessLeaseDuration makes eviction prefer older inactive entries before
// recently accessed entries. The newly stored entry is always protected.
const DefaultAccessLeaseDuration = 30 * time.Second

// Cache manages per-hash binary caching under a root directory.
type Cache struct {
	// Root is the directory containing per-hash launcher binary cache entries.
	Root string

	// MaxSizeBytes caps the cache after successful stores; zero uses the default, negative disables eviction.
	MaxSizeBytes int64

	// AccessLeaseDuration protects recently accessed entries from eviction; zero uses the default, negative disables protection.
	AccessLeaseDuration time.Duration
	now                 func() time.Time
}

// NewCache creates a Cache rooted at rootDir (typically ~/.weave/bin/).
func NewCache(rootDir string) *Cache {
	return &Cache{
		Root:                rootDir,
		MaxSizeBytes:        DefaultMaxCacheSizeBytes,
		AccessLeaseDuration: DefaultAccessLeaseDuration,
		now:                 time.Now,
	}
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
	dir, err := c.entryDir(hash)
	if err != nil {
		return "", false
	}

	binPath := filepath.Join(dir, cacheBinaryName)

	info, err := os.Stat(binPath)
	if err != nil {
		return "", false
	}

	if info.IsDir() {
		return "", false
	}

	_ = c.touchAccess(hash)

	return binPath, true
}

// Store copies the binary at src into the cache under the given hash.
// It writes to a PID-scoped temp file first, then renames atomically to
// avoid concurrent readers seeing a partial binary.
// Falls back to copy+delete when src and dst are on different filesystems.
func (c *Cache) Store(hash, src string) error {
	dir, err := c.entryDir(hash)
	if err != nil {
		return fmt.Errorf("cache: %w", err)
	}

	if mkdirErr := os.MkdirAll(dir, 0o750); mkdirErr != nil {
		return fmt.Errorf("cache: mkdir %s: %w", dir, mkdirErr)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("cache: open source: %w", err)
	}
	defer srcFile.Close()

	dst := filepath.Join(dir, cacheBinaryName)
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
		// Rename failed: fall back to copy to a temp name, then atomic rename.
		fallbackTmp := dst + ".tmp2." + strconv.Itoa(os.Getpid())
		_ = os.Remove(fallbackTmp)

		if copyErr := copyFile(tmp, fallbackTmp); copyErr != nil {
			return fmt.Errorf("cache: rename failed (%w) and copy failed: %w", err, copyErr)
		}

		if renameErr := os.Rename(fallbackTmp, dst); renameErr != nil {
			_ = os.Remove(fallbackTmp)

			return fmt.Errorf("cache: rename failed (%w) and fallback rename failed: %w", err, renameErr)
		}

		_ = os.Remove(tmp)
	}

	if err := c.touchAccess(hash); err != nil {
		return fmt.Errorf("cache: update access metadata: %w", err)
	}

	if err := c.evictToSize(hash); err != nil {
		return fmt.Errorf("cache: evict: %w", err)
	}

	return nil
}

// Clean removes all launcher binary cache entries under the cache root.
func (c *Cache) Clean() (int, error) {
	entries, err := c.cacheEntries()
	if err != nil {
		return 0, err
	}

	removed := 0

	for _, entry := range entries {
		if err := os.RemoveAll(entry.dir); err != nil {
			return removed, fmt.Errorf("cache: remove %s: %w", entry.hash, err)
		}

		removed++
	}

	return removed, nil
}

type cacheEntry struct {
	hash     string
	dir      string
	size     int64
	accessed time.Time
}

func (c *Cache) evictToSize(protectHash string) error {
	limit, ok := c.sizeLimit()
	if !ok {
		return nil
	}

	entries, err := c.cacheEntries()
	if err != nil {
		return err
	}

	var total int64
	for _, entry := range entries {
		total += entry.size
	}

	if total <= limit {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].accessed.Equal(entries[j].accessed) {
			return entries[i].hash < entries[j].hash
		}

		return entries[i].accessed.Before(entries[j].accessed)
	})

	now := c.timeNow()
	removed := make(map[string]bool)

	for pass := range 2 {
		evictLeased := pass == 1

		for _, entry := range entries {
			if total <= limit {
				break
			}

			if entry.hash == protectHash || removed[entry.hash] {
				continue
			}

			if !evictLeased && c.accessLeaseActive(now, entry.accessed) {
				continue
			}

			if err := os.RemoveAll(entry.dir); err != nil {
				return fmt.Errorf("remove %s: %w", entry.hash, err)
			}

			removed[entry.hash] = true
			total -= entry.size
		}
	}

	return nil
}

func (c *Cache) cacheEntries() ([]cacheEntry, error) {
	if c.Root == "" {
		return nil, errors.New("cache root is empty")
	}

	entries, err := os.ReadDir(c.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("cache: read root: %w", err)
	}

	result := make([]cacheEntry, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if !isCacheHash(entry.Name()) {
			continue
		}

		dir := filepath.Join(c.Root, entry.Name())
		binPath := filepath.Join(dir, cacheBinaryName)

		binInfo, err := os.Stat(binPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return nil, fmt.Errorf("cache: stat %s: %w", binPath, err)
		}

		if binInfo.IsDir() {
			continue
		}

		accessed := binInfo.ModTime()
		if dirInfo, statErr := os.Stat(dir); statErr == nil {
			accessed = dirInfo.ModTime()
		}

		result = append(result, cacheEntry{
			hash:     entry.Name(),
			dir:      dir,
			size:     binInfo.Size(),
			accessed: accessed,
		})
	}

	return result, nil
}

func (c *Cache) touchAccess(hash string) error {
	dir, err := c.entryDir(hash)
	if err != nil {
		return err
	}

	now := c.timeNow()
	if err := os.Chtimes(dir, now, now); err != nil {
		return fmt.Errorf("update access time: %w", err)
	}

	return nil
}

func (c *Cache) entryDir(hash string) (string, error) {
	if !isCacheHash(hash) || strings.ContainsAny(hash, `/\`) {
		return "", fmt.Errorf("invalid cache hash %q", hash)
	}

	return filepath.Join(c.Root, hash), nil
}

func isCacheHash(hash string) bool {
	if len(hash) != cacheHashLength {
		return false
	}

	for _, r := range hash {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}

	return true
}

func (c *Cache) sizeLimit() (int64, bool) {
	if c.MaxSizeBytes < 0 {
		return 0, false
	}

	if c.MaxSizeBytes == 0 {
		return DefaultMaxCacheSizeBytes, true
	}

	return c.MaxSizeBytes, true
}

func (c *Cache) accessLeaseActive(now, accessed time.Time) bool {
	lease := c.accessLeaseDuration()
	if lease <= 0 {
		return false
	}

	return now.Sub(accessed) < lease
}

func (c *Cache) accessLeaseDuration() time.Duration {
	if c.AccessLeaseDuration < 0 {
		return 0
	}

	if c.AccessLeaseDuration == 0 {
		return DefaultAccessLeaseDuration
	}

	return c.AccessLeaseDuration
}

func (c *Cache) timeNow() time.Time {
	if c.now != nil {
		return c.now()
	}

	return time.Now()
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
