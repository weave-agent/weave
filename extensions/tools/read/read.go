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

	"weave/internal/pathutil"
	"weave/sdk"
	"weave/utils/truncate"
)

// maxLineContentBytes caps raw line content so the formatted line (with line
// number prefix and optional truncation suffix) stays under truncate.DefaultMaxBytes.
const maxLineContentBytes = truncate.DefaultMaxBytes - 100

// ParamPath is the tool parameter name for the file path.
const ParamPath = "path"

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
	sdk.RegisterTool[struct{}]("read", func(_ sdk.Config, _ struct{}) (sdk.Tool, error) {
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
				ParamPath: map[string]any{
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
			"required": []string{ParamPath},
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

func (t *tool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	path, _ := args[ParamPath].(string)
	if path == "" {
		return sdk.ToolResult{Content: "error: path is required", IsError: true}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		normalized := pathutil.NormalizePath(path)
		if normalized != path {
			info, err = os.Stat(normalized)
			if err == nil {
				path = normalized
			}
		}

		if err != nil {
			return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
		}
	}

	if s := sdk.GetSandboxer(); s != nil && !s.AllowRead(path) {
		return sdk.ToolResult{Content: "sandbox: read denied — path is protected", IsError: true}, nil
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

	lines, err := readLines(reader, offset, limit)
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	content := strings.Join(lines, "\n")
	result := truncate.Truncate(content, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	if bus := sdk.BusFromContext(ctx); bus != nil {
		bus.Publish(sdk.NewEvent("tool.read.done", sdk.ReadDonePayload{
			Path:    path,
			ModTime: info.ModTime(),
		}))
	}

	// Record read synchronously to avoid a race where a back-to-back edit
	// checks the tracker before the async bus handler has processed the event.
	if tracker := sdk.GetFileTracker(); tracker != nil {
		tracker.RecordRead(path, info.ModTime())
	}

	return sdk.ToolResult{Content: result.Format(), IsError: false}, nil
}

// readLines reads formatted lines from r with the given offset and limit.
func readLines(r *bufio.Reader, offset, limit int) ([]string, error) {
	var lines []string

	lineNum := 0
	collected := 0

	for {
		line, lineTruncated, readErr := readLine(r, maxLineContentBytes)

		if errors.Is(readErr, io.EOF) && line == "" {
			break
		}

		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, readErr
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

	return lines, nil
}
