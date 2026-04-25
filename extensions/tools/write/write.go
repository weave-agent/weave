package write

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"weave/sdk"
)

type tool struct{}

func init() {
	sdk.RegisterTool("write", func(_ sdk.Config) (sdk.Tool, error) {
		return &tool{}, nil
	})
}

func (t *tool) Name() string { return "write" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "write",
		Description: "Write content to a file, creating parent directories if needed.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The file path to write to.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The content to write.",
				},
			},
			"required": []string{"path", "content"},
		},
	}
}

func (t *tool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return sdk.ToolResult{Content: "error: path is required", IsError: true}, nil
	}

	content, _ := args["content"].(string)

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0755 is intentional for created directories
			return sdk.ToolResult{Content: fmt.Sprintf("error: creating directories: %s", err), IsError: true}, nil
		}
	}

	perm := fs.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
	}

	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	return sdk.ToolResult{
		Content: fmt.Sprintf("wrote %d bytes to %s", len(content), path),
	}, nil
}
