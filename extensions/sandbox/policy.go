package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// mandatoryDenyWritePaths are paths that are always blocked from writes.
// Paths prefixed with "project:" are resolved against the project root
// (found by walking up from CWD), not CWD itself.
var mandatoryDenyWritePaths = []string{
	"~/.ssh/",
	"~/.bashrc",
	"~/.zshrc",
	"~/.profile",
	"~/.gitconfig",
	"project:.git/hooks/",
	"project:.git/config",
	"project:.weave/",
}

// defaultCachePaths are directories always allowed for writes by sandboxed
// commands. These are standard cache locations used by developer tools
// (golangci-lint, go build, npm, pip, etc.).
var defaultCachePaths = []string{
	"~/.cache",
	"~/Library/Caches",
	"~/.npm",
	"~/.local/share",
}

// mandatoryDenyReadPatterns are glob patterns always blocked from reading.
var mandatoryDenyReadPatterns = []string{
	"~/.ssh/id_*",
	"~/.aws/credentials",
	"**/.env",
	"**/.env.*",
}

func isDeniedWrite(abs, cwd string) bool {
	home, _ := os.UserHomeDir()

	for _, deny := range mandatoryDenyWritePaths {
		expanded := expandDenyRule(deny, home, cwd)

		if strings.HasSuffix(deny, "/") {
			// Directory rule (original had trailing /): prefix match.
			if strings.HasPrefix(abs, expanded+"/") || abs == expanded {
				return true
			}
		} else {
			// File rule: exact match only.
			if abs == expanded {
				return true
			}
		}
	}

	return false
}

func isDeniedRead(abs string) bool {
	home, _ := os.UserHomeDir()

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

				base := filepath.Base(abs)
				if m, _ := filepath.Match(suffix, base); m {
					return true
				}
			}
		}
	}

	return false
}

func pathMatches(path, pattern, cwd string) bool {
	if pattern == "." || pattern == "" {
		dir := cwd
		if dir == "" {
			dir, _ = os.Getwd()
		}

		return path == dir || strings.HasPrefix(path, dir+"/")
	}

	home, _ := os.UserHomeDir()
	expanded := expandHome(pattern, home)

	// Resolve relative paths against project root so they match absolute tool paths.
	if !filepath.IsAbs(expanded) {
		if cwd != "" {
			expanded = filepath.Join(cwd, expanded)
		} else if abs, err := filepath.Abs(expanded); err == nil {
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
// It walks path components left-to-right, following symlinks before
// processing '..' — matching kernel VFS behavior. This prevents
// symlink-plus-dotdot bypasses where filepath.Abs/Clean would remove
// '..' before symlinks are resolved.
func resolveAbs(path string) string {
	var abs string
	if filepath.IsAbs(path) {
		abs = path
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return path
		}

		abs = cwd + "/" + path
	}

	return walkComponents(abs)
}

// maxSymlinkDepth limits recursive symlink resolution to prevent infinite loops.
const maxSymlinkDepth = 40

// walkComponents resolves a path by processing components left-to-right using
// a queue. Symlinks are followed before '..' is processed, and symlink target
// components are reinserted into the queue so chained symlinks (link1 -> link2
// -> target) are fully resolved — matching kernel VFS behavior.
func walkComponents(path string) string {
	var queue []string

	for p := range strings.SplitSeq(path, "/") {
		if p != "" && p != "." {
			queue = append(queue, p)
		}
	}

	var stack []string

	symDepth := 0

	for len(queue) > 0 {
		if symDepth > maxSymlinkDepth {
			break
		}

		part := queue[0]
		queue = queue[1:]

		if part == ".." {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

			continue
		}

		candidate := "/" + strings.Join(append(stack, part), "/")

		fi, err := os.Lstat(candidate)
		if err != nil {
			stack = appendRemaining(stack, part, queue)

			break
		}

		if fi.Mode()&os.ModeSymlink != 0 {
			symDepth++

			link, err := os.Readlink(candidate)
			if err != nil {
				stack = append(stack, part)

				continue
			}

			var linkParts []string

			for lp := range strings.SplitSeq(link, "/") {
				if lp != "" && lp != "." {
					linkParts = append(linkParts, lp)
				}
			}

			if filepath.IsAbs(link) {
				stack = nil
			}

			queue = append(linkParts, queue...)
		} else {
			stack = append(stack, part)
		}
	}

	result := finalizeStack(stack)

	if resolved, err := filepath.EvalSymlinks(result); err == nil {
		return resolved
	}

	return resolveNonExistent(result)
}

// appendRemaining adds a non-existent component and all remaining parts to the stack.
func appendRemaining(stack []string, part string, remaining []string) []string {
	stack = append(stack, part)

	for _, r := range remaining {
		switch r {
		case "..":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, r)
		}
	}

	return stack
}

// resolveNonExistent walks up a non-existent path to find the deepest
// existing ancestor, resolves it, then appends the remaining components.
func resolveNonExistent(path string) string {
	dir := path

	var suffix []string

	for {
		if rd, err := filepath.EvalSymlinks(dir); err == nil {
			if len(suffix) == 0 {
				return rd
			}

			return filepath.Join(append([]string{rd}, suffix...)...)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return path
		}

		suffix = append([]string{filepath.Base(dir)}, suffix...)
		dir = parent
	}
}

func finalizeStack(stack []string) string {
	if len(stack) == 0 {
		return "/"
	}

	return "/" + strings.Join(stack, "/")
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
// Paths with "project:" prefix are resolved against the project root
// (found by walking up from dir for .git or .weave).
func expandDenyRule(path, home, dir string) string {
	if strings.HasPrefix(path, "~/") {
		return expandHome(path, home)
	}

	if rel, ok := strings.CutPrefix(path, "project:"); ok {
		root := findProjectRoot(dir)
		return filepath.Join(root, rel)
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

// findProjectRoot walks up from dir to find the nearest ancestor containing
// .git (directory) or .weave/ (directory). Falls back to dir itself.
func findProjectRoot(dir string) string {
	if dir == "" {
		dir, _ = os.Getwd()
	}

	cur := dir
	for {
		if fi, err := os.Stat(filepath.Join(cur, ".git")); err == nil && fi.IsDir() {
			return cur
		}

		if fi, err := os.Stat(filepath.Join(cur, ".weave")); err == nil && fi.IsDir() {
			return cur
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}

		cur = parent
	}

	return dir
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
