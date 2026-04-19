package launcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_NoExtensions(t *testing.T) {
	l := &Launcher{
		Cache:      NewCache(t.TempDir()),
		Build:      func(string, string, string, []string, []ExtensionInfo) (string, error) { return "", nil },
		ModuleRoot: "/fake",
	}

	err := l.Run(context.Background(), t.TempDir(), nil, nil, "", "loop", nil)
	require.Error(t, err, "expected error for empty extensions")
}

func TestRun_DiscoveryFails(t *testing.T) {
	l := &Launcher{
		Cache:      NewCache(t.TempDir()),
		Build:      func(string, string, string, []string, []ExtensionInfo) (string, error) { return "", nil },
		ModuleRoot: "/fake",
	}

	err := l.Run(context.Background(), t.TempDir(), []string{"nonexistent_ext"}, nil, "", "loop", nil)
	require.Error(t, err, "expected error for missing extension")
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
	require.Error(t, err, "expected error for build failure")
}

func TestRun_CacheHit(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", "package noop")

	cacheDir := t.TempDir()
	c := NewCache(cacheDir)

	exts, err := Discover(projectDir, []string{"noop"})
	require.NoError(t, err)

	hash, err := ComputeHash(exts, "")
	require.NoError(t, err)

	fakeBin := filepath.Join(cacheDir, hash, "weave")
	require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, hash), 0o750))
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o750))

	binPath, found := c.Lookup(hash)
	require.True(t, found)
	assert.Equal(t, fakeBin, binPath)
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

	exts, err := Discover(projectDir, []string{"noop"})
	require.NoError(t, err, "Discover")

	hash, err := ComputeHash(exts, "")
	require.NoError(t, err, "ComputeHash")

	_, found := l.Cache.Lookup(hash)
	assert.False(t, found, "expected cache miss for new extension")

	binPath, err := l.buildAndCache(hash, "loop", nil, exts)
	require.NoError(t, err, "buildAndCache")
	require.NotEmpty(t, binPath)

	cached, found := l.Cache.Lookup(hash)
	require.True(t, found, "expected cache hit after buildAndCache")
	assert.Equal(t, binPath, cached)

	got, err := os.ReadFile(cached)
	require.NoError(t, err)
	assert.Equal(t, "fake-binary", string(got))
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
	require.NoError(t, err)

	hash, err := ComputeHash(exts, "")
	require.NoError(t, err)

	_, err = l.buildAndCache(hash, "loop", nil, exts)
	require.NoError(t, err)
	assert.Equal(t, 1, buildCount)

	_, found := l.Cache.Lookup(hash)
	require.True(t, found, "expected cache hit on second run")
	assert.Equal(t, 1, buildCount)
}

func TestBuildDir_CustomTmpDir(t *testing.T) {
	l := &Launcher{BuildTmpDir: "/custom/tmp"}
	assert.Equal(t, "/custom/tmp/abc123", l.buildDir("abc123"))
}

func TestBuildDir_DefaultTmpDir(t *testing.T) {
	l := &Launcher{}
	assert.Equal(t, filepath.Join(os.TempDir(), "weave-build-abc123"), l.buildDir("abc123"))
}

// fmtError wraps a string as an error for test use.
type fmtError string

func (e fmtError) Error() string { return string(e) }
