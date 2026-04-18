package launcher

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLookup_Miss(t *testing.T) {
	c := NewCache(t.TempDir())

	path, found := c.Lookup("nonexistent")
	if found {
		t.Error("expected cache miss")
	}

	if path != "" {
		t.Errorf("expected empty path, got %s", path)
	}
}

func TestLookup_Hit(t *testing.T) {
	root := t.TempDir()
	hash := "abc123"

	dir := filepath.Join(root, hash)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	binPath := filepath.Join(dir, "weave")
	if err := os.WriteFile(binPath, []byte("binary"), 0o750); err != nil {
		t.Fatal(err)
	}

	c := NewCache(root)

	path, found := c.Lookup(hash)
	if !found {
		t.Error("expected cache hit")
	}

	if path != binPath {
		t.Errorf("expected %s, got %s", binPath, path)
	}
}

func TestLookup_DirInsteadOfFile(t *testing.T) {
	root := t.TempDir()
	hash := "abc123"

	dir := filepath.Join(root, hash, "weave")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	c := NewCache(root)

	_, found := c.Lookup(hash)
	if found {
		t.Error("should not find directory as binary")
	}
}

func TestStore_CreatesDirAndCopies(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "mybin")

	content := []byte("hello world")
	if err := os.WriteFile(src, content, 0o755); err != nil {
		t.Fatal(err)
	}

	c := NewCache(root)
	if err := c.Store("deadbeef", src); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	cached, found := c.Lookup("deadbeef")
	if !found {
		t.Fatal("expected to find stored binary")
	}

	got, err := os.ReadFile(cached)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	info, err := os.Stat(cached)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm()&0o111 == 0 {
		t.Error("cached binary should be executable")
	}
}

func TestStore_OverwriteExisting(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()

	src1 := filepath.Join(srcDir, "v1")
	if err := os.WriteFile(src1, []byte("v1"), 0o750); err != nil {
		t.Fatal(err)
	}

	c := NewCache(root)
	if err := c.Store("hash1", src1); err != nil {
		t.Fatal(err)
	}

	src2 := filepath.Join(srcDir, "v2")
	if err := os.WriteFile(src2, []byte("v2"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := c.Store("hash1", src2); err != nil {
		t.Fatalf("Store overwrite failed: %v", err)
	}

	cached, _ := c.Lookup("hash1")

	got, _ := os.ReadFile(cached)
	if string(got) != "v2" {
		t.Errorf("expected overwritten content, got %q", got)
	}
}

func TestStore_MissingSource(t *testing.T) {
	c := NewCache(t.TempDir())

	err := c.Store("hash", "/nonexistent/path/binary")
	if err == nil {
		t.Error("expected error for missing source file")
	}
}

func TestDefaultCacheDir(t *testing.T) {
	dir, err := DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir failed: %v", err)
	}

	home, _ := os.UserHomeDir()

	expected := filepath.Join(home, ".weave", "bin")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}
