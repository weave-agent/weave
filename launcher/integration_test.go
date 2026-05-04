package launcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"weave/bus"
	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopCode is a minimal extension that registers itself and publishes an event on Subscribe.
const noopCode = `package noop

import (
	"context"
	"weave/sdk"
)

func init() {
	sdk.RegisterExtension("noop", func(cfg sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) {
			b.Publish(sdk.NewEvent("noop.ready", "noop extension active"))
		}), nil
	})
	sdk.RegisterProvider("noop", func(cfg sdk.Config) (sdk.Provider, error) {
		return &noopProvider{}, nil
	})
}

type noopProvider struct{}

func (p *noopProvider) Stream(_ context.Context, _ sdk.ProviderRequest, _ ...sdk.StreamOption) (<-chan sdk.ProviderEvent, error) {
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
)

func init() {
	sdk.RegisterExtension("noop", func(cfg sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) {
			marker := os.Getenv("WEAVE_NOOP_MARKER")
			if marker != "" {
				os.WriteFile(marker, []byte("subscribed"), 0o644)
			}
		}), nil
	})
	sdk.RegisterProvider("noop", func(cfg sdk.Config) (sdk.Provider, error) {
		return &noopProvider{}, nil
	})
}

type noopProvider struct{}

func (p *noopProvider) Stream(_ context.Context, _ sdk.ProviderRequest, _ ...sdk.StreamOption) (<-chan sdk.ProviderEvent, error) {
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

func TestIntegration_FullPipeline(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)

	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", noopCode)

	exts, err := Discover(projectDir, []string{"noop"})
	require.NoError(t, err, "Discover")

	hash, err := ComputeHash(exts, "")
	require.NoError(t, err, "ComputeHash")

	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)
	_, found := cache.Lookup(hash)
	require.False(t, found, "expected cache miss before build")

	buildDir := t.TempDir()
	binPath, err := Build(buildDir, moduleRoot, "noop", []string{"noop"}, exts)
	require.NoError(t, err, "Build")

	require.NoError(t, cache.Store(hash, binPath), "Cache.Store")

	cachedPath, found := cache.Lookup(hash)
	require.True(t, found, "expected cache hit after store")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cachedPath)
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Start(), "start built binary")

	time.Sleep(500 * time.Millisecond)

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func TestIntegration_CacheHitOnSecondRun(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)

	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", noopCode)

	exts, err := Discover(projectDir, []string{"noop"})
	require.NoError(t, err)

	hash, err := ComputeHash(exts, "")
	require.NoError(t, err)

	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)

	buildDir := t.TempDir()
	binPath, err := Build(buildDir, moduleRoot, "noop", []string{"noop"}, exts)
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
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", noopMarkerCode)

	exts, err := Discover(projectDir, []string{"noop"})
	require.NoError(t, err)

	buildDir := t.TempDir()
	binPath, err := Build(buildDir, moduleRoot, "noop", []string{"noop"}, exts)
	require.NoError(t, err, "Build")

	markerFile := filepath.Join(t.TempDir(), "marker.txt")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath)

	cmd.Env = append(os.Environ(), "WEAVE_NOOP_MARKER="+markerFile)

	require.NoError(t, cmd.Start(), "start binary")

	time.Sleep(500 * time.Millisecond)

	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	data, err := os.ReadFile(markerFile)
	require.NoError(t, err, "marker file not found — Subscribe was not called")
	assert.Equal(t, "subscribed", string(data))
}

func TestIntegration_WireSubscribesExtensionsInProcess(t *testing.T) {
	sdk.ResetRegistry()

	var subscribeCalled bool

	sdk.RegisterExtension("noop", func(cfg sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) {
			subscribeCalled = true

			b.Publish(sdk.NewEvent("noop.subscribed", "hello"))
		}), nil
	})

	b := bus.New()

	var received sdk.Event
	b.OnAll(func(e sdk.Event) error {
		received = e
		return nil
	})

	wired, err := sdk.Wire([]string{"noop"}, b, nil)
	require.NoError(t, err, "Wire")

	require.True(t, subscribeCalled, "Subscribe was not called")

	require.NoError(t, wired.Close(), "Close")
	_ = b.Close()

	assert.Equal(t, "noop.subscribed", received.Topic)
	assert.Equal(t, "hello", received.Payload)
}

func TestIntegration_DiscoverCustomHome(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	globalDir := filepath.Join(homeDir, ".weave", "extensions", "noop")
	createGoFile(t, globalDir, "noop.go", "package noop")

	exts, err := DiscoverCustomHome(projectDir, homeDir, []string{"noop"})
	require.NoError(t, err, "DiscoverCustomHome")
	assert.Equal(t, globalDir, exts[0].Dir)

	localDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, localDir, "noop.go", "package noop")

	exts2, err := DiscoverCustomHome(projectDir, homeDir, []string{"noop"})
	require.NoError(t, err, "DiscoverCustomHome")
	assert.Equal(t, localDir, exts2[0].Dir)
}

// TestIntegration_DiscoverBuiltinNestedTools verifies that the discovery finds
// real tool extensions under extensions/tools/* in the module root.
func TestIntegration_DiscoverBuiltinNestedTools(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	tools := []string{"bash", "read", "edit", "write", "grep", "find", "ls"}

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, tools)
	require.NoError(t, err, "DiscoverCustomHomeWithBuiltins for tools")

	require.Len(t, exts, len(tools))

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

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, providers)
	require.NoError(t, err, "DiscoverCustomHomeWithBuiltins for providers")

	require.Len(t, exts, len(providers))

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

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"jsonl"})
	require.NoError(t, err, "DiscoverCustomHomeWithBuiltins for jsonl store")

	require.Len(t, exts, 1)
	assert.Equal(t, "jsonl", exts[0].Name)
	assert.Contains(t, exts[0].Dir, filepath.Join("extensions", "store", "jsonl"),
		"jsonl store should be found in nested store/ directory")
	assert.NotEmpty(t, exts[0].GoFiles)
}

// TestIntegration_DiscoverBuiltinLoopDirect verifies the loop extension is still
// found at the direct path extensions/loop/.
func TestIntegration_DiscoverBuiltinLoopDirect(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"loop"})
	require.NoError(t, err, "DiscoverCustomHomeWithBuiltins for loop")

	require.Len(t, exts, 1)
	assert.Equal(t, "loop", exts[0].Name)
	assert.Equal(t, filepath.Join(moduleRoot, "extensions", "loop"), exts[0].Dir)
	assert.NotEmpty(t, exts[0].GoFiles)
}
