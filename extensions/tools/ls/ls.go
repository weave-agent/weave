package ls

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"weave/sdk"
	"weave/utils/truncate"
)

const (
	paramPath   = "path"
	paramLimit  = "limit"
	paramIgnore = "ignore"
	paramDepth  = "depth"
)

// LSConfig holds per-tool settings for the ls tool.
type LSConfig struct {
	Limit int `json:"limit" default:"500" env:"LIMIT"`
}

type tool struct {
	defaultLimit int
}

var (
	sandboxerMu sync.RWMutex
	sandboxer   sdk.Sandboxer
)

func setSandboxer(s sdk.Sandboxer) {
	sandboxerMu.Lock()
	sandboxer = s
	sandboxerMu.Unlock()
}

func getSandboxer() sdk.Sandboxer {
	sandboxerMu.RLock()

	s := sandboxer

	sandboxerMu.RUnlock()

	return s
}

func init() {
	sdk.OnBusReady(func(bus sdk.Bus) {
		bus.On("sandbox.registered", func(ev sdk.Event) error {
			if s, ok := ev.Payload.(sdk.Sandboxer); ok {
				setSandboxer(s)
			}

			return nil
		})
	})

	sdk.RegisterTool("ls", func(_ sdk.Config, _ sdk.PreferenceReader, cfg LSConfig) (sdk.Tool, error) {
		limit := cfg.Limit
		if limit <= 0 {
			limit = 500
		}

		return &tool{defaultLimit: limit}, nil
	})
}

func (t *tool) Name() string { return "ls" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "ls",
		Description: "List directory contents. Returns file and directory names with type indicators. Use depth > 0 for hierarchical tree output.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				paramPath: map[string]any{
					"type":        "string",
					"description": "Directory path to list. Defaults to current directory.",
				},
				paramLimit: map[string]any{
					"type":        "number",
					"description": "Maximum number of entries to return. Defaults to 500.",
				},
				paramIgnore: map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "Glob patterns to ignore. Entries matching any pattern are excluded.",
				},
				paramDepth: map[string]any{
					"type":        "number",
					"description": "Maximum depth for tree output. 0 = flat list (default). > 0 = recursive tree with depth limit.",
				},
			},
			"additionalProperties": false,
		},
	}
}

