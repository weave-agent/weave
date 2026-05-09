package sandbox

import (
	"fmt"
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

	abs := resolveAbs(path)

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

	abs := resolveAbs(path)

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

	// Resolve relative paths against CWD so they match absolute tool paths.
	if !filepath.IsAbs(expanded) {
		if abs, err := filepath.Abs(expanded); err == nil {
			expanded = abs
		}
	}

	if strings.HasSuffix(expanded, "/") {
		return strings.HasPrefix(path, expanded)
	}

	return path == expanded || strings.HasPrefix(path, expanded+"/")
}

// resolveAbs resolves a path to an absolute path, following symlinks
// when possible to prevent symlink-based deny rule bypasses.
// For non-existent files, it walks up the path to find the deepest
// existing ancestor, resolves it, then appends the remaining components.
func resolveAbs(path string) string {
	// Build an absolute path WITHOUT cleaning '..' first.
	// filepath.Abs calls filepath.Clean, which removes '..' before
	// symlinks are resolved, allowing symlink-plus-dotdot bypasses
	// (e.g. link/../target where link is a symlink escapes the project).
	// EvalSymlinks walks left-to-right, resolving symlinks before
	// processing '..', which is the correct order.
	var abs string
	if filepath.IsAbs(path) {
		abs = path
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return path
		}

		abs = cwd + string(filepath.Separator) + path
	}

	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}

	// Walk up to find the deepest existing ancestor, then append remaining components.
	dir := abs

	var suffix []string

	for {
		if resolvedDir, err := filepath.EvalSymlinks(dir); err == nil {
			if len(suffix) == 0 {
				return resolvedDir
			}

			return filepath.Join(append([]string{resolvedDir}, suffix...)...)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding an existing ancestor.
			return filepath.Clean(abs)
		}

		suffix = append([]string{filepath.Base(dir)}, suffix...)
		dir = parent
	}
}

// resolveAbsUnsafe returns the resolved absolute CWD, following symlinks.
func resolveAbsUnsafe() string {
	cwd, err := os.Getwd()
	if err != nil {
		return cwd
	}

	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		return resolved
	}

	return cwd
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
		return "", fmt.Errorf("sandbox: unsupported platform %s", runtime.GOOS)
	}
}
