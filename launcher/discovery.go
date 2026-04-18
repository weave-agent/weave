package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
		info, err := findExtension(projectDir, homeDir, name)
		if err != nil {
			return nil, err
		}

		exts = append(exts, *info)
	}

	return exts, nil
}

func findExtension(projectDir, homeDir, name string) (*ExtensionInfo, error) {
	candidates := []string{
		filepath.Join(projectDir, ".weave", "extensions", name),
		filepath.Join(homeDir, ".weave", "extensions", name),
	}

	for _, dir := range candidates {
		goFiles, err := collectGoFiles(dir)
		if err != nil {
			continue
		}

		if len(goFiles) > 0 {
			return &ExtensionInfo{
				Name:    name,
				Dir:     dir,
				GoFiles: goFiles,
			}, nil
		}
	}

	return nil, fmt.Errorf("discover: extension %q not found in .weave/extensions/ (local or global)", name)
}

func collectGoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		if strings.HasSuffix(e.Name(), ".go") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .go files in %s", dir)
	}

	sort.Strings(files)

	return files, nil
}
