package launcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
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

func setupNamedNoopExtensionB(b *testing.B, extDir, moduleRoot, name string) {
	b.Helper()

	code := fmt.Sprintf(`package %s

import "github.com/weave-agent/weave/sdk"

func init() {
	sdk.RegisterExtension[struct{}](%q, func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc(%q, func(_ sdk.Bus) error {
			return nil
		}), nil
	})
}
`, name, name, name)

	createGoFileB(b, extDir, name+".go", code)
	createGoFileB(b, extDir, "go.mod", "module test/ext/"+name+"\n\ngo 1.22\n\nrequire github.com/weave-agent/weave v0.0.0\n\nreplace github.com/weave-agent/weave => "+moduleRoot+"\n")
}

func setupBenchmarkExtensionsB(b *testing.B, projectDir, moduleRoot string, count int) {
	b.Helper()

	for i := range count {
		name := fmt.Sprintf("noop%03d", i)
		extDir := filepath.Join(projectDir, ".weave", "extensions", name)

		setupNamedNoopExtensionB(b, extDir, moduleRoot, name)
	}
}

// discoverNoopExtension discovers a single noop extension from a project-local dir.
func discoverNoopExtension(b *testing.B, projectDir string) []ExtensionInfo {
	b.Helper()

	homeDir := b.TempDir()

	exts := discoverExtensionsB(b, projectDir, homeDir, "")

	if len(exts) == 0 {
		b.Fatal("no extensions discovered")
	}

	return exts
}

func discoverExtensionsB(b *testing.B, projectDir, homeDir, moduleRoot string) []ExtensionInfo {
	b.Helper()

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	if err != nil {
		b.Fatalf("AutoDiscover: %v", err)
	}

	return exts
}

