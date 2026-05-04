package components

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// PathCompletions returns completion items for file paths relative to baseDir,
// filtered by prefix. The prefix may contain path separators (e.g. "src/ma");
// the directory part is resolved against baseDir and the final component is
// used as a name filter. Directories get a trailing "/" in their Value.
func PathCompletions(baseDir, prefix string) []CompletionItem {
	dirPart, filter := splitPrefix(prefix)

	searchDir := baseDir
	if dirPart != "" {
		searchDir = filepath.Join(baseDir, dirPart)
	}

	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}

	filterLower := strings.ToLower(filter)

	var items []CompletionItem

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		if filterLower != "" && !strings.HasPrefix(strings.ToLower(name), filterLower) {
			continue
		}

		isDir := entry.IsDir()
		label := name
		value := dirPart + name

		if isDir {
			label += "/"
			value += "/"
		}

		items = append(items, CompletionItem{
			Label: label,
			Value: value,
		})
	}

	sortCompletionItems(items)

	return items
}

// splitPrefix splits a path prefix into its directory and name-filter parts.
// For "src/ma" it returns ("src/", "ma"). For "ma" it returns ("", "ma").
func splitPrefix(prefix string) (dirPart, filter string) {
	prefix = filepath.ToSlash(prefix)

	idx := strings.LastIndex(prefix, "/")
	if idx == -1 {
		return "", prefix
	}

	return prefix[:idx+1], prefix[idx+1:]
}

func sortCompletionItems(items []CompletionItem) {
	slices.SortFunc(items, func(a, b CompletionItem) int {
		return cmp.Compare(a.Label, b.Label)
	})
}
