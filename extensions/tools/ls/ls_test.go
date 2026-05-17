package ls

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weave-agent/weave/sdk"

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

func TestExecuteSandboxDenied(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("data"), 0o644))

	sb := &testSandboxer{allowReadFn: func(p string) bool { return false }}
	setSandboxer(sb)

	t.Cleanup(func() { setSandboxer(nil) })

	result, err := (&tool{}).Execute(context.Background(), map[string]any{"path": dir})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "sandbox: read denied")
}

func TestExecuteSandboxAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readable.txt"), []byte("data"), 0o644))

	sb := &testSandboxer{allowReadFn: func(p string) bool { return true }}
	setSandboxer(sb)

	t.Cleanup(func() { setSandboxer(nil) })

	result, err := (&tool{}).Execute(context.Background(), map[string]any{"path": dir})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "readable.txt")
}

func TestExecuteSandboxNil(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "normal.txt"), []byte("data"), 0o644))

	setSandboxer(nil)

	result, err := (&tool{}).Execute(context.Background(), map[string]any{"path": dir})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "normal.txt")
}

func TestExecuteSandboxPerEntryFiltering(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("secret"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("b"), 0o644))

	// Allow the directory and visible files, but deny .env.
	sb := &testSandboxer{allowReadFn: func(p string) bool {
		return !strings.HasSuffix(p, "/.env")
	}}
	setSandboxer(sb)

	t.Cleanup(func() { setSandboxer(nil) })

	result, err := (&tool{}).Execute(context.Background(), map[string]any{"path": dir})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "visible.txt")
	assert.Contains(t, result.Content, "readme.md")
	assert.NotContains(t, result.Content, ".env", ".env should be filtered out by sandbox")
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

func (ts *testSandboxer) Mode() string   { return "auto" }
func (ts *testSandboxer) SetMode(string) {}

func TestExecuteSorted(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Zebra.txt"), []byte("z"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "apple.txt"), []byte("a"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "Beta"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cherry.txt"), []byte("c"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{"path": dir})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	lines := strings.Split(result.Content, "\n")
	assert.Equal(t, []string{"apple.txt", "Beta/", "cherry.txt", "Zebra.txt"}, lines)
}

func TestExecuteLimit(t *testing.T) {
	dir := t.TempDir()

	for i := range 10 {
		name := fmt.Sprintf("file%02d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	}

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{"path": dir, "limit": 3})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	lines := strings.Split(result.Content, "\n")
	assert.Len(t, lines, 5) // 3 entries + blank line + truncation notice
	assert.Equal(t, "file00.txt", lines[0])
	assert.Equal(t, "file01.txt", lines[1])
	assert.Equal(t, "file02.txt", lines[2])
	assert.Contains(t, result.Content, "more entries not shown")
}

func TestExecuteLimitZero(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{"path": dir, "limit": 0})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.Contains(t, result.Content, "a.txt")
	assert.Contains(t, result.Content, "b.txt")
}