func buildExtensionsFromExts(b *testing.B, moduleRoot string, exts []ExtensionInfo) string {
	b.Helper()

	buildDir := b.TempDir()

	binPath, err := Build(context.Background(), buildDir, moduleRoot, "", "noop", false, exts)
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

func seedCacheEntryB(b *testing.B, cache *Cache, hash string) {
	b.Helper()

	src := filepath.Join(b.TempDir(), "weave")
	if err := os.WriteFile(src, []byte("cached launcher binary"), 0o750); err != nil {
		b.Fatalf("write cached binary: %v", err)
	}

	if err := cache.Store(hash, src); err != nil {
		b.Fatalf("Cache.Store seed: %v", err)
	}
}

func computeLauncherHashB(b *testing.B, exts []ExtensionInfo, moduleRoot string, coreDirs []string) string {
	b.Helper()

	hash, err := ComputeHash(deriveBuildInputs(exts, false), moduleRoot, "", false, "", coreDirs...)
	if err != nil {
		b.Fatalf("ComputeHash: %v", err)
	}

	return hash
}

func reportDurationMetric(b *testing.B, total time.Duration, count int, unit string) {
	b.Helper()

	if count == 0 {
		return
	}

	b.ReportMetric(float64(total.Nanoseconds())/float64(count), unit)
}

func runGoCommandB(b *testing.B, dir string, args ...string) {
	b.Helper()

	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Dir = dir

	if output, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func generateBuildFilesB(b *testing.B, buildDir, moduleRoot, moduleVersion, agentLoop string, headless bool, exts []ExtensionInfo) {
	b.Helper()

	sorted := make([]ExtensionInfo, len(exts))
	copy(sorted, exts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	if headless {
		filtered := make([]ExtensionInfo, 0, len(sorted))
		for _, ext := range sorted {
			if !ext.IsUIExt {
				filtered = append(filtered, ext)
			}
		}

		sorted = filtered
	}

	for _, ext := range sorted {
		if err := ensureExtGoMod(ext, moduleRoot, moduleVersion); err != nil {
			b.Fatalf("ensure extension go.mod for %s: %v", ext.Name, err)
		}
	}

	for i := range sorted {
		sorted[i].ModulePath = readModulePath(sorted[i].Dir)
	}

	if err := GenerateGoMod(buildDir, moduleRoot, moduleVersion, sorted); err != nil {
		b.Fatalf("GenerateGoMod: %v", err)
	}

	if err := GenerateMainGo(buildDir, sorted, agentLoop); err != nil {
		b.Fatalf("GenerateMainGo: %v", err)
	}
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

func benchmarkLauncherCacheHit(b *testing.B, extensionCount int) {
	moduleRoot := findModuleRootB(b)
	projectDir := b.TempDir()
	homeDir := b.TempDir()

	setupBenchmarkExtensionsB(b, projectDir, moduleRoot, extensionCount)

	cache := NewCache(b.TempDir())
	cache.MaxSizeBytes = -1

	coreDirs := NewLauncher(cache, moduleRoot, "").coreDirs()
	initialExts := discoverExtensionsB(b, projectDir, homeDir, moduleRoot)
	initialHash := computeLauncherHashB(b, initialExts, moduleRoot, coreDirs)
	seedCacheEntryB(b, cache, initialHash)

	var (
		discoveryTotal time.Duration
		hashTotal      time.Duration
		cacheTotal     time.Duration
		count          int
	)

	b.ReportMetric(float64(extensionCount), "extensions")
	b.ResetTimer()

	for b.Loop() {
		count++

		start := time.Now()
		exts := discoverExtensionsB(b, projectDir, homeDir, moduleRoot)
		discoveryTotal += time.Since(start)

		start = time.Now()
		hash := computeLauncherHashB(b, exts, moduleRoot, coreDirs)
		hashTotal += time.Since(start)

		start = time.Now()
		if _, found := cache.Lookup(hash); !found {
			b.Fatal("expected launcher cache hit")
		}

		cacheTotal += time.Since(start)
	}

	reportDurationMetric(b, discoveryTotal, count, "discovery_ns/op")
	reportDurationMetric(b, hashTotal, count, "hash_ns/op")
	reportDurationMetric(b, cacheTotal, count, "cache_lookup_ns/op")
}

func BenchmarkLauncherCacheHit_NoExtensions(b *testing.B) {
	benchmarkLauncherCacheHit(b, 0)
}

func BenchmarkLauncherCacheHit_OneExtension(b *testing.B) {
	benchmarkLauncherCacheHit(b, 1)
}

func BenchmarkLauncherCacheHit_ManyExtensions(b *testing.B) {
	benchmarkLauncherCacheHit(b, 20)
}

func BenchmarkLauncherBuildPhases_OneExtension(b *testing.B) {
	moduleRoot := findModuleRootB(b)
	projectDir := b.TempDir()
	homeDir := b.TempDir()
	coreDirs := NewLauncher(nil, moduleRoot, "").coreDirs()

	setupBenchmarkExtensionsB(b, projectDir, moduleRoot, 1)

	var (
		discoveryTotal      time.Duration
		hashTotal           time.Duration
		generatedFilesTotal time.Duration
		tidyTotal           time.Duration
		buildTotal          time.Duration
		cacheStoreTotal     time.Duration
		count               int
	)

	b.ResetTimer()

	for b.Loop() {
		count++

		cache := NewCache(b.TempDir())
		cache.MaxSizeBytes = -1

		start := time.Now()
		exts := discoverExtensionsB(b, projectDir, homeDir, moduleRoot)
		discoveryTotal += time.Since(start)

		start = time.Now()
		hash := computeLauncherHashB(b, exts, moduleRoot, coreDirs)
		hashTotal += time.Since(start)

		buildDir := b.TempDir()

		start = time.Now()
		generateBuildFilesB(b, buildDir, moduleRoot, "", "", false, deriveBuildInputs(exts, false))
		generatedFilesTotal += time.Since(start)

		start = time.Now()
		runGoCommandB(b, buildDir, "mod", "tidy")
		tidyTotal += time.Since(start)

		binaryPath := filepath.Join(buildDir, "weave")

		start = time.Now()
		runGoCommandB(b, buildDir, "build", "-o", binaryPath, ".")
		buildTotal += time.Since(start)

		start = time.Now()
		if err := cache.Store(hash, binaryPath); err != nil {
			b.Fatalf("Cache.Store: %v", err)
		}

		cacheStoreTotal += time.Since(start)
		reportBinarySize(b, binaryPath)
	}

	reportDurationMetric(b, discoveryTotal, count, "discovery_ns/op")
	reportDurationMetric(b, hashTotal, count, "hash_ns/op")
	reportDurationMetric(b, generatedFilesTotal, count, "generated_files_ns/op")
	reportDurationMetric(b, tidyTotal, count, "go_mod_tidy_ns/op")
	reportDurationMetric(b, buildTotal, count, "go_build_ns/op")
	reportDurationMetric(b, cacheStoreTotal, count, "cache_store_ns/op")
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

		hash, err := ComputeHash(exts, "", "", false, "")
		if err != nil {
			b.Fatalf("ComputeHash: %v", err)
		}

		// Cache miss (fresh cache) -> buildAndCache
		buildDir := b.TempDir()

		binPath, buildErr := Build(context.Background(), buildDir, moduleRoot, "", "noop", false, exts)
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
// Uses temp HOME, project config, and launcher cache directories so it does not
// mutate the repository .weave directory or the user's ~/.weave/bin cache.
// This is what you actually experience at the terminal.

func goRunEndToEnd(b *testing.B, settingsJSON string) {
	b.Helper()

	moduleRoot := findModuleRootB(b)

	// Create a project-local config to control which extensions are built.
	projectDir := b.TempDir()
	configDir := filepath.Join(projectDir, ".weave")

	if err := os.MkdirAll(configDir, 0o750); err != nil {
		b.Fatalf("mkdir .weave: %v", err)
	}

	configPath := filepath.Join(configDir, "settings.json")

	if err := os.WriteFile(configPath, []byte(settingsJSON), 0o600); err != nil {
		b.Fatalf("write config: %v", err)
	}

	homeDir := b.TempDir()
	cacheDir := filepath.Join(homeDir, ".weave", "bin")
	env := benchmarkEnvWithHomeB(b, homeDir)

	b.ResetTimer()

	for b.Loop() {
		// Clear launcher cache to force a full build each iteration.
		_ = os.RemoveAll(cacheDir)

		cmd := exec.Command("go", "run", "./cmd/weave/", "--config", configPath, "--skip-bootstrap", "-p", "hello")
		cmd.Dir = moduleRoot
		cmd.Env = env

		output, err := cmd.CombinedOutput()
		if err == nil {
			continue
		}

		// Accept exit code 1 (provider error, stdin error) but not other failures.
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}

		if exitCode != 1 {
			b.Fatalf("go run failed: %v\n%s", err, output)
		}
	}
}

func benchmarkEnvWithHomeB(b *testing.B, homeDir string) []string {
	b.Helper()

	env := upsertEnv(os.Environ(), "HOME", homeDir)

	for _, key := range []string{"GOCACHE", "GOMODCACHE"} {
		if os.Getenv(key) != "" {
			continue
		}

		value := goEnvValueB(b, key)
		if value != "" {
			env = upsertEnv(env, key, value)
		}
	}

	return env
}

func goEnvValueB(b *testing.B, key string) string {
	b.Helper()

	cmd := exec.Command("go", "env", key)
	output, err := cmd.Output()
	if err != nil {
		b.Fatalf("go env %s: %v", key, err)
	}

	return strings.TrimSpace(string(output))
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value

			return env
		}
	}

	return append(env, prefix+value)
}

func BenchmarkGoRun_NoTUI(b *testing.B) {
	goRunEndToEnd(b, `{"agent_loop":"loop","ui_extension":"none"}`)
}

func BenchmarkGoRun_TUI(b *testing.B) {
	goRunEndToEnd(b, `{"agent_loop":"loop","ui_extension":"tui"}`)
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
