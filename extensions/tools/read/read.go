package read

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"weave/internal/truncate"
	"weave/sdk"
)

// maxLineContentBytes caps raw line content so the formatted line (with line
// number prefix and optional truncation suffix) stays under truncate.DefaultMaxBytes.
const maxLineContentBytes = truncate.DefaultMaxBytes - 100

type tool struct{}

// readLine reads one line from r, returning at most maxBytes of content.
// If the line exceeds maxBytes the excess is consumed but discarded and
// truncated is true.
func readLine(r *bufio.Reader, maxBytes int) (line string, truncated bool, err error) {
	var buf strings.Builder

	for {
		chunk, sliceErr := r.ReadSlice('\n')
		if !truncated && len(chunk) > 0 {
			if buf.Len()+len(chunk) > maxBytes {
				n := maxBytes - buf.Len()
				if n > 0 {
					buf.Write(chunk[:n])
				}

				truncated = true
			} else {
				buf.Write(chunk)
			}
		}

		if sliceErr == nil {
			return buf.String(), truncated, nil
		}

		if errors.Is(sliceErr, bufio.ErrBufferFull) {
			continue
		}

		return buf.String(), truncated, sliceErr
	}
}

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

func parsePagination(args map[string]any) (offset, limit int) {
	offset = 1

	if v, ok := args["offset"]; ok {
		if val, ok := v.(float64); ok && val >= 1 {
			offset = int(val)
		}
	}

	if v, ok := args["limit"]; ok {
		if val, ok := v.(float64); ok && val > 0 {
			limit = int(val)
		}
	}

	return offset, limit
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

	offset, limit := parsePagination(args)

	reader := bufio.NewReader(f)

	var lines []string

	lineNum := 0
	collected := 0

	for {
		line, lineTruncated, readErr := readLine(reader, maxLineContentBytes)

		if errors.Is(readErr, io.EOF) && line == "" {
			break
		}

		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return sdk.ToolResult{Content: fmt.Sprintf("error: %s", readErr), IsError: true}, nil
		}

		lineNum++
		if lineNum >= offset {
			line = strings.TrimRight(line, "\r\n")
			if lineTruncated {
				line += "\n[... line truncated]"
			}

			lines = append(lines, strconv.Itoa(lineNum)+"\t"+line)

			collected++
			if limit > 0 && collected >= limit {
				break
			}
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
	}

	content := strings.Join(lines, "\n")
	result := truncate.Truncate(content, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	return sdk.ToolResult{Content: result.Format(), IsError: false}, nil
}