func TestExecuteDefaultLimit(t *testing.T) {
	dir := t.TempDir()

	for i := range 10 {
		name := fmt.Sprintf("file%02d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	}

	result, err := (&tool{defaultLimit: 5}).Execute(context.Background(), map[string]any{"path": dir})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	lines := strings.Split(result.Content, "\n")
	assert.Equal(t, "file00.txt", lines[0])
	assert.Equal(t, "file01.txt", lines[1])
	assert.Equal(t, "file02.txt", lines[2])
	assert.Equal(t, "file03.txt", lines[3])
	assert.Equal(t, "file04.txt", lines[4])
	assert.Contains(t, result.Content, "more entries not shown")
}

func TestExecuteIgnorePatterns(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("x"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":   dir,
		"ignore": []any{"*_test.go", ".env"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.Contains(t, result.Content, "README.md")
	assert.Contains(t, result.Content, "main.go")
	assert.NotContains(t, result.Content, "main_test.go")
	assert.NotContains(t, result.Content, ".env")
}

func TestExecuteIgnoreNoMatch(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":   dir,
		"ignore": []any{"*.go"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.Contains(t, result.Content, "a.txt")
	assert.Contains(t, result.Content, "b.txt")
}

func TestExecuteLimitAndIgnore(t *testing.T) {
	dir := t.TempDir()

	for i := range 10 {
		name := fmt.Sprintf("file%02d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("x"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":   dir,
		"limit":  3,
		"ignore": []any{"skip.txt"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.NotContains(t, result.Content, "skip.txt")
	assert.Equal(t, "file00.txt", strings.Split(result.Content, "\n")[0])
	assert.Equal(t, "file01.txt", strings.Split(result.Content, "\n")[1])
	assert.Equal(t, "file02.txt", strings.Split(result.Content, "\n")[2])
	assert.Contains(t, result.Content, "more entries not shown")
}

func TestExecuteIgnoreDirectories(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "vendor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "dep.go"), []byte("x"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":   dir,
		"ignore": []any{"vendor"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.Contains(t, result.Content, "main.go")
	assert.NotContains(t, result.Content, "vendor/")
	assert.NotContains(t, result.Content, "dep.go")
}

func TestExecuteTreeFormat(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.go"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file2.go"), []byte("b"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "nested.go"), []byte("c"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "another.go"), []byte("d"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 2,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	lines := strings.Split(result.Content, "\n")
	require.GreaterOrEqual(t, len(lines), 5)

	// Root directory name
	assert.True(t, strings.HasSuffix(lines[0], "/"), "root should end with /: %s", lines[0])

	// Tree characters present
	assert.Contains(t, result.Content, "├──")
	assert.Contains(t, result.Content, "└──")

	// All entries present
	assert.Contains(t, result.Content, "file1.go")
	assert.Contains(t, result.Content, "file2.go")
	assert.Contains(t, result.Content, "subdir/")
	assert.Contains(t, result.Content, "nested.go")
	assert.Contains(t, result.Content, "another.go")

	// Directory has trailing slash
	assert.Contains(t, result.Content, "subdir/")
}

func TestExecuteTreeDepthLimit(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "root.txt"), []byte("a"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "level1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "level1", "l1.txt"), []byte("b"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "level1", "level2"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "level1", "level2", "l2.txt"), []byte("c"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "level1", "level2", "level3"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "level1", "level2", "level3", "l3.txt"), []byte("d"), 0o644))

	// depth=1: only root and level1 entries
	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 1,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "root.txt")
	assert.Contains(t, result.Content, "level1/")
	assert.NotContains(t, result.Content, "l1.txt")

	// depth=2: root, level1, and l1.txt
	result, err = (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 2,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "l1.txt")
	assert.Contains(t, result.Content, "level2/")
	assert.NotContains(t, result.Content, "l2.txt")

	// depth=3: includes l2.txt
	result, err = (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 3,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "l2.txt")
	assert.Contains(t, result.Content, "level3/")
	assert.NotContains(t, result.Content, "l3.txt")

	// depth=4: includes l3.txt
	result, err = (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 4,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "l3.txt")
}

func TestExecuteTreeDeeplyNested(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644))

	// Build deep nesting: dir/1/2/3/4/5/deep.txt
	current := dir
	for i := 1; i <= 5; i++ {
		next := filepath.Join(current, fmt.Sprintf("level%d", i))
		require.NoError(t, os.Mkdir(next, 0o755))
		current = next
	}

	require.NoError(t, os.WriteFile(filepath.Join(current, "deep.txt"), []byte("deep"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 10,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.Contains(t, result.Content, "a.txt")
	assert.Contains(t, result.Content, "deep.txt")

	// Verify tree prefixes at various depths
	lines := strings.Split(result.Content, "\n")

	var deepLine string

	for _, line := range lines {
		if strings.Contains(line, "deep.txt") {
			deepLine = line
			break
		}
	}

	require.NotEmpty(t, deepLine)
	// Should have multiple │ or spaces followed by └──
	assert.Contains(t, deepLine, "└──")
	// The prefix should be long due to depth
	assert.Greater(t, len(deepLine), 20, "deep line should have long prefix: %s", deepLine)
}

func TestExecuteTreeEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 2,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.Contains(t, result.Content, "(empty directory)")
}

func TestExecuteTreeSorted(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Zebra.txt"), []byte("z"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "apple.txt"), []byte("a"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "Beta"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cherry.txt"), []byte("c"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "alpha"), 0o755))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 1,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	lines := strings.Split(result.Content, "\n")
	require.GreaterOrEqual(t, len(lines), 5)

	// Directories before files, case-insensitive sorted (with tree prefixes)
	assert.Equal(t, "├── alpha/", lines[1])
	assert.Equal(t, "├── Beta/", lines[2])
	assert.Equal(t, "├── apple.txt", lines[3])
	assert.Equal(t, "├── cherry.txt", lines[4])
	assert.Equal(t, "└── Zebra.txt", lines[5])
}

func TestExecuteTreeLimit(t *testing.T) {
	dir := t.TempDir()

	for i := range 10 {
		name := fmt.Sprintf("file%02d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	}

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 1,
		"limit": 3,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// limit=3 means 3 lines total (root + 2 entries) + blank line + truncation notice = 5 lines
	lines := strings.Split(result.Content, "\n")
	require.Len(t, lines, 5)
	assert.Contains(t, lines[1], "file00.txt")
	assert.Contains(t, lines[2], "file01.txt")
	assert.Contains(t, result.Content, "more entries not shown")
}

func TestExecuteTreeWithIgnore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("x"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "vendor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "dep.go"), []byte("x"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":   dir,
		"depth":  2,
		"ignore": []any{"*_test.go", "vendor"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.Contains(t, result.Content, "main.go")
	assert.NotContains(t, result.Content, "main_test.go")
	assert.NotContains(t, result.Content, "vendor/")
	assert.NotContains(t, result.Content, "dep.go")
}

func TestExecuteTreeSandboxFiltering(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("secret"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "secrets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secrets", "key.txt"), []byte("key"), 0o644))

	sb := &testSandboxer{allowReadFn: func(p string) bool {
		return !strings.Contains(p, ".env") && !strings.Contains(p, "secrets")
	}}
	setSandboxer(sb)

	t.Cleanup(func() { setSandboxer(nil) })

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":  dir,
		"depth": 2,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	assert.Contains(t, result.Content, "visible.txt")
	assert.NotContains(t, result.Content, ".env")
	assert.NotContains(t, result.Content, "secrets/")
	assert.NotContains(t, result.Content, "key.txt")
}

func TestExecuteIgnoreInvalidPattern(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644))

	// Invalid glob pattern should not crash and should not filter anything
	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path":   dir,
		"ignore": []any{"["},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Both files should still be present
	assert.Contains(t, result.Content, "a.txt")
	assert.Contains(t, result.Content, "b.txt")
}

func TestExecuteFlatStillWorks(t *testing.T) {
	// Ensure depth=0 (default) still gives flat list
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "nested.txt"), []byte("c"), 0o644))

	result, err := (&tool{defaultLimit: 500}).Execute(context.Background(), map[string]any{
		"path": dir,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// Flat list: only top-level entries
	assert.Contains(t, result.Content, "a.txt")
	assert.Contains(t, result.Content, "b.txt")
	assert.Contains(t, result.Content, "subdir/")
	assert.NotContains(t, result.Content, "nested.txt")

	// No tree characters in flat mode
	assert.NotContains(t, result.Content, "├──")
	assert.NotContains(t, result.Content, "└──")
}
