//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
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

	args := buildBwrapArgs(dir)
	escaped := strings.ReplaceAll(cmd, "'", "'\\''")
	parts := []string{"bwrap"}
	parts = append(parts, args...)
	parts = append(parts, "--", "bash", "-c", "'"+escaped+"'")

	return strings.Join(parts, " "), nil
}

func buildBwrapArgs(dir string) []string {
	var args []string

	args = append(args, "--ro-bind", "/", "/")

	if dir == "" {
		dir, _ = os.Getwd()
	}
	args = append(args, "--bind", dir, dir)

	args = append(args, "--unshare-pid", "--proc", "/proc")

	s := getCurrentSandbox()
	if s != nil && !s.cfg.Network {
		args = append(args, "--unshare-net")
	}

	return args
}

func wrapCommandDarwin(cmd, dir string) (string, error) {
	return cmd, nil
}

func generateSeatbeltProfile(cfg SandboxConfig, dir string) string {
	return ""
}
