package write

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"weave/internal/pathutil"
	"weave/sdk"
)

// Parameter name constants.
const (
	ParamPath    = "path"
	ParamContent = "content"
)

type tool struct {
	fileMutex sdk.FileMuter
}

func init() {
	sdk.RegisterTool[struct{}]("write", func(_ sdk.Config, _ struct{}) (sdk.Tool, error) {
		return &tool{fileMutex: sdk.GetFileMutex()}, nil
	})
}

// normalizePath applies macOS path normalization and falls back to the original
// if the normalized path does not exist.
func normalizePath(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}

	normalized := pathutil.NormalizePath(path)
	if normalized != path {
		if _, err := os.Stat(normalized); err == nil {
			return normalized
		}
	}

	return path
}

func (t *tool) Name() string { return "write" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "write",
		Description: "Write content to a file, creating parent directories if needed.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				ParamPath: map[string]any{
					"type":        "string",
					"description": "The file path to write to.",
				},
				ParamContent: map[string]any{
					"type":        "string",
					"description": "The content to write.",
				},
			},
			"required": []string{ParamPath, ParamContent},
		},
	}
}

func (t *tool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	path, _ := args[ParamPath].(string)
	if path == "" {
		return sdk.ToolResult{Content: "error: path is required", IsError: true}, nil
	}

	path = normalizePath(path)

	if t.fileMutex != nil {
		defer t.fileMutex.Lock(path)()
	}

	if s := sdk.GetSandboxer(); s != nil && !s.AllowWrite(path) {
		return sdk.ToolResult{Content: "sandbox: write denied — path is protected", IsError: true}, nil
	}

	content, _ := args[ParamContent].(string)

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0755 is intentional for created directories
			return sdk.ToolResult{Content: fmt.Sprintf("error: creating directories: %s", err), IsError: true}, nil
		}
	}

	perm := fs.FileMode(0o644)

	// No-op detection: skip write if content is identical.
	// Read first to avoid a race between stat and read.
	existing, readErr := os.ReadFile(path)
	if readErr == nil {
		if string(existing) == content {
			return sdk.ToolResult{
				Content: fmt.Sprintf("file %s already contains the exact content, no changes made", path),
			}, nil
		}

		if info, statErr := os.Stat(path); statErr == nil {
			perm = info.Mode().Perm()
		}
	}

	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	return sdk.ToolResult{
		Content: fmt.Sprintf("wrote %d bytes to %s", len(content), path),
	}, nil
}
