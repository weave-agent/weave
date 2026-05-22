package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// BuildFunc builds a binary from extension infos and returns its path.
type BuildFunc func(ctx context.Context, dir, moduleRoot, moduleVersion, agentLoop string, headless bool, exts []ExtensionInfo) (string, error)

// Launcher orchestrates the full pipeline: discover -> hash -> cache -> build -> exec.
type Launcher struct {
	Cache         *Cache
	Build         BuildFunc
	ModuleRoot    string
	ModuleVersion string // semver tag for release mode (e.g. "v0.0.1"); empty in dev mode
	BuildTmpDir   string
	HomeDir       string // overrides os.UserHomeDir() when set (for testing)
}

// NewLauncher creates a Launcher with the default Build function.
// moduleVersion is the semver tag extracted from the binary revision; it is used
// in release mode (moduleRoot == "") to generate require directives against the
// Go module proxy. Pass empty string in dev mode.
func NewLauncher(cache *Cache, moduleRoot, moduleVersion string) *Launcher {
	return &Launcher{
		Cache:         cache,
		Build:         Build,
		ModuleRoot:    moduleRoot,
		ModuleVersion: moduleVersion,
	}
}

// Run executes the full launcher pipeline:
//  1. Auto-discover extension source directories
//  2. Compute hash from extension contents (including headless flag)
//  3. Check cache for existing binary
//  4. Build if cache miss
//  5. Exec the binary
func (l *Launcher) Run(ctx context.Context, projectDir string, args []string, configPath, agentLoop string, headless bool, exclude []string) error {
	homeDir := l.HomeDir
	if homeDir == "" {
		var err error

		homeDir, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("launcher: get home dir: %w", err)
		}
	}

	exts, err := AutoDiscover(projectDir, homeDir, l.ModuleRoot, exclude)
	if err != nil {
		return fmt.Errorf("launcher: auto-discover: %w", err)
	}

	buildInputs := deriveBuildInputs(exts, headless)

	hash, err := ComputeHash(buildInputs, l.ModuleRoot, l.ModuleVersion, headless, agentLoop, l.coreDirs()...)
	if err != nil {
		return fmt.Errorf("launcher: hash: %w", err)
	}

	binPath, found := l.Cache.Lookup(hash)
	if !found {
		fmt.Fprintf(os.Stderr, "weave: building binary with %d extensions...\n", len(buildInputs))

		binPath, err = l.buildAndCache(ctx, hash, agentLoop, headless, buildInputs, l.ModuleVersion)
		if err != nil {
			return fmt.Errorf("launcher: build: %w", err)
		}
	}

	return l.exec(ctx, binPath, configPath, agentLoop, headless, projectDir, args)
}

func deriveBuildInputs(exts []ExtensionInfo, headless bool) []ExtensionInfo {
	inputs := make([]ExtensionInfo, 0, len(exts))
	for _, ext := range exts {
		if headless && ext.IsUIExt {
			continue
		}

		inputs = append(inputs, ext)
	}

	return inputs
}

func (l *Launcher) buildAndCache(ctx context.Context, hash, agentLoop string, headless bool, exts []ExtensionInfo, moduleVersion string) (string, error) {
	unlock, lockErr := lockBuildDir(hash)
	if lockErr != nil {
		return "", fmt.Errorf("acquire build lock: %w", lockErr)
	}
	defer unlock()

	// Re-check cache after acquiring lock — another process may have built it.
	if cached, found := l.Cache.Lookup(hash); found {
		return cached, nil
	}

	buildDir := l.buildDir(hash)
	if err := os.MkdirAll(buildDir, 0o750); err != nil {
		return "", fmt.Errorf("mkdir build dir: %w", err)
	}

	defer func() { _ = os.RemoveAll(buildDir) }()

	binPath, err := l.Build(ctx, buildDir, l.ModuleRoot, moduleVersion, agentLoop, headless, exts)
	if err != nil {
		return "", err
	}

	if err := l.Cache.Store(hash, binPath); err != nil {
		return "", fmt.Errorf("cache store: %w", err)
	}

	cached, found := l.Cache.Lookup(hash)
	if !found {
		return "", errors.New("cache: binary not found after store")
	}

	return cached, nil
}

