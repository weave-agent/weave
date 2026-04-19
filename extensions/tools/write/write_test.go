package write

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	tool, err := sdk.GetTool("write", nil)
	require.NoError(t, err)
	assert.Equal(t, "write", tool.Name())
}

func TestDefinition(t *testing.T) {
	tool := &tool{}
	def := tool.Definition()
	assert.Equal(t, "write", def.Name)
	assert.NotNil(t, def.Parameters)
}

func TestExecute(t *testing.T) {
	tool := &tool{}

	tests := []struct {
		name      string
		args      map[string]any
		wantError bool
		check     func(t *testing.T, result sdk.ToolResult)
	}{
		{
			name:      "missing path",
			args:      map[string]any{"content": "hello"},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "path is required")
			},
		},
		{
			name:      "empty path",
			args:      map[string]any{"path": "", "content": "hello"},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "path is required")
			},
		},
		{
			name: "write new file",
			args: func() map[string]any {
				return map[string]any{
					"path":    filepath.Join(t.TempDir(), "new.txt"),
					"content": "hello world",
				}
			}(),
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "wrote 11 bytes")
			},
		},
		{
			name: "overwrite existing file",
			args: func() map[string]any {
				dir := t.TempDir()
				p := filepath.Join(dir, "exists.txt")
				os.WriteFile(p, []byte("original"), 0o644)
				return map[string]any{"path": p, "content": "updated"}
			}(),
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "wrote 7 bytes")
			},
		},
		{
			name: "nested directory creation",
			args: func() map[string]any {
				return map[string]any{
					"path":    filepath.Join(t.TempDir(), "a", "b", "c", "deep.txt"),
					"content": "nested",
				}
			}(),
			wantError: false,
		},
		{
			name:      "permission error",
			args:      map[string]any{"path": "/proc/1/write-test.txt", "content": "nope"},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "error:")
			},
		},
		{
			name: "write empty content",
			args: func() map[string]any {
				return map[string]any{
					"path":    filepath.Join(t.TempDir(), "empty.txt"),
					"content": "",
				}
			}(),
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "wrote 0 bytes")
			},
		},
		{
			name: "write multiline content",
			args: func() map[string]any {
				return map[string]any{
					"path":    filepath.Join(t.TempDir(), "multi.txt"),
					"content": "line1\nline2\nline3",
				}
			}(),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantError, result.IsError)
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestExecuteFileContents(t *testing.T) {
	tool := &tool{}
	dir := t.TempDir()

	// Write content and verify file contents match exactly
	path := filepath.Join(dir, "verify.txt")
	content := "exact content\nwith newlines\n"
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": content,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestExecuteNestedDirCreation(t *testing.T) {
	tool := &tool{}
	dir := t.TempDir()

	// Write to deeply nested path that doesn't exist
	path := filepath.Join(dir, "a", "b", "c", "nested.txt")
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "deep",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "deep", string(data))
}
