package wire

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"weave/config"
)

const cloneTimeout = 5 * time.Minute

type sourceType int

const (
	sourceGitURL sourceType = iota
	sourceGitHub
	sourceLocalPath
)

type parsedSource struct {
	kind     sourceType
	gitURL   string
	localDir string
	rawName  string
}

func runInstall(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "weave install: missing source argument")
		fmt.Fprintln(os.Stderr, "usage: weave install <source> [--name <name>]")

		return 1
	}

	source := args[0]
	name := ""

	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == "--name":
			if i+1 >= len(rest) {
				fmt.Fprintln(os.Stderr, "weave install: --name requires a value")
				return 1
			}

			name = rest[i+1] //nolint:gosec // G602 — bounds check immediately above
			i++
		case strings.HasPrefix(rest[i], "--name="):
			n, _ := strings.CutPrefix(rest[i], "--name=")
			name = n
		default:
			fmt.Fprintf(os.Stderr, "weave install: unknown argument %q\n", rest[i])
			return 1
		}
	}

	parsed, err := parseSource(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave install: %v\n", err)
		return 1
	}

	extName := name
	if extName == "" {
		extName = parsed.rawName
	}

	if !config.ValidExtName(extName) {
		fmt.Fprintf(os.Stderr, "weave install: invalid extension name %q (must match [a-zA-Z0-9_-]+)\n", extName)
		return 1
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave install: %v\n", err)
		return 1
	}

	destDir := filepath.Join(homeDir, ".weave", "extensions", extName)

	// Reject self-install: a local source that resolves to or contains the
	// destination would be deleted by the staging-replace step.
	if parsed.kind == sourceLocalPath {
		if selfErr := rejectSelfInstall(parsed.localDir, destDir); selfErr != nil {
			fmt.Fprintf(os.Stderr, "weave install: %v\n", selfErr)
			return 1
		}
	}

	stagingDir, err := stagingPath(homeDir, extName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave install: %v\n", err)
		return 1
	}

	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	switch parsed.kind {
	case sourceGitURL, sourceGitHub:
		if err := cloneExtension(parsed.gitURL, stagingDir); err != nil {
			fmt.Fprintf(os.Stderr, "weave install: clone: %v\n", err)
			return 1
		}

	case sourceLocalPath:
		if err := copyExtension(parsed.localDir, stagingDir); err != nil {
			fmt.Fprintf(os.Stderr, "weave install: copy: %v\n", err)
			return 1
		}
	}

	if !hasGoFiles(stagingDir) {
		fmt.Fprintf(os.Stderr, "weave install: %s contains no .go files\n", source)
		return 1
	}

	if err := swapStaging(stagingDir, destDir); err != nil {
		fmt.Fprintf(os.Stderr, "weave install: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "installed extension %q to %s\n", extName, destDir)

	return 0
}

func rejectSelfInstall(srcAbs, destDir string) error {
	srcClean := filepath.Clean(srcAbs)
	destClean := filepath.Clean(destDir)

	if srcClean == destClean {
		return fmt.Errorf("source and destination are the same path %q", destClean)
	}

	rel, err := filepath.Rel(srcClean, destClean)
	if err == nil && rel != "" && !strings.HasPrefix(rel, "..") && rel != "." {
		return fmt.Errorf("destination %q is inside source %q", destClean, srcClean)
	}

	return nil
}

func stagingPath(homeDir, extName string) (string, error) {
	parent := filepath.Join(homeDir, ".weave", "extensions")
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return "", fmt.Errorf("create extensions dir: %w", err)
	}

	staging, err := os.MkdirTemp(parent, ".staging-"+extName+"-")
	if err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}

	if err := os.Remove(staging); err != nil {
		return "", fmt.Errorf("prepare staging dir: %w", err)
	}

	return staging, nil
}

func swapStaging(stagingDir, destDir string) error {
	if _, err := os.Stat(destDir); err == nil {
		if err := os.RemoveAll(destDir); err != nil {
			return fmt.Errorf("remove existing extension: %w", err)
		}
	}

	if err := os.Rename(stagingDir, destDir); err != nil {
		return fmt.Errorf("install staged extension: %w", err)
	}

	return nil
}

