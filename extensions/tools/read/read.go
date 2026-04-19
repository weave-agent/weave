package read

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"weave/internal/truncate"
	"weave/sdk"
)

type tool struct{}

func init() {
	sdk.RegisterTool("read", func(_ sdk.Config) (sdk.Tool, error) {
		return &tool{}, nil
	})
}

func (t *tool) Name() string { return "read" }

func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "read",
		Description: "Read the contents of a file with optional line-based pagination.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to read.",
				},
				"offset": map[string]any{
					"type":        "number",
					"description": "The line number to start reading from (1-based). Defaults to 1.",
				},
				"limit": map[string]any{
					"type":        "number",
					"description": "Maximum number of lines to read. Defaults to all lines.",
				},
			},
			"required": []string{"path"},
		},
	}
}

func (t *tool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return sdk.ToolResult{Content: "error: path is required", IsError: true}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}
	if info.IsDir() {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s is a directory", path), IsError: true}, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}
	defer f.Close()

	offset := 1
	if v, ok := args["offset"]; ok {
		if f, ok := v.(float64); ok && f >= 1 {
			offset = int(f)
		}
	}

	limit := 0
	if v, ok := args["limit"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			limit = int(f)
		}
	}

	scanner := bufio.NewScanner(f)
	var lines []string
	lineNum := 0
	collected := 0

	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		lines = append(lines, strconv.Itoa(lineNum)+"\t"+scanner.Text())
		collected++
		if limit > 0 && collected >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	content := strings.Join(lines, "\n")
	result := truncate.Truncate(content, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	output := result.Content
	if result.Truncated {
		output = fmt.Sprintf("%s\n[output truncated: %d lines, %d bytes]", output, result.Lines, result.Bytes)
	}

	return sdk.ToolResult{Content: output, IsError: false}, nil
}
