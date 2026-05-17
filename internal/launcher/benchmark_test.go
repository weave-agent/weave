package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// NOTE: Built-in extensions have been extracted to separate repos.
// Benchmark tests now use noop extensions created in temp dirs,
// matching the real-world pattern where extensions live in
// ~/.weave/extensions/ or .weave/extensions/, not in the main repo.

// setupNoopExtension creates a minimal noop extension in the given directory.
func setupNoopExtensionB(b *testing.B, extDir, moduleRoot string) {
	b.Helper()

	createGoFileB(b, extDir, "noop.go", noopCode)
	createGoFileB(b, extDir, "go.mod", "module test/ext/noop\n\ngo 1.22\n\nrequire github.com/weave-agent/weave v0.0.0\n\nreplace github.com/weave-agent/weave => "+moduleRoot+"\n")
}

// discoverNoopExtension discovers a single noop extension from a project-local dir.
func discoverNoopExtension(b *testing.B, projectDir string) []ExtensionInfo {
	b.Helper()

	homeDir := b.TempDir()

	exts, err := AutoDiscover(projectDir, homeDir, "", nil)
	if err != nil {
		b.Fatalf("AutoDiscover noop: %v", err)
	}

	if len(exts) == 0 {
		b.Fatal("no extensions discovered")
	}

	return exts
}

func buildExtensionsFromExts(b *testing.B, moduleRoot string, exts []ExtensionInfo) string {
	b.Helper()

	buildDir := b.TempDir()

	binPath, err := Build(buildDir, moduleRoot, "noop", false, exts)
	if err != nil {
		b.Fatalf("Build: %v", err)
	}

	return binPath
}

func reportBinarySize(b *testing.B, binPath string) {
	b.Helper()

	info, err := os.Stat(binPath)
	if err != nil {
		b.Fatalf("stat binary: %v", err)
	}

	b.ReportMetric(float64(info.Size()), "bytes")
}

func withGoCache(b *testing.B, cacheDir string, fn func()) {
	b.Helper()

	orig := os.Getenv("GOCACHE")

	if err := os.Setenv("GOCACHE", cacheDir); err != nil {
		b.Fatalf("set GOCACHE: %v", err)
	}

	defer func() {
		if orig == "" {
			_ = os.Unsetenv("GOCACHE")
		} else {
			_ = os.Setenv("GOCACHE", orig)
		}
	}()

	fn()
}

func createGoFileB(b *testing.B, dir, name, content string) {
	b.Helper()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		b.Fatalf("mkdir %s: %v", dir, err)
	}

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		b.Fatalf("write %s: %v", filepath.Join(dir, name), err)
	}
}

