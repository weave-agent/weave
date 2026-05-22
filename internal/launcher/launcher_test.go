package launcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_BuildFails(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createExtension(t, extDir, "noop", "package noop")
	moduleRoot := createModuleRoot(t)

	buildErr := error(fmtError("mock build failure"))

	type contextKey string

	ctx := context.WithValue(context.Background(), contextKey("build"), "sentinel")

	var capturedContext bool

	l := &Launcher{
		Cache: NewCache(t.TempDir()),
		Build: func(gotCtx context.Context, _, _, _, _ string, _ bool, _ []ExtensionInfo) (string, error) {
			capturedContext = gotCtx.Value(contextKey("build")) == "sentinel"
			return "", buildErr
		},
		ModuleRoot:  moduleRoot,
		BuildTmpDir: t.TempDir(),
		HomeDir:     t.TempDir(),
	}

	err := l.Run(ctx, projectDir, nil, "", "loop", false, nil)
	require.Error(t, err, "expected error for build failure")
	require.ErrorIs(t, err, buildErr)
	assert.True(t, capturedContext, "launcher should pass Run context to Build")
}

func TestBuildAndCache_ReturnsWhenContextCanceledWhileWaitingForLock(t *testing.T) {
	hash := fmt.Sprintf("lock-cancel-%d", time.Now().UnixNano())

	unlock, err := lockBuildDir(context.Background(), hash)
	require.NoError(t, err)

	defer unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	l := &Launcher{
		Cache:       NewCache(t.TempDir()),
		BuildTmpDir: t.TempDir(),
		Build: func(context.Context, string, string, string, string, bool, []ExtensionInfo) (string, error) {
			t.Fatal("Build should not run while the build lock is held and context is canceled")

			return "", nil
		},
	}

	start := time.Now()
	_, err = l.buildAndCache(ctx, hash, "loop", false, nil, "")

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), time.Second)
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
		Build: func(_ context.Context, dir, _, _, _ string, _ bool, _ []ExtensionInfo) (string, error) {
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

	binPath, err := l.buildAndCache(context.Background(), hash, "loop", false, exts, "")
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
		Build: func(_ context.Context, dir, _, _, _ string, _ bool, _ []ExtensionInfo) (string, error) {
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

	_, err = l.buildAndCache(context.Background(), hash, "loop", false, exts, "")
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

func TestCoreDirsHashesEntireInternalTree(t *testing.T) {
	moduleRoot := createModuleRoot(t)
	l := &Launcher{ModuleRoot: moduleRoot}

	coreDirs := l.coreDirs()
	assert.Contains(t, coreDirs, filepath.Join(moduleRoot, "internal"))
	assert.Contains(t, coreDirs, filepath.Join(moduleRoot, "utils"))
	assert.NotContains(t, coreDirs, filepath.Join(moduleRoot, "internal", "launcher"))
	assert.NotContains(t, coreDirs, filepath.Join(moduleRoot, "internal", "auth"))
	assert.NotContains(t, coreDirs, filepath.Join(moduleRoot, "internal", "extmanage"))
	assert.NotContains(t, coreDirs, filepath.Join(moduleRoot, "utils", "truncate"))

	wireFile := filepath.Join(moduleRoot, "internal", "wire", "wire.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(wireFile), 0o750))
	require.NoError(t, os.WriteFile(wireFile, []byte("package wire\nconst Value = 1\n"), 0o600))

	h1, err := ComputeHash(nil, moduleRoot, "", false, "", coreDirs...)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(wireFile, []byte("package wire\nconst Value = 2\n"), 0o600))

	h2, err := ComputeHash(nil, moduleRoot, "", false, "", coreDirs...)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "changes anywhere under internal/ should affect the launcher hash")
}

func TestCoreDirsHashesUtilsTree(t *testing.T) {
	moduleRoot := createModuleRoot(t)
	l := &Launcher{ModuleRoot: moduleRoot}

	coreDirs := l.coreDirs()

	utilFile := filepath.Join(moduleRoot, "utils", "openaicompat", "sse.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(utilFile), 0o750))
	require.NoError(t, os.WriteFile(utilFile, []byte("package openaicompat\nconst Value = 1\n"), 0o600))

	h1, err := ComputeHash(nil, moduleRoot, "", false, "", coreDirs...)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(utilFile, []byte("package openaicompat\nconst Value = 2\n"), 0o600))

	h2, err := ComputeHash(nil, moduleRoot, "", false, "", coreDirs...)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "changes anywhere under utils/ should affect the launcher hash")
}

