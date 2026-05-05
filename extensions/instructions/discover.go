package instructions

import (
	"os"
	"path/filepath"
)

type ContextFile struct {
	Path    string
	Content string
}

func discoverContextFiles(projectDir, globalDir string) []ContextFile {
	var files []ContextFile

	seen := make(map[string]bool)

	contextNames := []string{"CLAUDE.md", "AGENTS.md"}

	// Walk up from projectDir to filesystem root
	dir := projectDir
	for dir != "" {
		for _, name := range contextNames {
			path := filepath.Join(dir, name)

			abs := path
			if a, err := filepath.Abs(path); err == nil {
				abs = a
			}

			if seen[abs] {
				continue
			}

			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			seen[abs] = true

			files = append([]ContextFile{{Path: path, Content: string(data)}}, files...)

			break // only first match per directory
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	// Global fallback
	if globalDir != "" {
		for _, name := range contextNames {
			path := filepath.Join(globalDir, name)

			abs := path
			if a, err := filepath.Abs(path); err == nil {
				abs = a
			}

			if seen[abs] {
				continue
			}

			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			seen[abs] = true

			files = append(files, ContextFile{Path: path, Content: string(data)})

			break
		}
	}

	return files
}
