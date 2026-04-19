package grep

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"weave/internal/truncate"
	"weave/sdk"
)

type tool struct{}

func init() {
	sdk.RegisterTool("grep", func(_ sdk.Config) (sdk.Tool, error) {
		return &tool{}, nil
	})
}

func (t *tool) Name() string { return "grep" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "grep",
		Description: "Search files for a pattern using regular expressions. Returns matching lines with optional context.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The regular expression pattern to search for.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "File or directory to search. Defaults to current directory.",
				},
				"ignoreCase": map[string]any{
					"type":        "boolean",
					"description": "Case-insensitive matching. Defaults to false.",
				},
				"literal": map[string]any{
					"type":        "boolean",
					"description": "Treat pattern as a literal string instead of regex. Defaults to false.",
				},
				"context": map[string]any{
					"type":        "number",
					"description": "Number of context lines before and after each match. Defaults to 0.",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

func (t *tool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return sdk.ToolResult{Content: "error: pattern is required", IsError: true}, nil
	}

	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	ignoreCase, _ := args["ignoreCase"].(bool)
	literal, _ := args["literal"].(bool)

	var contextLines int
	if v, ok := args["context"]; ok {
		if f, ok := v.(float64); ok && f >= 0 {
			contextLines = min(int(f), 50)
		}
	}

	expr := pattern
	if literal {
		expr = regexp.QuoteMeta(pattern)
	}
	if ignoreCase {
		expr = "(?i)" + expr
	}

	re, err := regexp.Compile(expr)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: invalid pattern: %s", err), IsError: true}, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	var matches []string

	if info.IsDir() {
		err = filepath.WalkDir(absPath, func(walkPath string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" || name == ".hg" || name == ".svn" {
					return filepath.SkipDir
				}
				return nil
			}
			fileMatches := searchFile(walkPath, re, contextLines)
			matches = append(matches, fileMatches...)
			return nil
		})
		if err != nil {
			return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
		}
	} else {
		matches = searchFile(absPath, re, contextLines)
	}

	if len(matches) == 0 {
		return sdk.ToolResult{Content: "no matches found", IsError: false}, nil
	}

	output := strings.Join(matches, "\n")
	result := truncate.Truncate(output, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	return sdk.ToolResult{Content: result.Format(), IsError: false}, nil
}

func searchFile(path string, re *regexp.Regexp, contextLines int) []string {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > 10*1024*1024 {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil
	}

	var results []string
	matched := make(map[int]bool)

	for i, line := range lines {
		if re.MatchString(line) {
			for j := max(0, i-contextLines); j <= min(len(lines)-1, i+contextLines); j++ {
				matched[j] = true
			}
		}
	}

	for i := range len(lines) {
		if matched[i] {
			results = append(results, fmt.Sprintf("%s:%d:%s", path, i+1, lines[i]))
		}
	}

	return results
}
