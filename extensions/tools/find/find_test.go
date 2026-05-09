package find

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
	tool, err := sdk.GetTool("find", nil)
	require.NoError(t, err)
	assert.Equal(t, "find", tool.Name())
}

func TestDefinition(t *testing.T) {
	tool := &tool{}
	def := tool.Definition()
	assert.Equal(t, "find", def.Name)
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
			name:      "missing pattern",
			setup:     func(t *testing.T) string { return "." },
			args:      map[string]any{},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "pattern is required")
			},
		},
		{
			name: "find by extension",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "c.go"), []byte("package pkg"), 0o644))

				return dir
			},
			args: map[string]any{"pattern": "*.go"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "a.go")
				assert.Contains(t, result.Content, "c.go")
				assert.NotContains(t, result.Content, "b.txt")
			},
		},
		{
			name: "find by name",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("key: val"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644))

				return dir
			},
			args: map[string]any{"pattern": "config.yaml"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "config.yaml")
				assert.NotContains(t, result.Content, "config.json")
			},
		},
		{
			name: "nested match",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				sub := filepath.Join(dir, "sub", "deep")
				require.NoError(t, os.MkdirAll(sub, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(sub, "target.txt"), []byte("found"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "other.go"), []byte("package main"), 0o644))

				return dir
			},
			args: map[string]any{"pattern": "*.txt"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "target.txt")
				assert.NotContains(t, result.Content, "other.go")
			},
		},
		{
			name: "no matches",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))

				return dir
			},
			args: map[string]any{"pattern": "*.xyz"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "no files found")
			},
		},
		{
			name:      "nonexistent path",
			setup:     func(t *testing.T) string { return "/nonexistent/path/xyz" },
			args:      map[string]any{"pattern": "*.go", "path": "/nonexistent/path/xyz"},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "error:")
			},
		},
		{
			name: "skips ignored directories",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("git config"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644))

				return dir
			},
			args: map[string]any{"pattern": "*"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "main.go")
				assert.NotContains(t, result.Content, "config")
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
			args:      map[string]any{"pattern": "*.txt"},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "not a directory")
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

func TestExecuteSandboxDenied(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("data"), 0o644))

	sandboxer := &testSandboxer{allowReadFn: func(p string) bool { return false }}
	sdk.SetSandboxer(sandboxer)
	t.Cleanup(func() { sdk.SetSandboxer(nil) })

	result, err := (&tool{}).Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    dir,
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "sandbox: read denied")
}

func TestExecuteSandboxAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readable.txt"), []byte("data"), 0o644))

	sandboxer := &testSandboxer{allowReadFn: func(p string) bool { return true }}
	sdk.SetSandboxer(sandboxer)
	t.Cleanup(func() { sdk.SetSandboxer(nil) })

	result, err := (&tool{}).Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    dir,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "readable.txt")
}

func TestExecuteSandboxNil(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "normal.txt"), []byte("data"), 0o644))

	sdk.SetSandboxer(nil)

	result, err := (&tool{}).Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    dir,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "normal.txt")
}

type testSandboxer struct {
	allowReadFn  func(string) bool
	allowWriteFn func(string) bool
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
