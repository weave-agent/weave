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
	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/model"
)

func init() {
	sdk.RegisterExtension[struct{}]("noop", func(cfg sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) error {
			b.Publish(sdk.NewEvent("noop.ready", "noop extension active"))
			return nil
		}), nil
	})
	sdk.RegisterProvider[struct{}, struct{}]("noop", func(cfg sdk.Config, _ struct{}, _ struct{}) (sdk.Provider, error) {
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
	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/model"
)

func init() {
	sdk.RegisterExtension[struct{}]("noop", func(cfg sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) error {
			marker := os.Getenv("WEAVE_NOOP_MARKER")
			if marker != "" {
				os.WriteFile(marker, []byte("subscribed"), 0o644)
			}
			return nil
		}), nil
	})
	sdk.RegisterProvider[struct{}, struct{}]("noop", func(cfg sdk.Config, _ struct{}, _ struct{}) (sdk.Provider, error) {
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

	goMod := "module test/ext/noop\n\ngo 1.22\n\nrequire github.com/weave-agent/weave v0.0.0\n\nreplace github.com/weave-agent/weave => " + moduleRoot + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte(goMod), 0o600))
}

func TestIntegration_FullPipeline(t *testing.T) {
	moduleRoot := findModuleRootHelper(t)

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	setupTestExtension(t, extDir, moduleRoot, noopCode)

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	hash, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err, "ComputeHash")

	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)
	_, found := cache.Lookup(hash)
	require.False(t, found, "expected cache miss before build")

	buildDir := t.TempDir()
	binPath, err := Build(buildDir, moduleRoot, "", "noop", false, exts)
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

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)

	hash, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)

	buildDir := t.TempDir()
	binPath, err := Build(buildDir, moduleRoot, "", "noop", false, exts)
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

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)

	buildDir := t.TempDir()
	binPath, err := Build(buildDir, moduleRoot, "", "noop", true, exts)
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

	globalDir := filepath.Join(homeDir, ".weave", "extensions")
	createExtension(t, globalDir, "noop", "package noop")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")
	require.Len(t, exts, 1)
	assert.Contains(t, exts[0].Dir, globalDir)

	localDir := filepath.Join(projectDir, ".weave", "extensions")
	createExtension(t, localDir, "noop", "package noop")

	exts2, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")
	require.Len(t, exts2, 1)
	assert.Contains(t, exts2[0].Dir, localDir)
}

// Built-in extension discovery tests removed — extensions are now in separate repos.
// Discovery of extensions from moduleRoot/extensions/ is still covered by
// discovery_test.go unit tests that create temporary extension directories.
