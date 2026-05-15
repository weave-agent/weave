package agent

import (
	"os"
	"path/filepath"
)

// contextFile represents a discovered context file (CLAUDE.md or AGENTS.md).
type contextFile struct {
	Path    string
	Content string
}

// discoverContextFiles walks up from projectDir looking for CLAUDE.md and AGENTS.md,
// then falls back to globalDir. Project files take precedence over global files.
// Only the first matching file per directory is returned.
func discoverContextFiles(projectDir, globalDir string) []contextFile {
	var files []contextFile

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

			files = append([]contextFile{{Path: path, Content: string(data)}}, files...)

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

			files = append(files, contextFile{Path: path, Content: string(data)})

			break
		}
	}

	return files
}

// loadSystemPrompt loads SYSTEM.md (base) and APPEND_SYSTEM.md (append) from
// projectDir/.weave/ first, then globalDir. Project overrides global.
func loadSystemPrompt(projectDir, globalDir string) (base, append_ string) {
	base = loadFirst("SYSTEM.md", projectDir, globalDir)
	append_ = loadFirst("APPEND_SYSTEM.md", projectDir, globalDir)

	return base, append_
}

// discoverCompactPrompt looks for COMPACT.md in projectDir/.weave/ first,
// then globalDir. Project overrides global. Returns empty string if not found.
func discoverCompactPrompt(projectDir, globalDir string) string {
	return loadFirst("COMPACT.md", projectDir, globalDir)
}

// resolveCompactPrompt returns the summarization instructions in priority order:
//  1. customInstructions (from /compact args) if non-empty
//  2. COMPACT.md content (project or global) if found
//  3. default embedded prompt
func resolveCompactPrompt(customInstructions, projectDir, globalDir string) string {
	if customInstructions != "" {
		return customInstructions
	}

	if content := discoverCompactPrompt(projectDir, globalDir); content != "" {
		return content
	}

	return defaultCompactPrompt
}

// loadFirst reads filename from projectDir/.weave/ if projectDir is set,
// then falls back to globalDir.
func loadFirst(filename, projectDir, globalDir string) string {
	if projectDir != "" {
		data, err := os.ReadFile(filepath.Join(projectDir, ".weave", filename))
		if err == nil {
			return string(data)
		}
	}

	if globalDir != "" {
		data, err := os.ReadFile(filepath.Join(globalDir, filename))
		if err == nil {
			return string(data)
		}
	}

	return ""
}