func findModuleRootB(b *testing.B) string {
	b.Helper()

	dir, err := os.Getwd()
	if err != nil {
		b.Skipf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	b.Skip("cannot find module root")

	return ""
}

// Cold builds: empty Go build cache. Full compilation from scratch.

func BenchmarkColdBuild_NoopExtension(b *testing.B) {
	moduleRoot := findModuleRootB(b)

	for b.Loop() {
		withGoCache(b, b.TempDir(), func() {
			projectDir := b.TempDir()
			extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
			setupNoopExtensionB(b, extDir, moduleRoot)
			exts := discoverNoopExtension(b, projectDir)
			binPath := buildExtensionsFromExts(b, moduleRoot, exts)
			reportBinarySize(b, binPath)
		})
	}
}

// warmPipeline runs the full launcher build pipeline (discover -> hash -> cache miss ->
// build -> cache store) using a fresh cache each iteration. This matches the real
// `go run` path: hash always changes (fresh cache), Go build cache is whatever the
// system has.
func warmPipelineNoop(b *testing.B, moduleRoot string) {
	b.Helper()

	for b.Loop() {
		cacheDir := b.TempDir()
		cache := NewCache(cacheDir)

		projectDir := b.TempDir()
		extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
		setupNoopExtensionB(b, extDir, moduleRoot)

		exts := discoverNoopExtension(b, projectDir)

		hash, err := ComputeHash(exts, "", false, "")
		if err != nil {
			b.Fatalf("ComputeHash: %v", err)
		}

		// Cache miss (fresh cache) -> buildAndCache
		buildDir := b.TempDir()

		binPath, buildErr := Build(buildDir, moduleRoot, "noop", false, exts)
		if buildErr != nil {
			b.Fatalf("Build: %v", buildErr)
		}

		if storeErr := cache.Store(hash, binPath); storeErr != nil {
			b.Fatalf("Cache.Store: %v", storeErr)
		}

		cached, found := cache.Lookup(hash)
		if !found {
			b.Fatal("cache miss after store")
		}

		reportBinarySize(b, cached)
	}
}

// Warm builds: full launcher pipeline (discover -> hash -> build -> cache store).
// System Go build cache reflects real usage. Fresh launcher cache each iteration
// simulates a hash change from extension modification.

func BenchmarkWarmBuild_NoopExtension(b *testing.B) {
	moduleRoot := findModuleRootB(b)
	warmPipelineNoop(b, moduleRoot)
}

// End-to-end: measures the full `go run ./cmd/weave/ -p "hello"` path.
// Includes go run compilation + launcher pipeline (discover -> hash -> build -> cache).
// Uses a project-local .weave/settings.json to control extensions.
// This is what you actually experience at the terminal.

func goRunEndToEnd(b *testing.B, extYAML string) {
	b.Helper()

	moduleRoot := findModuleRootB(b)

	cacheDir, err := DefaultCacheDir()
	if err != nil {
		b.Fatalf("DefaultCacheDir: %v", err)
	}

	// Create a project-local config to control which extensions are built.
	configDir := filepath.Join(moduleRoot, ".weave")

	if err := os.MkdirAll(configDir, 0o750); err != nil {
		b.Fatalf("mkdir .weave: %v", err)
	}

	configPath := filepath.Join(configDir, "settings.json")

	if err := os.WriteFile(configPath, []byte(extYAML), 0o600); err != nil {
		b.Fatalf("write config: %v", err)
	}

	defer func() {
		_ = os.Remove(configPath)
		_ = os.Remove(configDir)
	}()

	for b.Loop() {
		// Clear launcher cache to force a full build each iteration.
		_ = os.RemoveAll(cacheDir)

		cmd := exec.Command("go", "run", "./cmd/weave/", "-p", "hello")
		cmd.Dir = moduleRoot

		output, err := cmd.CombinedOutput()
		if err == nil {
			continue
		}

		// Accept exit code 1 (provider error, stdin error) but not other failures.
		if cmd.ProcessState.ExitCode() != 1 {
			b.Fatalf("go run failed: %v\n%s", err, output)
		}
	}
}

func BenchmarkGoRun_NoTUI(b *testing.B) {
	goRunEndToEnd(b, `{"core":{"agent_loop":"loop"},"ui":"none"}`)
}

func BenchmarkGoRun_TUI(b *testing.B) {
	goRunEndToEnd(b, `{"core":{"agent_loop":"loop"},"ui":"tui"}`)
}

// Partial builds: Go cache primed with full build, but one extension source changed.
// Only the changed extension recompiles — everything else is cached.

func BenchmarkPartialBuild_OneExtRebuild(b *testing.B) {
	moduleRoot := findModuleRootB(b)

	projectDir := b.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	setupNoopExtensionB(b, extDir, moduleRoot)

	// Prime: build noop to populate Go cache.
	primeExts := discoverNoopExtension(b, projectDir)
	buildExtensionsFromExts(b, moduleRoot, primeExts)

	var iter int

	for b.Loop() {
		// Modify noop source to force recompilation of only that package.
		iter++

		versionCode := fmt.Sprintf("package noop\nvar _ = %d\n", iter)

		if err := os.WriteFile(filepath.Join(extDir, "version.go"), []byte(versionCode), 0o644); err != nil {
			b.Fatalf("write version.go: %v", err)
		}

		loopExts := discoverNoopExtension(b, projectDir)
		binPath := buildExtensionsFromExts(b, moduleRoot, loopExts)
		reportBinarySize(b, binPath)
	}
}

func BenchmarkPartialBuild_OneExtRebuild_Cold(b *testing.B) {
	moduleRoot := findModuleRootB(b)

	for b.Loop() {
		withGoCache(b, b.TempDir(), func() {
			projectDir := b.TempDir()
			extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
			setupNoopExtensionB(b, extDir, moduleRoot)

			// Prime temp cache with noop build.
			baseExts := discoverNoopExtension(b, projectDir)
			buildExtensionsFromExts(b, moduleRoot, baseExts)

			// Modify noop source so it's the only uncached package.
			versionCode := "package noop\nvar _ = 1\n"

			if writeErr := os.WriteFile(filepath.Join(extDir, "version.go"), []byte(versionCode), 0o644); writeErr != nil {
				b.Fatalf("write version.go: %v", writeErr)
			}

			noopExts := discoverNoopExtension(b, projectDir)
			binPath := buildExtensionsFromExts(b, moduleRoot, noopExts)
			reportBinarySize(b, binPath)
		})
	}
}
