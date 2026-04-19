package launcher

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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
	hash := "abc123"

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
	hash := "abc123"

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
	require.NoError(t, c.Store("deadbeef", src))

	cached, found := c.Lookup("deadbeef")
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
	require.NoError(t, c.Store("hash1", src1))

	src2 := filepath.Join(srcDir, "v2")
	require.NoError(t, os.WriteFile(src2, []byte("v2"), 0o750))

	require.NoError(t, c.Store("hash1", src2))

	cached, _ := c.Lookup("hash1")

	got, _ := os.ReadFile(cached)
	assert.Equal(t, "v2", string(got))
}

func TestStore_MissingSource(t *testing.T) {
	c := NewCache(t.TempDir())

	err := c.Store("hash", "/nonexistent/path/binary")
	require.Error(t, err)
}

func TestDefaultCacheDir(t *testing.T) {
	dir, err := DefaultCacheDir()
	require.NoError(t, err)

	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".weave", "bin"), dir)
}
