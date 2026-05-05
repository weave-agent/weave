package instructions

import (
	"os"
	"path/filepath"
)

func loadSystemPrompt(projectDir, globalDir string) (base, append_ string) {
	base = loadFirst("SYSTEM.md", projectDir, globalDir)
	append_ = loadFirst("APPEND_SYSTEM.md", projectDir, globalDir)

	return base, append_
}

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
