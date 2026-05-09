package edit

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	tool, err := sdk.GetTool("edit", nil)
	require.NoError(t, err)
	assert.Equal(t, "edit", tool.Name())
}

func TestDefinition(t *testing.T) {
	tool := &tool{}
	def := tool.Definition()
	assert.Equal(t, "edit", def.Name)
	assert.NotNil(t, def.Parameters)
}

func TestExecute(t *testing.T) {
	tool := &tool{}
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		setup     func(t *testing.T) string // returns file path
		args      map[string]any
		wantError bool
		check     func(t *testing.T, result sdk.ToolResult, path string)
	}{
		{
			name: "missing path",
			args: map[string]any{
				"path": "",
				"edits": []any{
					map[string]any{"oldText": "x", "newText": "y"},
				},
			},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult, _ string) {
				assert.Contains(t, result.Content, "path is required")
			},
		},
		{
			name: "missing edits",
			args: map[string]any{
				"path": filepath.Join(tmpDir, "test.txt"),
			},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult, _ string) {
				assert.Contains(t, result.Content, "at least one edit is required")
			},
		},
		{
			name: "single edit",
			setup: func(t *testing.T) string {
				p := filepath.Join(tmpDir, "single.txt")
				require.NoError(t, os.WriteFile(p, []byte("hello world"), 0o644))

				return p
			},
			args: map[string]any{
				"edits": []any{
					map[string]any{"oldText": "hello", "newText": "goodbye"},
				},
			},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult, path string) {
				assert.Contains(t, result.Content, "-hello")
				assert.Contains(t, result.Content, "+goodbye")

				data, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "goodbye world", string(data))
			},
		},
		{
			name: "multiple edits",
			setup: func(t *testing.T) string {
				p := filepath.Join(tmpDir, "multi.txt")
				require.NoError(t, os.WriteFile(p, []byte("foo bar baz"), 0o644))

				return p
			},
			args: map[string]any{
				"edits": []any{
					map[string]any{"oldText": "foo", "newText": "one"},
					map[string]any{"oldText": "baz", "newText": "three"},
				},
			},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult, path string) {
				assert.Contains(t, result.Content, "-foo bar baz")
				assert.Contains(t, result.Content, "+one bar three")

				data, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "one bar three", string(data))
			},
		},
		{
			name: "no match error",
			setup: func(t *testing.T) string {
				p := filepath.Join(tmpDir, "nomatch.txt")
				require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))

				return p
			},
			args: map[string]any{
				"edits": []any{
					map[string]any{"oldText": "not found", "newText": "x"},
				},
			},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult, _ string) {
				assert.Contains(t, result.Content, "oldText not found")
			},
		},
		{
			name: "empty file with replacement",
			setup: func(t *testing.T) string {
				p := filepath.Join(tmpDir, "empty.txt")
				require.NoError(t, os.WriteFile(p, []byte(""), 0o644))

				return p
			},
			args: map[string]any{
				"edits": []any{
					map[string]any{"oldText": "", "newText": "new content"},
				},
			},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "new content", string(data))
			},
		},
		{
			name: "create new file",
			setup: func(_ *testing.T) string {
				return filepath.Join(tmpDir, "newdir", "created.txt")
			},
			args: map[string]any{
				"edits": []any{
					map[string]any{"oldText": "", "newText": "fresh file"},
				},
			},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "fresh file", string(data))
			},
		},
		{
			name: "duplicate match error",
			setup: func(t *testing.T) string {
				p := filepath.Join(tmpDir, "dup.txt")
				require.NoError(t, os.WriteFile(p, []byte("aaa bbb aaa"), 0o644))

				return p
			},
			args: map[string]any{
				"edits": []any{
					map[string]any{"oldText": "aaa", "newText": "ccc"},
				},
			},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult, _ string) {
				assert.Contains(t, result.Content, "matched 2 times")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			if tt.setup != nil {
				path = tt.setup(t)
			} else {
				path = filepath.Join(tmpDir, "dummy.txt")
			}

			args := make(map[string]any, len(tt.args)+1)
			maps.Copy(args, tt.args)

			if _, hasPath := args["path"]; !hasPath {
				args["path"] = path
			}

			result, err := tool.Execute(context.Background(), args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantError, result.IsError)

			if tt.check != nil {
				tt.check(t, result, path)
			}
		})
	}
}

func TestExecuteMultilineDiff(t *testing.T) {
	tool := &tool{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "multi_line.txt")

	original := "line1\nline2\nline3\nline4\nline5"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "line2\nline3", "newText": "replaced"},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "-line2")
	assert.Contains(t, result.Content, "-line3")
	assert.Contains(t, result.Content, "+replaced")

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "line1\nreplaced\nline4\nline5"
	assert.Equal(t, expected, string(data))
}

func TestExecuteTruncation(t *testing.T) {
	tool := &tool{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "big.txt")

	lines := make([]string, 0, 3000)
	for range 3000 {
		lines = append(lines, "original line")
	}

	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644))

	newLines := make([]string, 0, 3000)
	for range 3000 {
		newLines = append(newLines, "replacement line that is longer")
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{
				"oldText": strings.Join(lines, "\n"),
				"newText": strings.Join(newLines, "\n"),
			},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "output truncated")
}

func TestExecuteSandboxDenied(t *testing.T) {
	tool := &tool{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "protected.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	sandboxer := &testSandboxer{
		allowWriteFn: func(p string) bool { return false },
	}
	sdk.SetSandboxer(sandboxer)
	t.Cleanup(func() { sdk.SetSandboxer(nil) })

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "original", "newText": "modified"},
		},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "sandbox: write denied")

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(data))
}

func TestExecuteSandboxAllowed(t *testing.T) {
	tool := &tool{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "allowed.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	sandboxer := &testSandboxer{
		allowWriteFn: func(p string) bool { return true },
	}
	sdk.SetSandboxer(sandboxer)
	t.Cleanup(func() { sdk.SetSandboxer(nil) })

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "hello", "newText": "world"},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "-hello")
	assert.Contains(t, result.Content, "+world")

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "world", string(data))
}

func TestExecuteSandboxNil(t *testing.T) {
	tool := &tool{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nosandbox.txt")
	require.NoError(t, os.WriteFile(path, []byte("before"), 0o644))

	sdk.SetSandboxer(nil)

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "before", "newText": "after"},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "after", string(data))
}

// testSandboxer is a minimal Sandboxer for edit-tool tests.
type testSandboxer struct {
	allowWriteFn func(string) bool
	allowReadFn  func(string) bool
	wrapFn       func(cmd, dir string) (string, error)
}

func (ts *testSandboxer) WrapCommand(cmd, dir string) (string, error) {
	if ts.wrapFn != nil {
		return ts.wrapFn(cmd, dir)
	}
	return cmd, nil
}
func (ts *testSandboxer) AllowWrite(path string) bool {
	if ts.allowWriteFn != nil {
		return ts.allowWriteFn(path)
	}
	return true
}
func (ts *testSandboxer) AllowRead(path string) bool {
	if ts.allowReadFn != nil {
		return ts.allowReadFn(path)
	}
	return true
}
