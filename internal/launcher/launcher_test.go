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

func TestRun_BuildFails(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createExtension(t, extDir, "noop", "package noop")

	buildErr := error(fmtError("mock build failure"))
	l := &Launcher{
		Cache: NewCache(t.TempDir()),
		Build: func(string, string, string, string, bool, []ExtensionInfo) (string, error) {
			return "", buildErr
		},
		ModuleRoot:  "/fake",
		BuildTmpDir: t.TempDir(),
	}

	err := l.Run(context.Background(), projectDir, nil, "", "loop", false, nil)
	require.Error(t, err, "expected error for build failure")
}

func TestRun_CacheHit(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createExtension(t, extDir, "noop", "package noop")

	cacheDir := t.TempDir()
	c := NewCache(cacheDir)

	exts, err := AutoDiscover(projectDir, t.TempDir(), "", nil)
	require.NoError(t, err)

	hash, err := ComputeHash(exts, "", "", false, "")
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
	createExtension(t, extDir, "noop", "package noop")

	cacheDir := t.TempDir()
	buildDir := t.TempDir()

	l := &Launcher{
		Cache: NewCache(cacheDir),
		Build: func(dir, _, _, _ string, _ bool, _ []ExtensionInfo) (string, error) {
			binPath := filepath.Join(dir, "weave")
			if err := os.WriteFile(binPath, []byte("fake-binary"), 0o750); err != nil {
				return "", fmt.Errorf("write fake binary: %w", err)
			}

			return binPath, nil
		},
		ModuleRoot:  "/fake",
		BuildTmpDir: buildDir,
	}

	exts, err := AutoDiscover(projectDir, t.TempDir(), "", nil)
	require.NoError(t, err, "AutoDiscover")

	hash, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err, "ComputeHash")

	_, found := l.Cache.Lookup(hash)
	assert.False(t, found, "expected cache miss for new extension")

	binPath, err := l.buildAndCache(hash, "loop", false, exts, "")
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
	createExtension(t, extDir, "noop", "package noop")

	cacheDir := t.TempDir()
	buildDir := t.TempDir()

	buildCount := 0
	l := &Launcher{
		Cache: NewCache(cacheDir),
		Build: func(dir, _, _, _ string, _ bool, _ []ExtensionInfo) (string, error) {
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

	exts, err := AutoDiscover(projectDir, t.TempDir(), "", nil)
	require.NoError(t, err)

	hash, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	_, err = l.buildAndCache(hash, "loop", false, exts, "")
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

func TestBuildExecEnv_PrependsWeaveVars(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"WEAVE_MODULE_ROOT=/stale",
		"HOME=/home/user",
	}

	env := buildExecEnv(parent, "/launcher", "abc123", "[\"weave\"]", "/correct", "")

	// Our vars should come first so they override stale parent values.
	require.Len(t, env, 7)
	assert.Equal(t, "WEAVE_LAUNCHER_PATH=/launcher", env[0])
	assert.Equal(t, "WEAVE_BUILD_HASH=abc123", env[1])
	assert.Equal(t, "WEAVE_ORIG_ARGS=[\"weave\"]", env[2])
	assert.Equal(t, "WEAVE_MODULE_ROOT=/correct", env[3])
	assert.Equal(t, "PATH=/usr/bin", env[4])
	assert.Equal(t, "WEAVE_MODULE_ROOT=/stale", env[5])
	assert.Equal(t, "HOME=/home/user", env[6])
}

// fmtError wraps a string as an error for test use.
type fmtError string

func (e fmtError) Error() string { return string(e) }

func TestRun_ExcludeExtensions(t *testing.T) {
	projectDir := t.TempDir()
	moduleRoot := createModuleRoot(t)

	// Create two extensions
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "alpha", "package alpha")
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "beta", "package beta")

	var capturedExts []ExtensionInfo

	l := &Launcher{
		Cache: NewCache(t.TempDir()),
		Build: func(_, _, _, _ string, _ bool, exts []ExtensionInfo) (string, error) {
			capturedExts = exts

			return "", fmtError("captured")
		},
		ModuleRoot:  moduleRoot,
		BuildTmpDir: t.TempDir(),
	}

	err := l.Run(context.Background(), projectDir, nil, "", "loop", false, []string{"alpha"})
	require.Error(t, err) // Build returns error to prevent exec

	require.Len(t, capturedExts, 1)
	assert.Equal(t, "beta", capturedExts[0].Name)
}

func TestRun_HeadlessPassedToBuild(t *testing.T) {
	projectDir := t.TempDir()
	moduleRoot := createModuleRoot(t)
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "noop", "package noop")

	var capturedHeadless bool

	l := &Launcher{
		Cache: NewCache(t.TempDir()),
		Build: func(_, _, _, _ string, headless bool, _ []ExtensionInfo) (string, error) {
			capturedHeadless = headless

			return "", fmtError("captured")
		},
		ModuleRoot:  moduleRoot,
		BuildTmpDir: t.TempDir(),
	}

	err := l.Run(context.Background(), projectDir, nil, "", "loop", true, nil)
	require.Error(t, err) // Build returns error to prevent exec
	assert.True(t, capturedHeadless)
}

func TestRun_NilExclude(t *testing.T) {
	projectDir := t.TempDir()
	moduleRoot := createModuleRoot(t)
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "alpha", "package alpha")
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "beta", "package beta")

	var capturedExts []ExtensionInfo

	l := &Launcher{
		Cache: NewCache(t.TempDir()),
		Build: func(_, _, _, _ string, _ bool, exts []ExtensionInfo) (string, error) {
			capturedExts = exts

			return "", fmtError("captured")
		},
		ModuleRoot:  moduleRoot,
		BuildTmpDir: t.TempDir(),
	}

	err := l.Run(context.Background(), projectDir, nil, "", "loop", false, nil)
	require.Error(t, err)

	require.Len(t, capturedExts, 2)
	assert.Equal(t, "alpha", capturedExts[0].Name)
	assert.Equal(t, "beta", capturedExts[1].Name)
}

// createModuleRoot creates a temporary directory mimicking the weave module root
// with empty core dirs so ComputeHash can walk them without error.
func createModuleRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, dir := range []string{"sdk", "bus", "settings", "utils/truncate", "internal/launcher", "internal/auth", "internal/extmanage"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o750))
	}

	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/weave-agent/weave\n\ngo 1.22\n"), 0o600))

	return root
}
