package main

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

	"weave/config"
)

// sourceType identifies the kind of install source.
type sourceType int

const (
	sourceGitURL    sourceType = iota // https:// URL
	sourceGitHub                      // github.com/user/repo shorthand
	sourceLocalPath                   // local filesystem path
)

// parsedSource is the result of parsing an install source string.
type parsedSource struct {
	kind     sourceType
	gitURL   string // full git clone URL (for git/GitHub sources)
	localDir string // absolute local path (for local sources)
	rawName  string // derived extension name (basename without .git)
}

// runInstall handles `weave install <source> [--name <name>]`.
func runInstall(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "weave install: missing source argument")
		fmt.Fprintln(os.Stderr, "usage: weave install <source> [--name <name>]")

		return 1
	}

	source := args[0]
	name := ""

	// Parse --name flag from remaining args.
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

	// Use explicit name or derived name.
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

	switch parsed.kind {
	case sourceGitURL, sourceGitHub:
		if err := cloneExtension(parsed.gitURL, destDir); err != nil {
			fmt.Fprintf(os.Stderr, "weave install: clone: %v\n", err)
			return 1
		}

	case sourceLocalPath:
		if err := copyExtension(parsed.localDir, destDir); err != nil {
			fmt.Fprintf(os.Stderr, "weave install: copy: %v\n", err)
			return 1
		}
	}

	// Validate that .go files exist.
	if !hasGoFiles(destDir) {
		_ = os.RemoveAll(destDir) //nolint:gosec // G703 — cleaning up our own extension dir

		fmt.Fprintf(os.Stderr, "weave install: %s contains no .go files\n", source)

		return 1
	}

	fmt.Fprintf(os.Stderr, "installed extension %q to %s\n", extName, destDir)

	return 0
}

// parseSource classifies and resolves an install source string.
func parseSource(source string) (parsedSource, error) {
	// Reject insecure http:// URLs — extensions are compiled and executed.
	if strings.HasPrefix(source, "http://") {
		return parsedSource{}, fmt.Errorf("insecure URL %q (use https:// instead)", source)
	}

	// Git URL: https:// only (reject git:// as unencrypted).
	if strings.HasPrefix(source, "https://") {
		return parsedSource{
			kind:    sourceGitURL,
			gitURL:  source,
			rawName: deriveNameFromURL(source),
		}, nil
	}

	// GitHub shorthand: github.com/user/repo
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

	// Local path: ./..., /..., ~/...
	if config.IsPathEntry(source) || filepath.IsAbs(source) {
		abs, err := config.ResolveExtPath(source, "")
		if err != nil {
			return parsedSource{}, fmt.Errorf("resolve path: %w", err)
		}

		stat, err := os.Stat(abs) //nolint:gosec // G703 — abs is resolved from user CLI arg
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

// deriveNameFromURL extracts the repo name from a git URL.
func deriveNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return filepath.Base(rawURL)
	}

	base := path.Base(parsed.Path)
	base = strings.TrimSuffix(base, ".git")

	return base
}

// cloneExtension runs git clone into destDir.
func cloneExtension(gitURL, destDir string) error {
	// If destDir already exists, remove it.
	if _, err := os.Stat(destDir); err == nil { //nolint:gosec // G703 — our own extension dir
		if err := os.RemoveAll(destDir); err != nil { //nolint:gosec // G703 — our own extension dir
			return fmt.Errorf("remove existing directory: %w", err)
		}
	}

	cmd := exec.CommandContext(context.Background(), "git", "clone", "--depth", "1", gitURL, destDir) //nolint:gosec // G702 — gitURL is user-provided CLI arg
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", gitURL, err)
	}

	return nil
}

// copyExtension copies a local directory to destDir.
func copyExtension(srcDir, destDir string) error {
	// If destDir already exists, remove it.
	if _, err := os.Stat(destDir); err == nil { //nolint:gosec // G703 — our own extension dir
		if err := os.RemoveAll(destDir); err != nil { //nolint:gosec // G703 — our own extension dir
			return fmt.Errorf("remove existing directory: %w", err)
		}
	}

	if err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk: %w", err)
		}

		// Skip .git directory.
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}

		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}

		target := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o750) //nolint:gosec // G703 — our own extension dir
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

// hasGoFiles reports whether a directory tree contains .go files.
func hasGoFiles(dir string) bool {
	found := false

	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, _ error) error { //nolint:gosec // G703 — reading from our own extension dir
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
