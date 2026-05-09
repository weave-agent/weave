package sandbox

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// mandatoryDenyWritePaths are paths that are always blocked from writes.
var mandatoryDenyWritePaths = []string{
	"~/.ssh/",
	"~/.bashrc",
	"~/.zshrc",
	"~/.profile",
	"~/.gitconfig",
	".git/hooks/",
	".git/config",
	".weave/",
}

// mandatoryDenyReadPatterns are glob patterns always blocked from reading.
var mandatoryDenyReadPatterns = []string{
	"~/.ssh/id_*",
	"~/.aws/credentials",
	"**/.env",
	"**/.env.*",
}

func isDeniedWrite(path string) bool {
	home, _ := os.UserHomeDir()

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	for _, deny := range mandatoryDenyWritePaths {
		expanded := expandDenyRule(deny, home, "")
		if strings.HasPrefix(abs, expanded) || strings.HasPrefix(path, deny) {
			return true
		}
	}

	return false
}

func isDeniedRead(path string) bool {
	home, _ := os.UserHomeDir()

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	for _, pattern := range mandatoryDenyReadPatterns {
		expanded := expandHome(pattern, home)

		matched, _ := filepath.Match(expanded, abs)
		if matched {
			return true
		}

		// Handle **/ prefix: match basename against the suffix.
		if strings.Contains(expanded, "**/") {
			parts := strings.SplitN(expanded, "**/", 2)
			if len(parts) == 2 {
				suffix := parts[1]
				// Match suffix (which may contain *) against the basename.
				base := filepath.Base(abs)
				if m, _ := filepath.Match(suffix, base); m {
					return true
				}
			}
		}

		if strings.HasPrefix(path, pattern) {
			return true
		}
	}

	return false
}

func pathMatches(path, pattern string) bool {
	if pattern == "." || pattern == "" {
		cwd, _ := os.Getwd()
		return path == cwd || strings.HasPrefix(path, cwd+"/")
	}

	home, _ := os.UserHomeDir()
	expanded := expandHome(pattern, home)

	if strings.HasSuffix(expanded, "/") {
		return strings.HasPrefix(path, expanded)
	}

	return path == expanded || strings.HasPrefix(path, expanded+"/")
}

func expandHome(path, home string) string {
	if strings.HasPrefix(path, "~/") {
		if home != "" {
			return filepath.Join(home, path[2:])
		}
	}

	return path
}

// expandDenyRule expands a deny rule for comparison against absolute paths.
// Home-relative paths (~/.ssh/) are expanded. Project-relative paths
// (.git/hooks/) are resolved against the given base directory.
func expandDenyRule(path, home, dir string) string {
	if strings.HasPrefix(path, "~/") {
		return expandHome(path, home)
	}

	if !filepath.IsAbs(path) {
		if dir != "" {
			return filepath.Join(dir, path)
		}

		abs, err := filepath.Abs(path)
		if err == nil {
			return abs
		}
	}

	return path
}

// wrapCommandPlatformWithConfig dispatches to the OS-specific implementation
// with an explicit config.
func wrapCommandPlatformWithConfig(cmd, dir string, cfg SandboxConfig) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return wrapCommandDarwinWithConfig(cmd, dir, cfg)
	case "linux":
		return wrapCommandLinuxWithConfig(cmd, dir, cfg)
	default:
		return cmd, nil
	}
}
