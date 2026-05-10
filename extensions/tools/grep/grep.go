package grep

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"weave/sdk"
	"weave/utils/ripgrep"
	"weave/utils/truncate"
)

const (
	ParamPattern = "pattern"
	paramPath    = "path"
	paramInclude = "include"
	jsonType     = "type"
	maxLineLen   = 500
)

type tool struct {
	cfg sdk.Config
}

func init() {
	sdk.RegisterTool("grep", func(cfg sdk.Config) (sdk.Tool, error) {
		return &tool{cfg: cfg}, nil
	})
}

func (t *tool) Name() string { return "grep" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "grep",
		Description: "Search files for a pattern using regular expressions. Uses ripgrep when available for .gitignore support and faster searches; falls back to pure Go when rg is absent. Returns matching file:line:content entries.",
		Parameters: map[string]any{
			jsonType: "object",
			"properties": map[string]any{
				ParamPattern: map[string]any{
					jsonType:      "string",
					"description": "The regular expression pattern to search for.",
				},
				paramPath: map[string]any{
					jsonType:      "string",
					"description": "File or directory to search. Defaults to current directory.",
				},
				paramInclude: map[string]any{
					jsonType:      "string",
					"description": "Glob filter to limit search to matching files (e.g. \"*.go\", \"*.{ts,tsx}\").",
				},
				"ignoreCase": map[string]any{
					jsonType:      "boolean",
					"description": "Case-insensitive matching. Defaults to false.",
				},
				"literal": map[string]any{
					jsonType:      "boolean",
					"description": "Treat pattern as a literal string instead of regex. Defaults to false.",
				},
				"context": map[string]any{
					jsonType:      "number",
					"description": "Number of context lines before and after each match. Defaults to 0.",
				},
			},
			"required": []string{ParamPattern},
		},
	}
}

func (t *tool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	pattern, _ := args[ParamPattern].(string)
	if pattern == "" {
		return sdk.ToolResult{Content: "error: pattern is required", IsError: true}, nil
	}

	path, _ := args[paramPath].(string)
	if path == "" {
		path = "."
	}

	include, _ := args[paramInclude].(string)
	ignoreCase, _ := args["ignoreCase"].(bool)
	literal, _ := args["literal"].(bool)

	contextLines := parseContextLines(args)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if s := sdk.GetSandboxer(); s != nil && !s.AllowRead(absPath) {
		return sdk.ToolResult{Content: "sandbox: read denied — path is protected", IsError: true}, nil
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	// Validate regex early so both rg and stdlib paths get consistent error handling
	expr := pattern
	if literal {
		expr = regexp.QuoteMeta(pattern)
	}

	if ignoreCase {
		expr = "(?i)" + expr
	}

	if _, compileErr := regexp.Compile(expr); compileErr != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: invalid pattern: %s", compileErr), IsError: true}, nil
	}

	respectGitignore := true
	if t.cfg != nil {
		respectGitignore = t.cfg.RespectGitignore()
	}

	matches := t.search(ctx, absPath, info.IsDir(), pattern, include, ignoreCase, literal, contextLines, respectGitignore)
	if len(matches) == 0 {
		return sdk.ToolResult{Content: "no matches found", IsError: false}, nil
	}

	for i, m := range matches {
		matches[i] = truncateLine(m)
	}

	output := strings.Join(matches, "\n")
	result := truncate.Truncate(output, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	return sdk.ToolResult{Content: result.Format(), IsError: false}, nil
}

func parseContextLines(args map[string]any) int {
	if v, ok := args["context"]; ok {
		if f, ok := v.(float64); ok && f >= 0 {
			return min(int(f), 50)
		}
	}

	return 0
}

// search tries rg first, then falls back to stdlib.
func (t *tool) search(ctx context.Context, absPath string, isDir bool, pattern, include string, ignoreCase, literal bool, contextLines int, respectGitignore bool) []string {
	if rgPath := ripgrep.Find(); rgPath != "" {
		matches, err := searchWithRipgrep(ctx, rgPath, absPath, isDir, pattern, include, ignoreCase, literal, contextLines, respectGitignore)
		if err == nil {
			return matches
		}
	}

	return searchWithStdlib(absPath, isDir, pattern, include, ignoreCase, literal, contextLines)
}

func searchWithStdlib(absPath string, isDir bool, pattern, include string, ignoreCase, literal bool, contextLines int) []string {
	expr := pattern
	if literal {
		expr = regexp.QuoteMeta(pattern)
	}

	if ignoreCase {
		expr = "(?i)" + expr
	}

	re, err := regexp.Compile(expr)
	if err != nil {
		return nil
	}

	if isDir {
		matches, dirErr := searchDir(absPath, re, contextLines, include)
		if dirErr != nil {
			return nil
		}

		return matches
	}

	return searchFile(absPath, re, contextLines)
}

func searchWithRipgrep(ctx context.Context, rgPath, absPath string, isDir bool, pattern, include string, ignoreCase, literal bool, contextLines int, respectGitignore bool) ([]string, error) {
	args := []string{"--json", "-H", "-n"}

	if ignoreCase {
		args = append(args, "-i")
	}

	if literal {
		args = append(args, "-F")
	}

	if include != "" {
		args = append(args, "--glob", include)
	}

	if !respectGitignore {
		args = append(args, "--no-ignore")
	}

	if contextLines > 0 {
		args = append(args, "-C", strconv.Itoa(contextLines))
	}

	args = append(args, "--", pattern)

	searchPath := absPath
	if !isDir {
		searchPath = filepath.Dir(absPath)
		args = append(args, filepath.Base(absPath))
	} else {
		args = append(args, ".")
	}

	cmd := exec.CommandContext(ctx, rgPath, args...)
	cmd.Dir = searchPath

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}

		return nil, fmt.Errorf("rg: %w", err)
	}

	return parseRgJSON(out, searchPath)
}

