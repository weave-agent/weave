package launcher

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var validExtName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type ExtensionInfo struct {
	Name    string
	Dir     string
	GoFiles []string
}

// Discover resolves each named extension to its source directory and Go files.
// For each name, it checks project-local (.weave/extensions/{name}/) then
// global (~/.weave/extensions/{name}/).
func Discover(projectDir string, names []string) ([]ExtensionInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("discover: get home dir: %w", err)
	}

	return DiscoverCustomHome(projectDir, homeDir, names)
}

// DiscoverCustomHome is like Discover but accepts an explicit home directory.
func DiscoverCustomHome(projectDir, homeDir string, names []string) ([]ExtensionInfo, error) {
	var exts []ExtensionInfo

	for _, name := range names {
		if !validExtName.MatchString(name) {
			return nil, fmt.Errorf("discover: invalid extension name %q (must match [a-zA-Z0-9_-]+)", name)
		}

		info, err := findExtension(projectDir, homeDir, name)
		if err != nil {
			return nil, err
		}

		exts = append(exts, *info)
	}

	return exts, nil
}

func findExtension(projectDir, homeDir, name string) (*ExtensionInfo, error) {
	localDir := filepath.Join(projectDir, ".weave", "extensions", name)

	stat, statErr := os.Stat(localDir)
	if statErr == nil {
		if !stat.IsDir() {
			return nil, fmt.Errorf("discover: local extension path %q exists but is not a directory", localDir)
		}

		goFiles, err := collectGoFiles(localDir)
		if err != nil {
			return nil, fmt.Errorf("discover: local extension %q: %w", name, err)
		}

		return &ExtensionInfo{
			Name:    name,
			Dir:     localDir,
			GoFiles: goFiles,
		}, nil
	}

	if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("discover: local extension path %q: %w", localDir, statErr)
	}

	globalDir := filepath.Join(homeDir, ".weave", "extensions", name)

	goFiles, err := collectGoFiles(globalDir)
	if err != nil {
		return nil, fmt.Errorf("discover: extension %q not found in .weave/extensions/ (local or global)", name)
	}

	if len(goFiles) > 0 {
		return &ExtensionInfo{
			Name:    name,
			Dir:     globalDir,
			GoFiles: goFiles,
		}, nil
	}

	return nil, fmt.Errorf("discover: extension %q not found in .weave/extensions/ (local or global)", name)
}

func collectGoFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect go files in %s: %w", dir, err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .go files in %s", dir)
	}

	sort.Strings(files)

	return files, nil
}
