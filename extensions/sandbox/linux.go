//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	cfg := currentConfig()

	args := buildBwrapArgs(cfg, dir)

	// Shell-escape each arg and the command for safe bash -c consumption.
	quoted := make([]string, 0, len(args)+4)
	quoted = append(quoted, "bwrap")
	for _, a := range args {
		quoted = append(quoted, shellQuote(a))
	}
	quoted = append(quoted, "--", "bash", "-c", shellQuote(cmd))

	return strings.Join(quoted, " "), nil
}

// shellQuote wraps a string in single quotes, escaping embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
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
