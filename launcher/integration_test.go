package launcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopCode is a minimal extension that registers itself and publishes an event on Subscribe.
const noopCode = `package noop

import (
	"context"
	"weave/sdk"
	"weave/sdk/model"
)

func init() {
	sdk.RegisterExtension("noop", func(cfg sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) error {
			b.Publish(sdk.NewEvent("noop.ready", "noop extension active"))
			return nil
		}), nil
	})
	sdk.RegisterProvider("noop", func(cfg sdk.Config) (sdk.Provider, error) {
		return &noopProvider{}, nil
	})
}

type noopProvider struct{}

func (p *noopProvider) Stream(_ context.Context, _ sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
	ch := make(chan sdk.ProviderEvent)
	close(ch)
	return ch, nil
}
`

// noopMarkerCode is an extension that writes a marker file when Subscribe is called.
const noopMarkerCode = `package noop

import (
	"context"
	"os"
	"weave/sdk"
	"weave/sdk/model"
)

func init() {
	sdk.RegisterExtension("noop", func(cfg sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) error {
			marker := os.Getenv("WEAVE_NOOP_MARKER")
			if marker != "" {
				os.WriteFile(marker, []byte("subscribed"), 0o644)
			}
			return nil
		}), nil
	})
	sdk.RegisterProvider("noop", func(cfg sdk.Config) (sdk.Provider, error) {
		return &noopProvider{}, nil
	})
}

type noopProvider struct{}

func (p *noopProvider) Stream(_ context.Context, _ sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
	ch := make(chan sdk.ProviderEvent)
	close(ch)
	return ch, nil
}
`

func findModuleRootHelper(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Skipf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	t.Skip("cannot find module root")

	return ""
}

func setupTestExtension(t *testing.T, extDir, moduleRoot, code string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "noop.go"), []byte(code), 0o600))

	goMod := "module test/ext/noop\n\ngo 1.22\n\nrequire weave v0.0.0\n\nreplace weave => " + moduleRoot + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte(goMod), 0o600))
}

func TestIntegration_FullPipeline(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	setupTestExtension(t, extDir, moduleRoot, noopCode)

	exts, _, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	hash, err := ComputeHash(exts, "", false)
	require.NoError(t, err, "ComputeHash")

	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)
	_, found := cache.Lookup(hash)
	require.False(t, found, "expected cache miss before build")

	buildDir := t.TempDir()
	binPath, err := Build(buildDir, moduleRoot, "noop", false, exts)
	require.NoError(t, err, "Build")

	require.NoError(t, cache.Store(hash, binPath), "Cache.Store")

	cachedPath, found := cache.Lookup(hash)
	require.True(t, found, "expected cache hit after store")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cachedPath)
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Start(), "start built binary")

	// Allow extra time for startup — auto-discovery compiles in all extensions including TUI.
	time.Sleep(2 * time.Second)

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func TestIntegration_CacheHitOnSecondRun(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	setupTestExtension(t, extDir, moduleRoot, noopCode)

	exts, _, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)

	hash, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)

	buildDir := t.TempDir()
	binPath, err := Build(buildDir, moduleRoot, "noop", false, exts)
	require.NoError(t, err, "first build")

	require.NoError(t, cache.Store(hash, binPath), "cache store")

	cachedPath, found := cache.Lookup(hash)
	require.True(t, found, "expected cache hit on second lookup")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cachedPath)
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Start(), "start cached binary")

	time.Sleep(500 * time.Millisecond)

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func TestIntegration_ExtensionInitAndWireInBuiltBinary(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	setupTestExtension(t, extDir, moduleRoot, noopMarkerCode)

	exts, _, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)

	buildDir := t.TempDir()
	binPath, err := Build(buildDir, moduleRoot, "noop", false, exts)
	require.NoError(t, err, "Build")

	markerFile := filepath.Join(t.TempDir(), "marker.txt")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath)

	cmd.Env = append(os.Environ(), "WEAVE_NOOP_MARKER="+markerFile)

	require.NoError(t, cmd.Start(), "start binary")

	// Allow extra time for Subscribe to complete — with auto-discovery the TUI
	// extension is also compiled in, so startup takes longer.
	time.Sleep(2 * time.Second)

	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	data, err := os.ReadFile(markerFile)
	require.NoError(t, err, "marker file not found — Subscribe was not called")
	assert.Equal(t, "subscribed", string(data))
}