func (l *Launcher) coreDirs() []string {
	if l.ModuleRoot == "" {
		return nil
	}

	return []string{
		filepath.Join(l.ModuleRoot, "sdk"),
		filepath.Join(l.ModuleRoot, "bus"),
		filepath.Join(l.ModuleRoot, "settings"),
		filepath.Join(l.ModuleRoot, "utils", "truncate"),
		filepath.Join(l.ModuleRoot, "internal"),
	}
}

func (l *Launcher) buildDir(hash string) string {
	if l.BuildTmpDir != "" {
		return filepath.Join(l.BuildTmpDir, hash)
	}

	return filepath.Join(os.TempDir(), "weave-build-"+hash)
}

func (l *Launcher) exec(_ context.Context, binPath, configPath, agentLoop string, headless bool, projectDir string, args []string) error {
	argv := []string{binPath}
	if configPath != "" {
		argv = append(argv, "--weave-config="+configPath)
	}

	argv = append(argv, "--weave-agent-loop="+agentLoop)

	if headless {
		argv = append(argv, "--weave-headless=true")
	}

	if projectDir != "" {
		argv = append(argv, "--weave-project-dir="+projectDir)
	}

	argv = append(argv, args...)

	launcherPath, _ := os.Executable()
	if launcherPath == "" {
		launcherPath = os.Args[0]
	}

	if !filepath.IsAbs(launcherPath) {
		absPath, err := filepath.Abs(launcherPath)
		if err == nil {
			launcherPath = absPath
		}
	}

	// binPath is <cache.Root>/<hash>/weave — extract hash for reload.
	hash := filepath.Base(filepath.Dir(binPath))

	origArgs, _ := json.Marshal(os.Args)

	// Pass module root so generated binaries can locate built-in extension skills
	// regardless of the user's current working directory.
	// Prepend our vars before os.Environ() so they override any stale values
	// that may already exist in the parent environment.
	env := buildExecEnv(os.Environ(), launcherPath, hash, string(origArgs), l.ModuleRoot, l.ModuleVersion)

	return fmt.Errorf("exec binary: %w", syscall.Exec(binPath, argv, env))
}

// buildExecEnv constructs the environment for the exec'd binary.
// Weave-specific vars are prepended before the parent environment so they
// override any stale values that may already be present.
func buildExecEnv(parentEnv []string, launcherPath, hash, origArgs, moduleRoot, moduleVersion string) []string {
	env := []string{
		"WEAVE_LAUNCHER_PATH=" + launcherPath,
		"WEAVE_BUILD_HASH=" + hash,
		"WEAVE_ORIG_ARGS=" + origArgs,
	}

	if moduleRoot != "" {
		env = append(env, "WEAVE_MODULE_ROOT="+moduleRoot)
	}

	if moduleVersion != "" {
		env = append(env, "WEAVE_MODULE_VERSION="+moduleVersion)
	}

	return append(env, parentEnv...)
}

// lockBuildDir acquires a file-based lock for the given build hash to prevent
// concurrent builds from racing on the shared build directory.
func lockBuildDir(hash string) (unlock func(), err error) {
	lockPath := filepath.Join(os.TempDir(), "weave-build-"+hash+".lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	fd := int(f.Fd())

	// Retry with backoff for up to 30s — another process may be building.
	deadline := time.Now().Add(30 * time.Second)

	for {
		if lockErr := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); lockErr == nil {
			break
		}

		if time.Now().After(deadline) {
			_ = f.Close()

			return nil, fmt.Errorf("timed out waiting for build lock %s", lockPath)
		}

		time.Sleep(100 * time.Millisecond)
	}

	return func() {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