func TestDeriveBuildInputs_FiltersUIExtensionsInHeadless(t *testing.T) {
	exts := []ExtensionInfo{
		{Name: "agent"},
		{Name: "tui", IsUIExt: true},
	}

	headlessInputs := deriveBuildInputs(exts, true)
	require.Len(t, headlessInputs, 1)
	assert.Equal(t, "agent", headlessInputs[0].Name)

	interactiveInputs := deriveBuildInputs(exts, false)
	require.Len(t, interactiveInputs, 2)
	assert.Equal(t, exts, interactiveInputs)
}

func TestDeriveBuildInputs_HeadlessHashIgnoresUIOnlyChanges(t *testing.T) {
	root := t.TempDir()

	agentDir := filepath.Join(root, "agent")
	require.NoError(t, os.MkdirAll(agentDir, 0o750))
	agentFile := filepath.Join(agentDir, "agent.go")
	require.NoError(t, os.WriteFile(agentFile, []byte("package agent\nconst Version = 1\n"), 0o600))

	uiDir := filepath.Join(root, "tui")
	require.NoError(t, os.MkdirAll(uiDir, 0o750))
	uiFile := filepath.Join(uiDir, "tui.go")
	require.NoError(t, os.WriteFile(uiFile, []byte("package tui\nconst Version = 1\n"), 0o600))

	exts := []ExtensionInfo{
		{Name: "agent", Dir: agentDir, GoFiles: []string{agentFile}},
		{Name: "tui", Dir: uiDir, GoFiles: []string{uiFile}, IsUIExt: true},
	}

	headlessHash1, err := ComputeHash(deriveBuildInputs(exts, true), "", "", true, "loop")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(uiFile, []byte("package tui\nconst Version = 2\n"), 0o600))

	headlessHash2, err := ComputeHash(deriveBuildInputs(exts, true), "", "", true, "loop")
	require.NoError(t, err)
	assert.Equal(t, headlessHash1, headlessHash2, "headless hash should ignore UI-only extension changes")

	interactiveHash1, err := ComputeHash(deriveBuildInputs(exts, false), "", "", false, "loop")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(uiFile, []byte("package tui\nconst Version = 3\n"), 0o600))

	interactiveHash2, err := ComputeHash(deriveBuildInputs(exts, false), "", "", false, "loop")
	require.NoError(t, err)
	assert.NotEqual(t, interactiveHash1, interactiveHash2, "interactive hash should include UI extension changes")
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
		Build: func(_ context.Context, _, _, _, _ string, _ bool, exts []ExtensionInfo) (string, error) {
			capturedExts = exts

			return "", fmtError("captured")
		},
		ModuleRoot:  moduleRoot,
		BuildTmpDir: t.TempDir(),
		HomeDir:     t.TempDir(), // isolate from real ~/.weave/extensions
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
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "tui", "package tui\n\nfunc init() { RegisterUIExtension(\"tui\", nil) }")

	var (
		capturedHeadless bool
		capturedExts     []ExtensionInfo
	)

	l := &Launcher{
		Cache: NewCache(t.TempDir()),
		Build: func(_ context.Context, _, _, _, _ string, headless bool, exts []ExtensionInfo) (string, error) {
			capturedHeadless = headless
			capturedExts = exts

			return "", fmtError("captured")
		},
		ModuleRoot:  moduleRoot,
		BuildTmpDir: t.TempDir(),
		HomeDir:     t.TempDir(),
	}

	err := l.Run(context.Background(), projectDir, nil, "", "loop", true, nil)
	require.Error(t, err) // Build returns error to prevent exec
	assert.True(t, capturedHeadless)
	require.Len(t, capturedExts, 1)
	assert.Equal(t, "noop", capturedExts[0].Name)
}

func TestRun_NilExclude(t *testing.T) {
	projectDir := t.TempDir()
	moduleRoot := createModuleRoot(t)
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "alpha", "package alpha")
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "beta", "package beta")

	var capturedExts []ExtensionInfo

	l := &Launcher{
		Cache: NewCache(t.TempDir()),
		Build: func(_ context.Context, _, _, _, _ string, _ bool, exts []ExtensionInfo) (string, error) {
			capturedExts = exts

			return "", fmtError("captured")
		},
		ModuleRoot:  moduleRoot,
		BuildTmpDir: t.TempDir(),
		HomeDir:     t.TempDir(), // isolate from real ~/.weave/extensions
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
	for _, dir := range []string{"sdk", "bus", "settings", "utils/truncate", "utils/openaicompat", "internal/launcher", "internal/auth", "internal/extmanage"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o750))
	}

	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/weave-agent/weave\n\ngo 1.22\n"), 0o600))

	return root
}