func parseSource(source string) (parsedSource, error) {
	// Reject insecure transports — extensions are compiled and executed, so
	// unauthenticated transports allow MITM code injection.
	if strings.HasPrefix(source, "http://") {
		return parsedSource{}, fmt.Errorf("insecure URL %q (use https:// instead)", source)
	}

	if strings.HasPrefix(source, "git://") {
		return parsedSource{}, fmt.Errorf("insecure URL %q (use https:// or ssh instead)", source)
	}

	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "ssh://") {
		return parsedSource{
			kind:    sourceGitURL,
			gitURL:  source,
			rawName: deriveNameFromURL(source),
		}, nil
	}

	if rest, ok := strings.CutPrefix(source, "github.com/"); ok {
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return parsedSource{}, fmt.Errorf("invalid GitHub shorthand %q (expected github.com/user/repo)", source)
		}

		repoName := strings.TrimSuffix(parts[1], ".git")

		return parsedSource{
			kind:    sourceGitHub,
			gitURL:  "https://" + source,
			rawName: repoName,
		}, nil
	}

	if config.IsPathEntry(source) || filepath.IsAbs(source) {
		abs, err := config.ResolveExtPath(source, "")
		if err != nil {
			return parsedSource{}, fmt.Errorf("resolve path: %w", err)
		}

		stat, err := os.Stat(abs)
		if err != nil {
			return parsedSource{}, fmt.Errorf("path %q: %w", source, err)
		}

		if !stat.IsDir() {
			return parsedSource{}, fmt.Errorf("path %q is not a directory", source)
		}

		return parsedSource{
			kind:     sourceLocalPath,
			localDir: abs,
			rawName:  filepath.Base(abs),
		}, nil
	}

	return parsedSource{}, fmt.Errorf("invalid source %q (expected git URL, github.com/user/repo, or local path)", source)
}

func deriveNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return filepath.Base(rawURL)
	}

	base := path.Base(parsed.Path)
	base = strings.TrimSuffix(base, ".git")

	return base
}

func cloneExtension(gitURL, destDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", gitURL, destDir)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git clone %s: timed out after %s", gitURL, cloneTimeout)
		}

		return fmt.Errorf("git clone %s: %w", gitURL, err)
	}

	return nil
}

func copyExtension(srcDir, destDir string) error {
	// Verify symlink targets stay inside the source tree to prevent
	// exfiltrating files outside it (e.g. /etc/passwd, ~/.ssh/id_rsa).
	absSrcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return fmt.Errorf("resolve source dir: %w", err)
	}

	absSrcDir, err = filepath.EvalSymlinks(absSrcDir)
	if err != nil {
		return fmt.Errorf("resolve source dir: %w", err)
	}

	if err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk: %w", err)
		}

		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}

		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}

		target := filepath.Join(destDir, rel)

		if d.Type()&fs.ModeSymlink != 0 {
			// Never preserve symlinks as-is to prevent path traversal.
			resolved, resolveErr := filepath.EvalSymlinks(path)
			if resolveErr != nil {
				return fmt.Errorf("resolve symlink %q: %w", path, resolveErr)
			}

			if !pathContains(absSrcDir, resolved) {
				return fmt.Errorf("symlink %q points outside source dir", path)
			}

			stat, statErr := os.Stat(resolved)
			if statErr != nil {
				return fmt.Errorf("stat symlink target %q: %w", resolved, statErr)
			}

			if stat.IsDir() {
				return os.MkdirAll(target, 0o750)
			}

			data, readErr := os.ReadFile(resolved) //nolint:gosec // G122 — reading from known source dir
			if readErr != nil {
				return fmt.Errorf("read file: %w", readErr)
			}

			return os.WriteFile(target, data, stat.Mode().Perm()) //nolint:gosec // G703 — our own extension dir
		}

		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}

		data, readErr := os.ReadFile(path) //nolint:gosec // G122 — reading from known source dir
		if readErr != nil {
			return fmt.Errorf("read file: %w", readErr)
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("file info: %w", infoErr)
		}

		return os.WriteFile(target, data, info.Mode().Perm()) //nolint:gosec // G703 — our own extension dir
	}); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	return nil
}

func pathContains(parent, target string) bool {
	rel, err := filepath.Rel(parent, target)
	if err != nil {
		return false
	}

	if rel == "." {
		return true
	}

	return !strings.HasPrefix(rel, "..") && rel != ".."
}

func hasGoFiles(dir string) bool {
	found := false

	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, _ error) error {
		if d != nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
			found = true
			return fs.SkipAll
		}

		return nil
	})
	if err != nil {
		return false
	}

	return found
}