func parseRgJSON(data []byte, baseDir string) ([]string, error) {
	var matches []string

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				LineNumber int `json:"line_number"`
				Lines      struct {
					Text string `json:"text"`
				} `json:"lines"`
			} `json:"data"`
		}

		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type != "match" && entry.Type != "context" {
			continue
		}

		content := strings.TrimRight(entry.Data.Lines.Text, "\n\r")

		relPath := entry.Data.Path.Text

		if rel, relErr := filepath.Rel(baseDir, entry.Data.Path.Text); relErr == nil {
			relPath = rel
		}

		matches = append(matches, fmt.Sprintf("%s:%d:%s", relPath, entry.Data.LineNumber, content))
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("parsing rg output: %w", scanErr)
	}

	return matches, nil
}

func searchDir(root string, re *regexp.Regexp, contextLines int, include string) ([]string, error) {
	var matches []string

	err := filepath.WalkDir(root, func(walkPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // walkErr intentionally swallowed to skip inaccessible entries
		}

		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".hg" || name == ".svn" {
				return filepath.SkipDir
			}

			return nil
		}

		if !fileMatchesInclude(include, d.Name()) {
			return nil
		}

		if s := sdk.GetSandboxer(); s != nil && !s.AllowRead(walkPath) {
			return nil
		}

		fileMatches := searchFile(walkPath, re, contextLines)
		matches = append(matches, fileMatches...)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("grep: walk directory: %w", err)
	}

	return matches, nil
}

func fileMatchesInclude(include, name string) bool {
	if include == "" {
		return true
	}

	matched, err := filepath.Match(include, name)

	return err == nil && matched
}

func searchFile(path string, re *regexp.Regexp, contextLines int) []string {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > 10*1024*1024 {
		return nil
	}

	if isBinaryFile(path) {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

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

	for i := range lines {
		if matched[i] {
			results = append(results, fmt.Sprintf("%s:%d:%s", path, i+1, lines[i]))
		}
	}

	return results
}

func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()

	buf := make([]byte, 512)

	n, err := f.Read(buf)
	if err != nil {
		return true
	}

	contentType := http.DetectContentType(buf[:n])

	return !strings.HasPrefix(contentType, "text/")
}

func truncateLine(line string) string {
	if len(line) <= maxLineLen {
		return line
	}

	return line[:maxLineLen] + fmt.Sprintf("... [%d chars truncated]", len(line)-maxLineLen)
}
