package write

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/weave-agent/weave/sdk"

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
				require.NoError(t, os.WriteFile(p, []byte("original"), 0o644))

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

func TestExecutePreservesPermissions(t *testing.T) {
	tool := &tool{}
	dir := t.TempDir()

	// Create a file with executable permissions
	p := filepath.Join(dir, "script.sh")
	require.NoError(t, os.WriteFile(p, []byte("original"), 0o755))

	// Overwrite it
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    p,
		"content": "#!/bin/bash\necho hello",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Verify permissions are preserved
	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o755), info.Mode().Perm())
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

func TestExecuteSandboxDenied(t *testing.T) {
	tool := &tool{}
	dir := t.TempDir()
	path := filepath.Join(dir, "protected.txt")

	sb := &testSandboxer{
		allowWriteFn: func(p string) bool { return false },
	}
	setSandboxer(sb)

	t.Cleanup(func() { setSandboxer(nil) })

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "should not write",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "sandbox: write denied")

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestExecuteSandboxAllowed(t *testing.T) {
	tool := &tool{}
	dir := t.TempDir()
	path := filepath.Join(dir, "allowed.txt")

	sb := &testSandboxer{
		allowWriteFn: func(p string) bool { return true },
	}
	setSandboxer(sb)

	t.Cleanup(func() { setSandboxer(nil) })

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "hello",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "wrote 5 bytes")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestExecuteSandboxNil(t *testing.T) {
	tool := &tool{}
	dir := t.TempDir()
	path := filepath.Join(dir, "nosandbox.txt")

	setSandboxer(nil)

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "works normally",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "wrote 14 bytes")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "works normally", string(data))
}

// testSandboxer is a minimal Sandboxer for write-tool tests.
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

func (ts *testSandboxer) Mode() string   { return "auto" }
func (ts *testSandboxer) SetMode(string) {}

func TestExecuteNoop(t *testing.T) {
	tool := &tool{}
	dir := t.TempDir()
	path := filepath.Join(dir, "noop.txt")
	content := "identical content\nwith newlines"

	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	info, err := os.Stat(path)
	require.NoError(t, err)

	modTime := info.ModTime()

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": content,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "already contains the exact content")
	assert.Contains(t, result.Content, path)

	// Verify file was not touched (mod time unchanged)
	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o644), info.Mode().Perm())
	assert.Equal(t, modTime, info.ModTime())
}

func TestExecuteNoopDifferentContent(t *testing.T) {
	tool := &tool{}
	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.txt")

	require.NoError(t, os.WriteFile(path, []byte("original content"), 0o644))

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "new content",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "wrote 11 bytes")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(data))
}

func TestExecuteNoopNewFile(t *testing.T) {
	tool := &tool{}
	dir := t.TempDir()
	path := filepath.Join(dir, "brandnew.txt")
	content := "new file content"

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": content,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "wrote 16 bytes")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}
