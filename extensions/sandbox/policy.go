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
		expanded := expandHome(deny, home)
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
		return strings.HasPrefix(path, cwd)
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

// wrapCommandPlatform dispatches to the OS-specific implementation.
func wrapCommandPlatform(cmd, dir string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return wrapCommandDarwin(cmd, dir)
	case "linux":
		return wrapCommandLinux(cmd, dir)
	default:
		return cmd, nil
	}
}
