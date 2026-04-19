package ls

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
	tool, err := sdk.GetTool("ls", nil)
	require.NoError(t, err)
	assert.Equal(t, "ls", tool.Name())
}

func TestDefinition(t *testing.T) {
	tool := &tool{}
	def := tool.Definition()
	assert.Equal(t, "ls", def.Name)
	assert.NotNil(t, def.Parameters)
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		args      map[string]any
		wantError bool
		check     func(t *testing.T, result sdk.ToolResult)
	}{
		{
			name: "list specific dir",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("a"), 0o644))
				require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "file2.go"), []byte("b"), 0o644))
				return dir
			},
			args: map[string]any{},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "file1.txt")
				assert.Contains(t, result.Content, "file2.go")
				assert.Contains(t, result.Content, "subdir/")
				assert.False(t, result.IsError)
			},
		},
		{
			name:      "nonexistent dir",
			setup:     func(t *testing.T) string { return "/nonexistent/path/xyz" },
			args:      map[string]any{"path": "/nonexistent/path/xyz"},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "error:")
			},
		},
		{
			name: "empty dir",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args: map[string]any{},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "empty directory")
				assert.False(t, result.IsError)
			},
		},
		{
			name: "path is a file",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				f := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(f, []byte("hi"), 0o644))
				return f
			},
			args:      map[string]any{},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "not a directory")
			},
		},
		{
			name: "list cwd default",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0o644))
				return dir
			},
			args: map[string]any{},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "hello.txt")
				assert.False(t, result.IsError)
			},
		},
		{
			name: "directories have trailing slash",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.Mkdir(filepath.Join(dir, "mydir"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "myfile"), []byte("x"), 0o644))
				return dir
			},
			args: map[string]any{},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "mydir/")
				assert.Contains(t, result.Content, "myfile")
				assert.NotContains(t, result.Content, "myfile/")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			args := tt.args
			if _, ok := args["path"]; !ok {
				args["path"] = path
			}

			result, err := (&tool{}).Execute(context.Background(), args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantError, result.IsError)
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
