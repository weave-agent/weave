package read

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
	tool, err := sdk.GetTool("read", nil)
	require.NoError(t, err)
	assert.Equal(t, "read", tool.Name())
}

func TestDefinition(t *testing.T) {
	tool := &tool{}
	def := tool.Definition()
	assert.Equal(t, "read", def.Name)
	assert.NotNil(t, def.Parameters)
}

func TestExecute(t *testing.T) {
	tool := &tool{}

	tmpDir := t.TempDir()

	t.Run("missing path", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), map[string]any{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content, "path is required")
	})

	t.Run("nonexistent file", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), map[string]any{
			"path": filepath.Join(tmpDir, "nope.txt"),
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content, "error:")
	})

	t.Run("directory path", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), map[string]any{
			"path": tmpDir,
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content, "is a directory")
	})

	t.Run("read full file", func(t *testing.T) {
		path := filepath.Join(tmpDir, "full.txt")
		content := "line one\nline two\nline three"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{"path": path})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "1\tline one")
		assert.Contains(t, result.Content, "2\tline two")
		assert.Contains(t, result.Content, "3\tline three")
	})

	t.Run("read with offset", func(t *testing.T) {
		path := filepath.Join(tmpDir, "offset.txt")
		content := "first\nsecond\nthird\nfourth"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{
			"path":   path,
			"offset": float64(3),
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "3\tthird")
		assert.Contains(t, result.Content, "4\tfourth")
		assert.NotContains(t, result.Content, "1\tfirst")
	})

	t.Run("read with limit", func(t *testing.T) {
		path := filepath.Join(tmpDir, "limit.txt")
		content := "first\nsecond\nthird\nfourth\nfifth"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{
			"path":  path,
			"limit": float64(2),
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "1\tfirst")
		assert.Contains(t, result.Content, "2\tsecond")
		assert.NotContains(t, result.Content, "3\tthird")
	})

	t.Run("read with offset and limit", func(t *testing.T) {
		path := filepath.Join(tmpDir, "offsetlimit.txt")
		content := "a\nb\nc\nd\ne"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{
			"path":   path,
			"offset": float64(2),
			"limit":  float64(2),
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "2\tb")
		assert.Contains(t, result.Content, "3\tc")
		assert.NotContains(t, result.Content, "1\ta")
		assert.NotContains(t, result.Content, "4\td")
	})

	t.Run("binary file", func(t *testing.T) {
		path := filepath.Join(tmpDir, "binary.bin")
		data := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
		require.NoError(t, os.WriteFile(path, data, 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{"path": path})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.NotEmpty(t, result.Content)
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(tmpDir, "empty.txt")
		require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{"path": path})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Empty(t, result.Content)
	})

	t.Run("large file truncation", func(t *testing.T) {
		path := filepath.Join(tmpDir, "large.txt")
		lines := make([]string, 3000)
		for i := range lines {
			lines[i] = strings.Repeat("x", 20)
		}
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{"path": path})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "output truncated")
	})

	t.Run("long line", func(t *testing.T) {
		path := filepath.Join(tmpDir, "longline.txt")
		longLine := strings.Repeat("x", 2*1024*1024)
		require.NoError(t, os.WriteFile(path, []byte("before\nTARGET"+longLine+"\nafter"), 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{"path": path})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "1\tbefore")
	})

	t.Run("very long line exceeds old scanner cap", func(t *testing.T) {
		path := filepath.Join(tmpDir, "verylongline.txt")
		longLine := strings.Repeat("y", 12*1024*1024)
		require.NoError(t, os.WriteFile(path, []byte("first\n"+longLine+"\nlast"), 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{"path": path})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "1\tfirst")
	})

	t.Run("single long line produces visible content", func(t *testing.T) {
		path := filepath.Join(tmpDir, "singlelongline.txt")
		longLine := strings.Repeat("a", 60000)
		require.NoError(t, os.WriteFile(path, []byte(longLine), 0o644))

		result, err := tool.Execute(context.Background(), map[string]any{"path": path})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "1\t")
		assert.Contains(t, result.Content, "line truncated")
		assert.Contains(t, result.Content, "a")
	})
}
