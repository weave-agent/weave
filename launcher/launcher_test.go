package launcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_NoExtensions(t *testing.T) {
	l := &Launcher{
		Cache:      NewCache(t.TempDir()),
		Build:      func(string, string, string, []string, []ExtensionInfo) (string, error) { return "", nil },
		ModuleRoot: "/fake",
	}

	err := l.Run(context.Background(), t.TempDir(), nil, nil, "", "loop", nil)
	if err == nil {
		t.Fatal("expected error for empty extensions")
	}
}

func TestRun_DiscoveryFails(t *testing.T) {
	l := &Launcher{
		Cache:      NewCache(t.TempDir()),
		Build:      func(string, string, string, []string, []ExtensionInfo) (string, error) { return "", nil },
		ModuleRoot: "/fake",
	}

	err := l.Run(context.Background(), t.TempDir(), []string{"nonexistent_ext"}, nil, "", "loop", nil)
	if err == nil {
		t.Fatal("expected error for missing extension")
	}
}

func TestRun_BuildFails(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", "package noop")

	buildErr := error(fmtError("mock build failure"))
	l := &Launcher{
		Cache: NewCache(t.TempDir()),
		Build: func(string, string, string, []string, []ExtensionInfo) (string, error) {
			return "", buildErr
		},
		ModuleRoot:  "/fake",
		BuildTmpDir: t.TempDir(),
	}

	err := l.Run(context.Background(), projectDir, []string{"noop"}, nil, "", "loop", nil)
	if err == nil {
		t.Fatal("expected error for build failure")
	}
}

func TestRun_CacheHit(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", "package noop")

	cacheDir := t.TempDir()
	c := NewCache(cacheDir)

	// Pre-seed the cache with a binary
	// First, discover and compute hash to know what hash to seed
	exts, err := Discover(projectDir, []string{"noop"})
	if err != nil {
		t.Fatal(err)
	}

	hash, err := ComputeHash(exts, "")
	if err != nil {
		t.Fatal(err)
	}

	// Create a fake cached binary that exits successfully
	fakeBin := filepath.Join(cacheDir, hash, "weave")
	if err := os.MkdirAll(filepath.Join(cacheDir, hash), 0o750); err != nil {
		t.Fatal(err)
	}
	// Write a simple script that just exits 0
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o750); err != nil {
		t.Fatal(err)
	}

	binPath, found := c.Lookup(hash)
	if !found {
		t.Fatal("expected cache hit")
	}

	if binPath != fakeBin {
		t.Errorf("expected %s, got %s", fakeBin, binPath)
	}
}

func TestRun_FullPipelineWithMockBuild(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", "package noop")

	cacheDir := t.TempDir()
	buildDir := t.TempDir()

	l := &Launcher{
		Cache: NewCache(cacheDir),
		Build: func(dir, _, _ string, _ []string, _ []ExtensionInfo) (string, error) {
			binPath := filepath.Join(dir, "weave")
			if err := os.WriteFile(binPath, []byte("fake-binary"), 0o750); err != nil {
				return "", fmt.Errorf("write fake binary: %w", err)
			}

			return binPath, nil
		},
		ModuleRoot:  "/fake",
		BuildTmpDir: buildDir,
	}

	// We can't call Run() because syscall.Exec replaces the process.
	// Test the pipeline steps manually (discover, hash, cache miss, build, cache store).
	exts, err := Discover(projectDir, []string{"noop"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	hash, err := ComputeHash(exts, "")
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	_, found := l.Cache.Lookup(hash)
	if found {
		t.Fatal("expected cache miss for new extension")
	}

	// Use the buildAndCache method indirectly by creating a launcher with a
	// non-exec-ing exec. Instead, test buildAndCache directly.
	binPath, err := l.buildAndCache(hash, "loop", nil, exts)
	if err != nil {
		t.Fatalf("buildAndCache: %v", err)
	}

	if binPath == "" {
		t.Fatal("expected non-empty binPath")
	}

	// Verify cached
	cached, found := l.Cache.Lookup(hash)
	if !found {
		t.Fatal("expected cache hit after buildAndCache")
	}

	if cached != binPath {
		t.Errorf("cached path %q != built path %q", cached, binPath)
	}

	// Verify the cached binary content
	got, err := os.ReadFile(cached)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "fake-binary" {
		t.Errorf("cached content = %q, want %q", got, "fake-binary")
	}
}

func TestRun_SecondRunUsesCache(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", "package noop")

	cacheDir := t.TempDir()
	buildDir := t.TempDir()

	buildCount := 0
	l := &Launcher{
		Cache: NewCache(cacheDir),
		Build: func(dir, _, _ string, _ []string, _ []ExtensionInfo) (string, error) {
			buildCount++

			binPath := filepath.Join(dir, "weave")

			if err := os.WriteFile(binPath, []byte("fake-binary"), 0o750); err != nil {
				return "", fmt.Errorf("write fake binary: %w", err)
			}

			return binPath, nil
		},
		ModuleRoot:  "/fake",
		BuildTmpDir: buildDir,
	}

	exts, err := Discover(projectDir, []string{"noop"})
	if err != nil {
		t.Fatal(err)
	}

	hash, err := ComputeHash(exts, "")
	if err != nil {
		t.Fatal(err)
	}

	// First build
	_, err = l.buildAndCache(hash, "loop", nil, exts)
	if err != nil {
		t.Fatal(err)
	}

	if buildCount != 1 {
		t.Errorf("expected 1 build, got %d", buildCount)
	}

	// Second "run" — cache should hit, build not called again
	_, found := l.Cache.Lookup(hash)
	if !found {
		t.Fatal("expected cache hit on second run")
	}

	if buildCount != 1 {
		t.Errorf("expected 1 build after cache hit, got %d", buildCount)
	}
}

func TestBuildDir_CustomTmpDir(t *testing.T) {
	l := &Launcher{BuildTmpDir: "/custom/tmp"}
	dir := l.buildDir("abc123")

	expected := "/custom/tmp/abc123"
	if dir != expected {
		t.Errorf("buildDir = %q, want %q", dir, expected)
	}
}

func TestBuildDir_DefaultTmpDir(t *testing.T) {
	l := &Launcher{}
	dir := l.buildDir("abc123")

	expected := filepath.Join(os.TempDir(), "weave-build-abc123")
	if dir != expected {
		t.Errorf("buildDir = %q, want %q", dir, expected)
	}
}

// fmtError wraps a string as an error for test use.
type fmtError string

func (e fmtError) Error() string { return string(e) }
