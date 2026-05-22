package launcher

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookup_Miss(t *testing.T) {
	c := NewCache(t.TempDir())

	path, found := c.Lookup("nonexistent")
	assert.False(t, found)
	assert.Empty(t, path)
}

func TestLookup_Hit(t *testing.T) {
	root := t.TempDir()
	hash := cacheTestHash("a")

	dir := filepath.Join(root, hash)
	require.NoError(t, os.MkdirAll(dir, 0o750))

	binPath := filepath.Join(dir, "weave")
	require.NoError(t, os.WriteFile(binPath, []byte("binary"), 0o750))

	c := NewCache(root)

	path, found := c.Lookup(hash)
	require.True(t, found)
	assert.Equal(t, binPath, path)
}

func TestLookup_DirInsteadOfFile(t *testing.T) {
	root := t.TempDir()
	hash := cacheTestHash("a")

	dir := filepath.Join(root, hash, "weave")
	require.NoError(t, os.MkdirAll(dir, 0o750))

	c := NewCache(root)

	_, found := c.Lookup(hash)
	assert.False(t, found, "should not find directory as binary")
}

func TestStore_CreatesDirAndCopies(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "mybin")

	content := []byte("hello world")
	require.NoError(t, os.WriteFile(src, content, 0o755))

	c := NewCache(root)
	hash := cacheTestHash("a")
	require.NoError(t, c.Store(hash, src))

	cached, found := c.Lookup(hash)
	require.True(t, found)

	got, err := os.ReadFile(cached)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(got, content))

	info, err := os.Stat(cached)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode().Perm()&0o111, "cached binary should be executable")
}

func TestStore_OverwriteExisting(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()

	src1 := filepath.Join(srcDir, "v1")
	require.NoError(t, os.WriteFile(src1, []byte("v1"), 0o750))

	c := NewCache(root)
	hash := cacheTestHash("a")
	require.NoError(t, c.Store(hash, src1))

	src2 := filepath.Join(srcDir, "v2")
	require.NoError(t, os.WriteFile(src2, []byte("v2"), 0o750))

	require.NoError(t, c.Store(hash, src2))

	cached, _ := c.Lookup(hash)

	got, _ := os.ReadFile(cached)
	assert.Equal(t, "v2", string(got))
}

func TestStore_MissingSource(t *testing.T) {
	c := NewCache(t.TempDir())

	err := c.Store(cacheTestHash("a"), "/nonexistent/path/binary")
	require.Error(t, err)
}

func TestStoreAndLookup_UpdateAccessTime(t *testing.T) {
	root := t.TempDir()
	src := writeCacheTestBinary(t, "binary")

	times := []time.Time{
		time.Unix(10, 0),
		time.Unix(20, 0),
	}

	c := NewCache(root)
	hash := cacheTestHash("a")
	c.now = func() time.Time {
		require.NotEmpty(t, times)

		next := times[0]
		times = times[1:]

		return next
	}

	require.NoError(t, c.Store(hash, src))
	assert.True(t, cacheEntryModTime(t, c, hash).Equal(time.Unix(10, 0)))

	_, found := c.Lookup(hash)
	require.True(t, found)
	assert.True(t, cacheEntryModTime(t, c, hash).Equal(time.Unix(20, 0)))
}

func TestStore_EvictsLeastRecentlyUsedEntry(t *testing.T) {
	root := t.TempDir()
	src := writeCacheTestBinary(t, "0123456789")
	c := newTickingCache(root)
	c.MaxSizeBytes = -1
	c.AccessLeaseDuration = -1

	oldHash := cacheTestHash("a")
	recentHash := cacheTestHash("b")
	newestHash := cacheTestHash("c")

	require.NoError(t, c.Store(oldHash, src))
	require.NoError(t, c.Store(recentHash, src))

	_, found := c.Lookup(oldHash)
	require.True(t, found)

	c.MaxSizeBytes = cacheTotalSize(t, c)
	require.NoError(t, c.Store(newestHash, src))

	_, found = c.Lookup(recentHash)
	assert.False(t, found, "least recently used entry should be evicted")

	_, found = c.Lookup(oldHash)
	assert.True(t, found, "lookup should refresh access metadata")

	_, found = c.Lookup(newestHash)
	assert.True(t, found, "newly stored entry should be protected from eviction")
}

