package grep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	tool, err := sdk.GetTool("grep", nil)
	require.NoError(t, err)
	assert.Equal(t, "grep", tool.Name())
}

func TestDefinition(t *testing.T) {
	tool := &tool{}
	def := tool.Definition()
	assert.Equal(t, "grep", def.Name)
	assert.NotNil(t, def.Parameters)
}

func createTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	return path
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // returns path
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
			name:  "simple match",
			setup: func(t *testing.T) string { return createTempFile(t, "hello world\nfoo bar\nhello again") },
			args:  map[string]any{"pattern": "hello"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "hello world")
				assert.Contains(t, result.Content, "hello again")
				assert.NotContains(t, result.Content, "foo bar")
			},
		},
		{
			name:  "no match",
			setup: func(t *testing.T) string { return createTempFile(t, "hello world\nfoo bar") },
			args:  map[string]any{"pattern": "notfound"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "no matches found")
			},
		},
		{
			name:  "case-insensitive",
			setup: func(t *testing.T) string { return createTempFile(t, "Hello World\nfoo bar") },
			args:  map[string]any{"pattern": "hello", "ignoreCase": true},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "Hello World")
			},
		},
		{
			name:  "literal mode",
			setup: func(t *testing.T) string { return createTempFile(t, "test (foo)\ntest bar") },
			args:  map[string]any{"pattern": "(foo)", "literal": true},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "test (foo)")
				assert.NotContains(t, result.Content, "test bar")
			},
		},
		{
			name: "context lines",
			setup: func(t *testing.T) string {
				return createTempFile(t, "line1\nline2\nMATCH\nline4\nline5")
			},
			args: map[string]any{"pattern": "MATCH", "context": float64(1)},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "line2")
				assert.Contains(t, result.Content, "MATCH")
				assert.Contains(t, result.Content, "line4")
				assert.NotContains(t, result.Content, "line1")
				assert.NotContains(t, result.Content, "line5")
			},
		},
		{
			name:      "invalid regex",
			setup:     func(t *testing.T) string { return createTempFile(t, "hello") },
			args:      map[string]any{"pattern": "[invalid"},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "invalid pattern")
			},
		},
		{
			name:      "nonexistent path",
			setup:     func(t *testing.T) string { return "/nonexistent/path/xyz" },
			args:      map[string]any{"pattern": "test", "path": "/nonexistent/path/xyz"},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "error:")
			},
		},
		{
			name: "directory search",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("findme in a"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("no match here"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte("findme in c"), 0o644))

				return dir
			},
			args: map[string]any{"pattern": "findme"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "findme in a")
				assert.Contains(t, result.Content, "findme in c")
				assert.NotContains(t, result.Content, "no match here")
			},
		},
		{
			name: "skips ignored directories",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("findme git"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "main.txt"), []byte("findme main"), 0o644))

				return dir
			},
			args: map[string]any{"pattern": "findme"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.Contains(t, result.Content, "findme main")
				assert.NotContains(t, result.Content, "findme git")
			},
		},
		{
			name: "long line",
			setup: func(t *testing.T) string {
				longLine := strings.Repeat("x", 2*1024*1024)
				return createTempFile(t, "before\nTARGET"+longLine+"\nafter")
			},
			args: map[string]any{"pattern": "TARGET"},
			check: func(t *testing.T, result sdk.ToolResult) {
				assert.NotContains(t, result.Content, "no matches found")
				assert.Contains(t, result.Content, "output truncated")
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("findme"), 0o644))

	sandboxer := &testSandboxer{allowReadFn: func(p string) bool { return false }}
	sdk.SetSandboxer(sandboxer)
	t.Cleanup(func() { sdk.SetSandboxer(nil) })

	result, err := (&tool{}).Execute(context.Background(), map[string]any{
		"pattern": "findme",
		"path":    dir,
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "sandbox: read denied")
}

func TestExecuteSandboxAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readable.txt"), []byte("findme here"), 0o644))

	sandboxer := &testSandboxer{allowReadFn: func(p string) bool { return true }}
	sdk.SetSandboxer(sandboxer)
	t.Cleanup(func() { sdk.SetSandboxer(nil) })

	result, err := (&tool{}).Execute(context.Background(), map[string]any{
		"pattern": "findme",
		"path":    dir,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "findme here")
}

func TestExecuteSandboxNil(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "normal.txt"), []byte("findme normal"), 0o644))

	sdk.SetSandboxer(nil)

	result, err := (&tool{}).Execute(context.Background(), map[string]any{
		"pattern": "findme",
		"path":    dir,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "findme normal")
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