func (t *tool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	path, _ := args[paramPath].(string)
	if path == "" {
		path = "."
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if s := getSandboxer(); s != nil {
		if !s.AllowRead(absPath) {
			return sdk.ToolResult{Content: "sandbox: read denied — path is protected", IsError: true}, nil
		}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if !info.IsDir() {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s is not a directory", absPath), IsError: true}, nil
	}

	sb := getSandboxer()
	ignorePatterns := resolveIgnorePatterns(args)
	limit := resolveLimit(args, t.defaultLimit)
	depth := resolveDepth(args)

	if depth > 0 {
		return t.executeTree(absPath, depth, limit, sb, ignorePatterns)
	}

	return t.executeFlat(absPath, limit, sb, ignorePatterns)
}

func (t *tool) executeFlat(absPath string, limit int, sb sdk.Sandboxer, ignorePatterns []string) (sdk.ToolResult, error) {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if len(entries) == 0 {
		return sdk.ToolResult{Content: "(empty directory)", IsError: false}, nil
	}

	var lines []string

	for _, e := range entries {
		name := e.Name()
		if sb != nil && !sb.AllowRead(filepath.Join(absPath, name)) {
			continue
		}

		if matchesAnyIgnore(name, ignorePatterns) {
			continue
		}

		if e.IsDir() {
			lines = append(lines, name+"/")
		} else {
			lines = append(lines, name)
		}
	}

	sort.Slice(lines, func(i, j int) bool {
		return strings.ToLower(lines[i]) < strings.ToLower(lines[j])
	})

	truncated := false

	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
		truncated = true
	}

	output := strings.Join(lines, "\n")
	result := truncate.Truncate(output, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	content := result.Format()

	if truncated {
		content += "\n\n... (more entries not shown — use a higher limit to see all)"
	}

	return sdk.ToolResult{Content: content, IsError: false}, nil
}

type treeEntry struct {
	name     string
	isDir    bool
	children []treeEntry
}

func (t *tool) executeTree(absPath string, maxDepth, limit int, sb sdk.Sandboxer, ignorePatterns []string) (sdk.ToolResult, error) {
	rootName := filepath.Base(absPath)
	if rootName == "." || rootName == "" {
		rootName = absPath
	}

	children, _, err := buildTreeEntries(absPath, 1, maxDepth, sb, ignorePatterns)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if len(children) == 0 {
		return sdk.ToolResult{Content: rootName + "/\n(empty directory)", IsError: false}, nil
	}

	lines := []string{rootName + "/"}
	lines = append(lines, renderTreeEntries(children, nil)...)

	truncated := false

	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
		truncated = true
	}

	output := strings.Join(lines, "\n")
	result := truncate.Truncate(output, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	content := result.Format()

	if truncated {
		content += "\n\n... (more entries not shown — use a higher limit to see all)"
	}

	return sdk.ToolResult{Content: content, IsError: false}, nil
}

func buildTreeEntries(dir string, currentDepth, maxDepth int, sb sdk.Sandboxer, ignorePatterns []string) ([]treeEntry, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, fmt.Errorf("read directory %s: %w", dir, err)
	}

	var result []treeEntry

	totalCount := 0

	for _, e := range entries {
		name := e.Name()
		fullPath := filepath.Join(dir, name)

		if sb != nil && !sb.AllowRead(fullPath) {
			continue
		}

		if matchesAnyIgnore(name, ignorePatterns) {
			continue
		}

		entry := treeEntry{name: name, isDir: e.IsDir()}
		totalCount++

		if e.IsDir() && currentDepth < maxDepth {
			children, childCount, err := buildTreeEntries(fullPath, currentDepth+1, maxDepth, sb, ignorePatterns)
			if err != nil {
				return nil, 0, err
			}

			entry.children = children
			totalCount += childCount
		}

		result = append(result, entry)
	}

	sort.Slice(result, func(i, j int) bool {
		// Directories come before files at the same level
		if result[i].isDir != result[j].isDir {
			return result[i].isDir
		}

		return strings.ToLower(result[i].name) < strings.ToLower(result[j].name)
	})

	return result, totalCount, nil
}

func renderTreeEntries(entries []treeEntry, parentLast []bool) []string {
	var lines []string

	for i, e := range entries {
		isLast := i == len(entries)-1

		line := buildPrefix(parentLast, isLast)
		if e.isDir {
			line += e.name + "/"
		} else {
			line += e.name
		}

		lines = append(lines, line)

		if len(e.children) > 0 {
			childLast := append(append([]bool(nil), parentLast...), isLast)
			lines = append(lines, renderTreeEntries(e.children, childLast)...)
		}
	}

	return lines
}

func buildPrefix(parentLast []bool, isLast bool) string {
	var b strings.Builder

	for _, last := range parentLast {
		if last {
			b.WriteString("    ")
		} else {
			b.WriteString("│   ")
		}
	}

	if isLast {
		b.WriteString("└── ")
	} else {
		b.WriteString("├── ")
	}

	return b.String()
}

func resolveIgnorePatterns(args map[string]any) []string {
	var patterns []string

	if v, ok := args[paramIgnore]; ok {
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					patterns = append(patterns, s)
				}
			}
		}
	}

	return patterns
}

func resolveLimit(args map[string]any, defaultLimit int) int {
	if v, ok := args[paramLimit]; ok {
		if f, ok := v.(float64); ok && f >= 0 {
			return int(f)
		}

		if i, ok := v.(int); ok && i >= 0 {
			return i
		}
	}

	return defaultLimit
}

func resolveDepth(args map[string]any) int {
	if v, ok := args[paramDepth]; ok {
		if f, ok := v.(float64); ok && f >= 0 {
			return int(f)
		}

		if i, ok := v.(int); ok && i >= 0 {
			return i
		}
	}

	return 0
}

func matchesAnyIgnore(name string, patterns []string) bool {
	for _, pat := range patterns {
		matched, err := filepath.Match(pat, name)
		if err == nil && matched {
			return true
		}
	}

	return false
}
