package launcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// BuildFunc builds a binary from extension infos and returns its path.
type BuildFunc func(dir string, moduleRoot string, exts []ExtensionInfo) (string, error)

// Launcher orchestrates the full pipeline: discover -> hash -> cache -> build -> exec.
type Launcher struct {
	Cache       *Cache
	Build       BuildFunc
	ModuleRoot  string
	BuildTmpDir string
}

// NewLauncher creates a Launcher with the default Build function.
func NewLauncher(cache *Cache, moduleRoot string) *Launcher {
	return &Launcher{
		Cache:      cache,
		Build:      Build,
		ModuleRoot: moduleRoot,
	}
}

// Run executes the full launcher pipeline:
//  1. Discover extension source directories
//  2. Compute hash from extension contents
//  3. Check cache for existing binary
//  4. Build if cache miss
//  5. Exec the binary
func (l *Launcher) Run(ctx context.Context, projectDir string, extensionNames []string, args []string) error {
	if len(extensionNames) == 0 {
		return fmt.Errorf("launcher: no extensions configured")
	}

	exts, err := Discover(projectDir, extensionNames)
	if err != nil {
		return fmt.Errorf("launcher: discover: %w", err)
	}

	hash, err := ComputeHash(exts)
	if err != nil {
		return fmt.Errorf("launcher: hash: %w", err)
	}

	binPath, found := l.Cache.Lookup(hash)
	if !found {
		binPath, err = l.buildAndCache(hash, exts)
		if err != nil {
			return fmt.Errorf("launcher: build: %w", err)
		}
	}

	return l.exec(ctx, binPath, args)
}

func (l *Launcher) buildAndCache(hash string, exts []ExtensionInfo) (string, error) {
	buildDir := l.buildDir(hash)
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir build dir: %w", err)
	}

	binPath, err := l.Build(buildDir, l.ModuleRoot, exts)
	if err != nil {
		return "", err
	}

	if err := l.Cache.Store(hash, binPath); err != nil {
		return "", fmt.Errorf("cache store: %w", err)
	}

	cached, _ := l.Cache.Lookup(hash)
	if cached != "" {
		return cached, nil
	}
	return binPath, nil
}

func (l *Launcher) buildDir(hash string) string {
	if l.BuildTmpDir != "" {
		return filepath.Join(l.BuildTmpDir, hash)
	}
	return filepath.Join(os.TempDir(), "weave-build-"+hash)
}

func (l *Launcher) exec(_ context.Context, binPath string, args []string) error {
	argv := append([]string{binPath}, args...)
	return syscall.Exec(binPath, argv, os.Environ())
}

// RunCommand runs the binary as a subprocess (non-replacing, for testing).
func RunCommand(ctx context.Context, binPath string, args []string) error {
	argv := append([]string{binPath}, args...)
	cmd := exec.CommandContext(ctx, binPath, argv[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
