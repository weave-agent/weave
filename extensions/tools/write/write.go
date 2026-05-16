package write

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

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

var (
	sandboxerMu sync.RWMutex
	sandboxer   sdk.Sandboxer
)

func setSandboxer(s sdk.Sandboxer) {
	sandboxerMu.Lock()
	sandboxer = s
	sandboxerMu.Unlock()
}

func getSandboxer() sdk.Sandboxer {
	sandboxerMu.RLock()

	s := sandboxer

	sandboxerMu.RUnlock()

	return s
}

func init() {
	sdk.OnBusReady(func(bus sdk.Bus) {
		bus.On("sandbox.registered", func(ev sdk.Event) error {
			if s, ok := ev.Payload.(sdk.Sandboxer); ok {
				setSandboxer(s)
			}

			return nil
		})
	})

	sdk.RegisterTool[struct{}]("write", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Tool, error) {
		return &tool{fileMutex: sdk.GetFileMutex()}, nil
	})
}

// normalizePath applies macOS path normalization and falls back to the original
// if the normalized path does not exist.
func normalizePath(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}

	normalized := normalizeMacOSPath(path)
	if normalized != path {
		if _, err := os.Stat(normalized); err == nil {
			return normalized
		}
	}

	return path
}

// readExistingForWrite reads an existing file to detect no-op writes and to
// capture its permissions. It records the read in the global FileTracker so
// that subsequent edits are allowed. Returns the file's permission bits and
// true when the existing content matches the desired content (no-op).
func (t *tool) readExistingForWrite(path, content string) (fs.FileMode, bool) {
	perm := fs.FileMode(0o644)

	f, err := os.Open(path)
	if err != nil {
		return perm, false
	}
	defer f.Close()

	var modTime time.Time

	if info, statErr := f.Stat(); statErr == nil {
		perm = info.Mode().Perm()
		modTime = info.ModTime()
	}

	existing, readErr := io.ReadAll(f)
	if readErr != nil {
		return perm, false
	}

	if ft := sdk.GetFileTracker(); ft != nil && !modTime.IsZero() {
		ft.RecordRead(path, modTime)
	}

	return perm, string(existing) == content
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
			"required":             []string{ParamPath, ParamContent},
			"additionalProperties": false,
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

	if s := getSandboxer(); s != nil && !s.AllowWrite(path) {
		return sdk.ToolResult{Content: "sandbox: write denied — path is protected", IsError: true}, nil
	}

	content, _ := args[ParamContent].(string)

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0755 is intentional for created directories
			return sdk.ToolResult{Content: fmt.Sprintf("error: creating directories: %s", err), IsError: true}, nil
		}
	}

	perm, isNoOp := t.readExistingForWrite(path, content)
	if isNoOp {
		return sdk.ToolResult{
			Content: fmt.Sprintf("file %s already contains the exact content, no changes made", path),
		}, nil
	}

	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	return sdk.ToolResult{
		Content: fmt.Sprintf("wrote %d bytes to %s", len(content), path),
	}, nil
}