func TestStore_PrefersInactiveEntriesDuringEviction(t *testing.T) {
	root := t.TempDir()
	src := writeCacheTestBinary(t, "0123456789")
	c := NewCache(root)
	c.MaxSizeBytes = -1
	c.AccessLeaseDuration = time.Minute

	now := time.Unix(1000, 0)
	c.now = func() time.Time {
		return now
	}

	now = now.Add(-time.Hour)

	oldHash := cacheTestHash("a")
	activeHash := cacheTestHash("b")
	newestHash := cacheTestHash("c")

	require.NoError(t, c.Store(oldHash, src))
	require.NoError(t, c.Store(activeHash, src))

	now = time.Unix(1000, 0)
	_, found := c.Lookup(activeHash)
	require.True(t, found)

	c.MaxSizeBytes = 20
	now = now.Add(time.Second)

	require.NoError(t, c.Store(newestHash, src))

	_, found = c.Lookup(oldHash)
	assert.False(t, found, "old inactive entry should be evicted")

	_, found = c.Lookup(activeHash)
	assert.True(t, found, "recently accessed entry should be kept when inactive entries satisfy the limit")

	_, found = c.Lookup(newestHash)
	assert.True(t, found, "newly stored entry should be protected from eviction")
}

func TestStore_EvictsRecentlyAccessedEntriesWhenNeededForSizeLimit(t *testing.T) {
	root := t.TempDir()
	src := writeCacheTestBinary(t, "0123456789")
	c := NewCache(root)
	c.MaxSizeBytes = -1
	c.AccessLeaseDuration = time.Minute

	now := time.Unix(1000, 0)
	c.now = func() time.Time {
		return now
	}

	now = now.Add(-time.Hour)

	oldHash := cacheTestHash("a")
	activeHash := cacheTestHash("b")
	newestHash := cacheTestHash("c")

	require.NoError(t, c.Store(oldHash, src))
	require.NoError(t, c.Store(activeHash, src))

	now = time.Unix(1000, 0)
	_, found := c.Lookup(activeHash)
	require.True(t, found)

	c.MaxSizeBytes = 15
	now = now.Add(time.Second)

	require.NoError(t, c.Store(newestHash, src))

	_, found = c.Lookup(oldHash)
	assert.False(t, found, "old inactive entry should be evicted first")

	_, found = c.Lookup(activeHash)
	assert.False(t, found, "recently accessed entry should be evicted when needed to satisfy the size limit")

	_, found = c.Lookup(newestHash)
	assert.True(t, found, "newly stored entry should be protected from eviction")
}

func TestStore_KeepsNewEntryWhenEntryExceedsLimit(t *testing.T) {
	src := writeCacheTestBinary(t, "this binary is larger than the cache limit")
	c := NewCache(t.TempDir())
	c.MaxSizeBytes = 1
	hash := cacheTestHash("a")

	require.NoError(t, c.Store(hash, src))

	_, found := c.Lookup(hash)
	assert.True(t, found, "store should not evict the entry it just wrote")
}

func TestClean_RemovesOnlyLauncherCacheEntries(t *testing.T) {
	root := t.TempDir()
	src := writeCacheTestBinary(t, "binary")
	c := NewCache(root)
	hash := cacheTestHash("a")

	require.NoError(t, c.Store(hash, src))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "not-cache"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "not-cache", "file"), []byte("keep"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "not-cache", "weave"), []byte("keep"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "missing-binary"), 0o750))

	removed, err := c.Clean()
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	assert.NoFileExists(t, filepath.Join(root, hash, "weave"))
	assert.FileExists(t, filepath.Join(root, "notes.txt"))
	assert.FileExists(t, filepath.Join(root, "not-cache", "file"))
	assert.FileExists(t, filepath.Join(root, "not-cache", "weave"))
	assert.DirExists(t, filepath.Join(root, "missing-binary"))
}

func TestDefaultCacheDir(t *testing.T) {
	dir, err := DefaultCacheDir()
	require.NoError(t, err)

	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".weave", "bin"), dir)
}

func writeCacheTestBinary(t *testing.T, content string) string {
	t.Helper()

	src := filepath.Join(t.TempDir(), "weave")
	require.NoError(t, os.WriteFile(src, []byte(content), 0o750))

	return src
}

func newTickingCache(root string) *Cache {
	c := NewCache(root)

	var tick int64

	c.now = func() time.Time {
		tick++

		return time.Unix(tick, 0)
	}

	return c
}

func cacheEntryModTime(t *testing.T, c *Cache, hash string) time.Time {
	t.Helper()

	info, err := os.Stat(filepath.Join(c.Root, hash))
	require.NoError(t, err)

	return info.ModTime()
}

func cacheTotalSize(t *testing.T, c *Cache) int64 {
	t.Helper()

	entries, err := c.cacheEntries()
	require.NoError(t, err)

	var total int64
	for _, entry := range entries {
		total += entry.size
	}

	return total
}

func cacheTestHash(char string) string {
	return strings.Repeat(char, cacheHashLength)
}
