//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"weave/sdk"
)

func bwrapAvailable() error {
	if _, err := exec.LookPath("bwrap"); err != nil {
		return fmt.Errorf("bubblewrap not installed")
	}
	return nil
}

func wrapCommandLinux(cmd, dir string) (string, error) {
	if err := bwrapAvailable(); err != nil {
		return "", err
	}

	s := getCurrentSandbox()
	var cfg SandboxConfig
	if s != nil {
		s.mu.RLock()
		cfg = s.cfg
		s.mu.RUnlock()
	} else {
		cfg = SandboxConfig{Mode: sdk.SandboxAuto, Network: true}
	}

	args := buildBwrapArgs(cfg, dir)
	escaped := strings.ReplaceAll(cmd, "'", "'\\''")
	parts := []string{"bwrap"}
	parts = append(parts, args...)
	parts = append(parts, "--", "bash", "-c", "'"+escaped+"'")

	return strings.Join(parts, " "), nil
}

func buildBwrapArgs(cfg SandboxConfig, dir string) []string {
	var args []string
	home, _ := os.UserHomeDir()

	// Read-only root
	args = append(args, "--ro-bind", "/", "/")

	// Resolve dir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	// Writable paths
	writable := cfg.Writable
	if len(writable) == 0 {
		writable = []string{dir}
	}
	for _, w := range writable {
		if w == "." {
			w = dir
		}
		args = append(args, "--bind", w, w)
	}

	// Mandatory deny write paths
	for _, deny := range mandatoryDenyWritePaths {
		expanded := expandDenyPath(deny, home, dir)
		if strings.HasSuffix(deny, "/") {
			args = append(args, "--tmpfs", expanded)
		} else {
			args = append(args, "--ro-bind-try", "/dev/null", expanded)
		}
	}

	// User-configured deny write paths
	for _, deny := range cfg.DenyWrite {
		expanded := expandDenyPath(deny, home, dir)
		if strings.HasSuffix(deny, "/") {
			args = append(args, "--tmpfs", expanded)
		} else {
			args = append(args, "--ro-bind-try", "/dev/null", expanded)
		}
	}

	// PID isolation
	args = append(args, "--unshare-pid", "--proc", "/proc")

	// Network
	if !cfg.Network {
		args = append(args, "--unshare-net")
	}

	return args
}

func expandDenyPath(path, home, dir string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	if !filepath.IsAbs(path) {
		return filepath.Join(dir, path)
	}
	return path
}

// Darwin stubs for cross-compilation.
func wrapCommandDarwin(cmd, dir string) (string, error) {
	return cmd, nil
}

func generateSeatbeltProfile(cfg SandboxConfig, dir string) string {
	return ""
}

func wrapCommandDarwinWithConfig(cmd, dir string, cfg SandboxConfig) (string, error) {
	return cmd, nil
}

func wrapCommandLinuxWithConfig(cmd, dir string, cfg SandboxConfig) (string, error) {
	return cmd, nil
}
