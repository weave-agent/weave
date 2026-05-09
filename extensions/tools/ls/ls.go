package ls

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"weave/sdk"
	"weave/utils/truncate"
)

type tool struct{}

func init() {
	sdk.RegisterTool("ls", func(_ sdk.Config) (sdk.Tool, error) {
		return &tool{}, nil
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
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path to list. Defaults to current directory.",
				},
			},
		},
	}
}

func (t *tool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	path, _ := args["path"].(string)
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

	info, err := os.Stat(path)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if !info.IsDir() {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s is not a directory", path), IsError: true}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if len(entries) == 0 {
		return sdk.ToolResult{Content: "(empty directory)", IsError: false}, nil
	}

	sb := sdk.GetSandboxer()

	var lines []string

	for _, e := range entries {
		if sb != nil && !sb.AllowRead(filepath.Join(absPath, e.Name())) {
			continue
		}

		if e.IsDir() {
			lines = append(lines, e.Name()+"/")
		} else {
			lines = append(lines, e.Name())
		}
	}

	output := strings.Join(lines, "\n")
	result := truncate.Truncate(output, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	return sdk.ToolResult{Content: result.Format(), IsError: false}, nil
}
