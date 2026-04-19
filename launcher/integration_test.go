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

func (p *noopProvider) Stream(_ context.Context, _ sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
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

func (p *noopProvider) Stream(_ context.Context, _ sdk.ProviderRequest) (<-chan sdk.ProviderEvent, error) {
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
	allCh := b.SubscribeAll()

	wired, err := sdk.Wire([]string{"noop"}, b, nil)
	require.NoError(t, err, "Wire")

	require.True(t, subscribeCalled, "Subscribe was not called")

	select {
	case evt := <-allCh:
		assert.Equal(t, "noop.subscribed", evt.Topic)
		assert.Equal(t, "hello", evt.Payload)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	require.NoError(t, wired.Close(), "Close")
	_ = b.Close()
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