func TestIntegration_AutoDiscoverLocalOverGlobal(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	globalDir := filepath.Join(homeDir, ".weave", "extensions", "noop")
	createExtension(t, globalDir, "noop", "package noop")

	exts, _, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")
	require.Len(t, exts, 1)
	assert.Contains(t, exts[0].Dir, globalDir)

	localDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createExtension(t, localDir, "noop", "package noop")

	exts2, _, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")
	require.Len(t, exts2, 1)
	assert.Contains(t, exts2[0].Dir, localDir)
}

// TestIntegration_DiscoverBuiltinNestedTools verifies that the discovery finds
// real tool extensions under extensions/tools/* in the module root.
func TestIntegration_DiscoverBuiltinNestedTools(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	tools := []string{"bash", "read", "edit", "write", "grep", "find", "ls"}

	exts, _, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	extMap := make(map[string]ExtensionInfo, len(exts))
	for _, e := range exts {
		extMap[e.Name] = e
	}

	for _, name := range tools {
		info, ok := extMap[name]
		require.True(t, ok, "tool %q not discovered", name)
		assert.NotEmpty(t, info.GoFiles, "tool %q has no .go files", name)
		assert.Contains(t, info.Dir, filepath.Join("extensions", "tools", name),
			"tool %q should be found in nested tools/ directory", name)
	}
}

// TestIntegration_DiscoverBuiltinNestedProviders verifies that the discovery finds
// real provider extensions under extensions/providers/* in the module root.
func TestIntegration_DiscoverBuiltinNestedProviders(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	providers := []string{"anthropic", "openai", "zai"}

	exts, _, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	extMap := make(map[string]ExtensionInfo, len(exts))
	for _, e := range exts {
		extMap[e.Name] = e
	}

	for _, name := range providers {
		info, ok := extMap[name]
		require.True(t, ok, "provider %q not discovered", name)
		assert.NotEmpty(t, info.GoFiles, "provider %q has no .go files", name)
		assert.Contains(t, info.Dir, filepath.Join("extensions", "providers", name),
			"provider %q should be found in nested providers/ directory", name)
	}
}

// TestIntegration_DiscoverBuiltinNestedStore verifies that the jsonl store extension
// is discovered via nested lookup at extensions/store/jsonl/.
func TestIntegration_DiscoverBuiltinNestedStore(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	exts, _, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	var jsonlExt *ExtensionInfo

	for i := range exts {
		if exts[i].Name == "jsonl" {
			jsonlExt = &exts[i]
			break
		}
	}

	require.NotNil(t, jsonlExt, "jsonl should be discovered")
	assert.Contains(t, jsonlExt.Dir, filepath.Join("extensions", "store", "jsonl"),
		"jsonl store should be found in nested store/ directory")
	assert.NotEmpty(t, jsonlExt.GoFiles)
}

// TestIntegration_DiscoverBuiltinLoopDirect verifies the loop extension is still
// found at the direct path extensions/loop/.
func TestIntegration_DiscoverBuiltinLoopDirect(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	exts, _, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	var loopExt *ExtensionInfo

	for i := range exts {
		if exts[i].Name == "loop" {
			loopExt = &exts[i]
			break
		}
	}

	require.NotNil(t, loopExt, "loop should be discovered")
	assert.Equal(t, filepath.Join(moduleRoot, "extensions", "loop"), loopExt.Dir)
	assert.NotEmpty(t, loopExt.GoFiles)
}
