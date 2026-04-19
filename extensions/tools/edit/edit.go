package edit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"weave/internal/truncate"
	"weave/sdk"

	"github.com/pmezard/go-difflib/difflib"
)

type tool struct{}

func init() {
	sdk.RegisterTool("edit", func(_ sdk.Config) (sdk.Tool, error) {
		return &tool{}, nil
	})
}

func (t *tool) Name() string { return "edit" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "edit",
		Description: "Apply text replacements to a file and return a unified diff of the changes.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to edit.",
				},
				"edits": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"oldText": map[string]any{
								"type":        "string",
								"description": "The text to find. Empty means create a new file.",
							},
							"newText": map[string]any{
								"type":        "string",
								"description": "The text to replace with.",
							},
						},
						"required": []string{"oldText", "newText"},
					},
					"description": "List of text replacements to apply in order.",
				},
			},
			"required": []string{"path", "edits"},
		},
	}
}

func (t *tool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return sdk.ToolResult{Content: "error: path is required", IsError: true}, nil
	}

	editsRaw, ok := args["edits"].([]any)
	if !ok || len(editsRaw) == 0 {
		return sdk.ToolResult{Content: "error: at least one edit is required", IsError: true}, nil
	}

	edits := make([]struct{ oldText, newText string }, 0, len(editsRaw))
	for i, e := range editsRaw {
		m, ok := e.(map[string]any)
		if !ok {
			return sdk.ToolResult{Content: fmt.Sprintf("error: edit %d is not an object", i), IsError: true}, nil
		}
		oldText, _ := m["oldText"].(string)
		newText, _ := m["newText"].(string)
		edits = append(edits, struct{ oldText, newText string }{oldText, newText})
	}

	original, _ := os.ReadFile(path)
	content := string(original)

	for i, e := range edits {
		if e.oldText == "" {
			if content != "" {
				return sdk.ToolResult{
					Content: fmt.Sprintf("error: empty oldText but file has content (edit %d)", i),
					IsError: true,
				}, nil
			}
			content = e.newText
			continue
		}
		if !strings.Contains(content, e.oldText) {
			return sdk.ToolResult{
				Content: fmt.Sprintf("error: oldText not found in file (edit %d)", i),
				IsError: true,
			}, nil
		}
		content = strings.Replace(content, e.oldText, e.newText, 1)
	}

	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(original)),
		B:        difflib.SplitLines(content),
		FromFile: "a" + path,
		ToFile:   "b" + path,
		Context:  3,
	})

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	result := truncate.Truncate(diff, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	output := result.Content
	if result.Truncated {
		output = fmt.Sprintf("%s\n[output truncated: %d lines, %d bytes]", output, result.Lines, result.Bytes)
	}

	return sdk.ToolResult{Content: output, IsError: false}, nil
}
