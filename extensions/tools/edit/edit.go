package edit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"weave/sdk"
	"weave/utils/truncate"

	"github.com/pmezard/go-difflib/difflib"
)

// Parameter name constants.
const (
	ParamEdits      = "edits"
	ParamOldText    = "oldText"
	ParamNewText    = "newText"
	ParamReplaceAll = "replace_all"
)

type tool struct {
	tracker   sdk.FileTracker
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

	sdk.RegisterTool("edit", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Tool, error) {
		return &tool{
			tracker:   sdk.GetFileTracker(),
			fileMutex: sdk.GetFileMutex(),
		}, nil
	})
}

func (t *tool) Name() string { return "edit" }

//nolint:goconst // JSON schema "type" keys are intentionally repeated literals.
func (t *tool) Definition() sdk.ToolDef {
	return sdk.ToolDef{
		Name:        "edit",
		Description: "Apply text replacements to a file and return a unified diff of the changes. Set replace_all=true to replace every occurrence of oldText.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{ //nolint:goconst // JSON parameter name, not a magic constant
					"type":        "string",
					"description": "The absolute path to the file to edit.",
				},
				ParamEdits: map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							ParamOldText: map[string]any{
								"type":        "string",
								"description": "The text to find. Empty means create a new file.",
							},
							ParamNewText: map[string]any{
								"type":        "string",
								"description": "The text to replace with.",
							},
							ParamReplaceAll: map[string]any{
								"type":        "boolean",
								"description": "If true, replace every occurrence of oldText in the file. If false (default), oldText must match exactly once.",
							},
						},
						"required": []string{ParamOldText, ParamNewText},
					},
					"description": "List of text replacements to apply in order.",
				},
			},
			"required": []string{"path", ParamEdits},
		},
	}
}

func (t *tool) Execute(_ context.Context, args map[string]any) (sdk.ToolResult, error) {
	path, _ := args["path"].(string)
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

	editsRaw, ok := args[ParamEdits].([]any)
	if !ok || len(editsRaw) == 0 {
		return sdk.ToolResult{Content: "error: at least one edit is required", IsError: true}, nil
	}

	edits, err := parseEdits(editsRaw)
	if err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	originalBytes, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if err == nil {
		if result, shouldReturn := t.checkFileTracker(path); shouldReturn {
			return result, nil
		}
	}

	normalizedBytes, ending := normalizeToLF(originalBytes)
	content := string(normalizedBytes)

	// Normalize edit parameters to LF for consistent matching.
	for i := range edits {
		edits[i].oldText = strings.ReplaceAll(edits[i].oldText, "\r\n", "\n")
		edits[i].newText = strings.ReplaceAll(edits[i].newText, "\r\n", "\n")
	}

	content, err = applyEdits(content, edits)
	if err != nil {
		return sdk.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	finalBytes := restoreLineEndings([]byte(content), ending)

	if bytes.Equal(finalBytes, originalBytes) {
		return sdk.ToolResult{Content: "no changes made (content is identical)", IsError: false}, nil
	}

	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(originalBytes)),
		B:        difflib.SplitLines(string(finalBytes)),
		FromFile: "a" + path,
		ToFile:   "b" + path,
		Context:  3,
	})
	if err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: generating diff: %s", err), IsError: true}, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: 0755 is intentional for created directories
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	if err := writeFilePreservingOwnership(path, finalBytes); err != nil {
		return sdk.ToolResult{Content: fmt.Sprintf("error: %s", err), IsError: true}, nil
	}

	result := truncate.Truncate(diff, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)

	return sdk.ToolResult{Content: result.Format(), IsError: false}, nil
}

// editEntry is a single text replacement instruction.
type editEntry struct {
	oldText, newText string
	replaceAll       bool
}

// applyEdits applies a sequence of text replacements to content and returns the result.
// It returns an error if any edit cannot be applied (not found, multiple matches when not allowed, etc.).
func applyEdits(content string, edits []editEntry) (string, error) {
	for i, e := range edits {
		if e.oldText == "" {
			if content != "" {
				return "", fmt.Errorf("error: empty oldText but file has content (edit %d)", i)
			}

			content = e.newText

			continue
		}

		if !strings.Contains(content, e.oldText) {
			return "", fmt.Errorf("error: oldText not found in file (edit %d)", i)
		}

		if e.replaceAll {
			content = strings.ReplaceAll(content, e.oldText, e.newText)
		} else {
			count := strings.Count(content, e.oldText)
			if count > 1 {
				return "", fmt.Errorf("error: oldText matched %d times in file, expected exactly 1 (edit %d)", count, i)
			}

			content = strings.Replace(content, e.oldText, e.newText, 1)
		}
	}

	return content, nil
}

// parseEdits converts raw edit parameters into typed editEntry values.
func parseEdits(editsRaw []any) ([]editEntry, error) {
	edits := make([]editEntry, 0, len(editsRaw))

	for i, e := range editsRaw {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("error: edit %d is not an object", i)
		}

		oldText, oldOk := m[ParamOldText].(string)
		newText, newOk := m[ParamNewText].(string)

		if !oldOk {
			return nil, fmt.Errorf("error: edit %d oldText must be a string", i)
		}

		if !newOk {
			return nil, fmt.Errorf("error: edit %d newText must be a string", i)
		}

		replaceAll, _ := m[ParamReplaceAll].(bool)
		edits = append(edits, editEntry{oldText, newText, replaceAll})
	}

	return edits, nil
}

// writeFilePreservingOwnership writes data to path. For existing files it uses
// O_WRONLY|O_TRUNC to preserve Unix ownership; for new files it uses os.WriteFile.
//
//nolint:gosec,wrapcheck // G703: path is a tool parameter; errors pass through directly
func writeFilePreservingOwnership(path string, data []byte) error {
	perm := os.FileMode(0o644)
	fileExists := false

	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
		fileExists = true
	}

	if fileExists {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, perm)
		if err != nil {
			return err
		}

		_, writeErr := f.Write(data)
		closeErr := f.Close()

		if writeErr != nil {
			return writeErr
		}

		if closeErr != nil {
			return closeErr
		}

		return nil
	}

	return os.WriteFile(path, data, perm)
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

// checkFileTracker validates that the file at path has been read and not modified since.
// It returns (zero result, false) when checks pass or no tracker is configured.
// It returns (error result, true) when the file fails the read-before-edit policy.
func (t *tool) checkFileTracker(path string) (sdk.ToolResult, bool) {
	if t.tracker == nil {
		return sdk.ToolResult{}, false
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sdk.ToolResult{}, false
		}

		return sdk.ToolResult{
			Content: "error: cannot stat file: " + err.Error(),
			IsError: true,
		}, true
	}

	if !t.tracker.WasRead(path) {
		return sdk.ToolResult{
			Content: "error: file must be read before editing: " + path,
			IsError: true,
		}, true
	}

	readTime, ok := t.tracker.GetReadTime(path)
	if ok && !info.ModTime().Equal(readTime) {
		return sdk.ToolResult{
			Content: "error: file was modified externally since last read (" +
				info.ModTime().Format(time.RFC3339) + " > " + readTime.Format(time.RFC3339) +
				"), please re-read before editing: " + path,
			IsError: true,
		}, true
	}

	return sdk.ToolResult{}, false
}
