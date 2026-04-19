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
)

// noopCode is a minimal extension that registers itself and publishes an event on Subscribe.
const noopCode = `package noop

import "weave/sdk"

func init() {
	sdk.RegisterExtension("noop", func(cfg sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) {
			b.Publish(sdk.NewEvent("noop.ready", "noop extension active"))
		}), nil
	})
}
`

// noopMarkerCode is an extension that writes a marker file when Subscribe is called.
const noopMarkerCode = `package noop

import (
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

	// Step 1: Discover
	exts, err := Discover(projectDir, []string{"noop"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Step 2: Compute hash
	hash, err := ComputeHash(exts)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	// Step 3: Cache miss
	cacheDir := t.TempDir()

	cache := NewCache(cacheDir)
	if _, found := cache.Lookup(hash); found {
		t.Fatal("expected cache miss before build")
	}

	// Step 4: Build
	buildDir := t.TempDir()

	binPath, err := Build(buildDir, moduleRoot, exts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Step 5: Store in cache
	if err := cache.Store(hash, binPath); err != nil {
		t.Fatalf("Cache.Store: %v", err)
	}

	// Step 6: Cache hit
	cachedPath, found := cache.Lookup(hash)
	if !found {
		t.Fatal("expected cache hit after store")
	}

	// Step 7: Run the built binary — it blocks on signal, so use a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cachedPath)
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		t.Fatalf("start built binary: %v", err)
	}

	// Give it time to wire extensions, then kill
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
	if err != nil {
		t.Fatal(err)
	}

	hash, err := ComputeHash(exts)
	if err != nil {
		t.Fatal(err)
	}

	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)

	// First build + store
	buildDir := t.TempDir()

	binPath, err := Build(buildDir, moduleRoot, exts)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}

	if err := cache.Store(hash, binPath); err != nil {
		t.Fatalf("cache store: %v", err)
	}

	// Second lookup = cache hit
	cachedPath, found := cache.Lookup(hash)
	if !found {
		t.Fatal("expected cache hit on second lookup")
	}

	// Verify cached binary runs (blocks on signal, so use timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cachedPath)
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		t.Fatalf("start cached binary: %v", err)
	}

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
	if err != nil {
		t.Fatal(err)
	}

	// Build
	buildDir := t.TempDir()

	binPath, err := Build(buildDir, moduleRoot, exts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Run the binary with a marker env var.
	// The generated main creates a bus and calls sdk.Wire, which triggers Subscribe.
	markerFile := filepath.Join(t.TempDir(), "marker.txt")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath)

	cmd.Env = append(os.Environ(), "WEAVE_NOOP_MARKER="+markerFile)

	if startErr := cmd.Start(); startErr != nil {
		t.Fatalf("start binary: %v", startErr)
	}

	// Give Wire + Subscribe time to run
	time.Sleep(500 * time.Millisecond)

	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	// Subscribe was called → marker file should exist
	data, readErr := os.ReadFile(markerFile)
	if readErr != nil {
		t.Fatalf("marker file not found — Subscribe was not called: %v", readErr)
	}

	if string(data) != "subscribed" {
		t.Errorf("marker content = %q, want %q", string(data), "subscribed")
	}
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
	if err != nil {
		t.Fatalf("Wire: %v", err)
	}

	if !subscribeCalled {
		t.Fatal("Subscribe was not called")
	}

	select {
	case evt := <-allCh:
		if evt.Topic != "noop.subscribed" {
			t.Errorf("topic = %q, want %q", evt.Topic, "noop.subscribed")
		}

		if evt.Payload != "hello" {
			t.Errorf("payload = %v, want %q", evt.Payload, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	if err := wired.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_ = b.Close()
}

func TestIntegration_DiscoverCustomHome(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// No local extension, only global
	globalDir := filepath.Join(homeDir, ".weave", "extensions", "noop")
	createGoFile(t, globalDir, "noop.go", "package noop")

	exts, err := DiscoverCustomHome(projectDir, homeDir, []string{"noop"})
	if err != nil {
		t.Fatalf("DiscoverCustomHome: %v", err)
	}

	if exts[0].Dir != globalDir {
		t.Errorf("dir = %q, want %q", exts[0].Dir, globalDir)
	}

	// Local preferred over global
	localDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, localDir, "noop.go", "package noop")

	exts2, err := DiscoverCustomHome(projectDir, homeDir, []string{"noop"})
	if err != nil {
		t.Fatalf("DiscoverCustomHome: %v", err)
	}

	if exts2[0].Dir != localDir {
		t.Errorf("dir = %q, want %q", exts2[0].Dir, localDir)
	}
}
