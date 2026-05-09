package find

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"weave/sdk"
	"weave/utils/truncate"
)

// ParamPattern is the tool parameter name for the glob pattern.
const ParamPattern = "pattern"

type tool struct{}

func init() {
	sdk.RegisterTool("find", func(_ sdk.Config) (sdk.Tool, error) {
		return &tool{}, nil
	})
}

func (t *tool) Name() string { return "find" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "find",
		Description: "Find files and directories matching a glob pattern. Walks directory tree, skipping common ignored directories (.git, node_modules).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				ParamPattern: map[string]any{
					"type":        "string",
					"description": "Glob pattern to match against file names (e.g. \"*.go\", \"config.yaml\").",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Directory to search in. Defaults to current directory.",
				},
			},
			"required": []string{ParamPattern},
		},
	}
}

func matchName(pattern, name, rel string) bool {
	matched, _ := filepath.Match(pattern, name)
	if !matched {
		matched, _ = filepath.Match(pattern, rel)
	}

	return matched
}

func (t *tool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	pattern, _ := args[ParamPattern].(string)
	if pattern == "" {
		return sdk.ToolResult{Content: "error: pattern is required", IsError: true}, nil
	}

	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

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

	if !info.IsDir() {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s is not a directory", absPath), IsError: true}, nil
	}

	if _, validateErr := filepath.Match(pattern, ""); validateErr != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: invalid pattern: %s", validateErr), IsError: true}, nil
	}

	var matches []string

	err = filepath.WalkDir(absPath, func(walkPath string, d fs.DirEntry, walkErr error) error {
		//nolint:nilerr // walkErr/relErr are intentionally swallowed to skip inaccessible paths
		if walkErr != nil {
			return nil
		}

		rel, relErr := filepath.Rel(absPath, walkPath)
		if relErr != nil {
			return nil //nolint:nilerr // relErr intentionally swallowed to skip inaccessible paths
		}

		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".hg" || name == ".svn" {
				return filepath.SkipDir
			}

			if rel != "." && matchName(pattern, name, rel) {
				matches = append(matches, rel)
			}

			return nil
		}

		if matchName(pattern, d.Name(), rel) {
			matches = append(matches, rel)
		}

		return nil
	})
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if len(matches) == 0 {
		return sdk.ToolResult{Content: "no files found", IsError: false}, nil
	}

	output := strings.Join(matches, "\n")
	result := truncate.Truncate(output, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	return sdk.ToolResult{Content: result.Format(), IsError: false}, nil
}
