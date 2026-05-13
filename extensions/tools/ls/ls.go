package ls

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"weave/sdk"
	"weave/utils/truncate"
)

const (
	paramPath   = "path"
	paramLimit  = "limit"
	paramIgnore = "ignore"
)

// LSConfig holds per-tool settings for the ls tool.
type LSConfig struct {
	Limit int `json:"limit" default:"500" env:"LIMIT"`
}

type tool struct {
	defaultLimit int
}

func init() {
	sdk.RegisterTool("ls", func(_ sdk.Config, cfg LSConfig) (sdk.Tool, error) {
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
		Description: "List directory contents. Returns file and directory names with type indicators.",
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
			},
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

	if s := sdk.GetSandboxer(); s != nil {
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

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if len(entries) == 0 {
		return sdk.ToolResult{Content: "(empty directory)", IsError: false}, nil
	}

	sb := sdk.GetSandboxer()
	ignorePatterns := resolveIgnorePatterns(args)
	limit := resolveLimit(args, t.defaultLimit)

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

	filteredCount := len(lines)
	truncated := false

	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
		truncated = true
	}

	output := strings.Join(lines, "\n")
	result := truncate.Truncate(output, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	content := result.Format()

	if truncated {
		content += fmt.Sprintf("\n\n... (%d more entries not shown — use a higher limit to see all)", filteredCount-limit)
	}

	return sdk.ToolResult{Content: content, IsError: false}, nil
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
		switch n := v.(type) {
		case float64:
			if n >= 0 {
				return int(n)
			}
		case int:
			if n >= 0 {
				return n
			}
		case int64:
			if n >= 0 {
				return int(n)
			}
		}
	}

	return defaultLimit
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
