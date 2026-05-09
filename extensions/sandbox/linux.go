//go:build linux

package sandbox

import (
	"fmt"
	"io/fs"
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
	if len(writable) == 1 && writable[0] == "" {
		// Sentinel: explicitly no writable paths (readonly mode).
		writable = nil
	} else if len(writable) == 0 {
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
		expanded := expandDenyRule(deny, home, dir)
		if strings.HasSuffix(deny, "/") {
			args = append(args, "--tmpfs", expanded)
		} else {
			args = append(args, "--ro-bind-try", "/dev/null", expanded)
		}
	}

	// User-configured deny write paths
	for _, deny := range cfg.DenyWrite {
		expanded := expandDenyRule(deny, home, dir)
		if strings.HasSuffix(deny, "/") {
			args = append(args, "--tmpfs", expanded)
		} else {
			args = append(args, "--ro-bind-try", "/dev/null", expanded)
		}
	}

	// Mandatory deny read paths (SSH keys, AWS credentials, .env files)
	for _, pattern := range mandatoryDenyReadPatterns {
		expanded := expandHome(pattern, home)

		if strings.Contains(expanded, "**/") {
			// Recursive glob: filepath.Glob does not support '**', so walk
			// the project directory to find matching files.
			parts := strings.SplitN(expanded, "**/", 2)
			suffix := parts[1]
			filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}

				if m, _ := filepath.Match(suffix, filepath.Base(path)); m {
					args = append(args, "--ro-bind-try", "/dev/null", path)
				}

				return nil
			})

			continue
		}

		if strings.Contains(expanded, "*") {
			// Expand simple globs (e.g. ~/.ssh/id_*) to individual file entries.
			matches, _ := filepath.Glob(expanded)
			for _, m := range matches {
				args = append(args, "--ro-bind-try", "/dev/null", m)
			}

			continue
		}

		args = append(args, "--ro-bind-try", "/dev/null", expanded)
	}

	// User-configured deny read paths
	for _, deny := range cfg.DenyRead {
		expanded := expandDenyRule(deny, home, dir)
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

// Darwin stubs for cross-compilation.
func generateSeatbeltProfile(cfg SandboxConfig, dir string) string {
	return ""
}

func wrapCommandDarwinWithConfig(cmd, dir string, cfg SandboxConfig) (string, error) {
	return cmd, nil
}

func wrapCommandLinuxWithConfig(cmd, dir string, cfg SandboxConfig) (string, error) {
	if err := bwrapAvailable(); err != nil {
		return "", err
	}

	args := buildBwrapArgs(cfg, dir)

	quoted := make([]string, 0, len(args)+4)
	quoted = append(quoted, "bwrap")
	for _, a := range args {
		quoted = append(quoted, shellQuote(a))
	}
	quoted = append(quoted, "--", "bash", "-c", shellQuote(cmd))

	return strings.Join(quoted, " "), nil
}
