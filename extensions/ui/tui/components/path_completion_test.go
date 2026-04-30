package components

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathCompletionsNoPrefix(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "alpha.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "beta.txt"), []byte(""), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(tmp, "gamma"), 0o755))

	items := PathCompletions(tmp, "")
	require.Len(t, items, 3)

	assert.Equal(t, "alpha.go", items[0].Label)
	assert.Equal(t, "alpha.go", items[0].Value)

	assert.Equal(t, "beta.txt", items[1].Label)
	assert.Equal(t, "beta.txt", items[1].Value)

	assert.Equal(t, "gamma/", items[2].Label)
	assert.Equal(t, "gamma/", items[2].Value)
}

func TestPathCompletionsPartialMatch(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "apple.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "application.txt"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "banana.go"), []byte(""), 0o644))

	items := PathCompletions(tmp, "app")
	require.Len(t, items, 2)

	assert.Equal(t, "apple.go", items[0].Label)
	assert.Equal(t, "application.txt", items[1].Label)
}

func TestPathCompletionsCaseInsensitive(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "Apple.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "apricot.txt"), []byte(""), 0o644))

	items := PathCompletions(tmp, "AP")
	require.Len(t, items, 2)
	assert.Equal(t, "Apple.go", items[0].Label)
	assert.Equal(t, "apricot.txt", items[1].Label)
}

func TestPathCompletionsNestedPaths(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "util.go"), []byte(""), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(srcDir, "internal"), 0o755))

	items := PathCompletions(tmp, "src/")
	require.Len(t, items, 3)

	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.Label
	}

	assert.Contains(t, labels, "internal/")
	assert.Contains(t, labels, "main.go")
	assert.Contains(t, labels, "util.go")

	// Filter within nested path
	items = PathCompletions(tmp, "src/ma")
	require.Len(t, items, 1)
	assert.Equal(t, "main.go", items[0].Label)
	assert.Equal(t, "src/main.go", items[0].Value)

	// Directory gets trailing slash in value
	items = PathCompletions(tmp, "src/in")
	require.Len(t, items, 1)
	assert.Equal(t, "internal/", items[0].Label)
	assert.Equal(t, "src/internal/", items[0].Value)
}

func TestPathCompletionsNonexistentDir(t *testing.T) {
	tmp := t.TempDir()

	items := PathCompletions(tmp, "nosuchdir/")
	assert.Empty(t, items)

	items = PathCompletions(tmp, "nosuchdir/file")
	assert.Empty(t, items)
}

func TestPathCompletionsSkipsHiddenFiles(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "visible.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".hidden.go"), []byte(""), 0o644))

	items := PathCompletions(tmp, "")
	require.Len(t, items, 1)
	assert.Equal(t, "visible.go", items[0].Label)
}

func TestPathCompletionsEmptyDir(t *testing.T) {
	tmp := t.TempDir()

	items := PathCompletions(tmp, "")
	assert.Empty(t, items)
}

func TestPathCompletionsNoMatch(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "alpha.go"), []byte(""), 0o644))

	items := PathCompletions(tmp, "zzz")
	assert.Empty(t, items)
}

func TestSplitPrefix(t *testing.T) {
	tests := []struct {
		prefix   string
		wantDir  string
		wantName string
	}{
		{"", "", ""},
		{"foo", "", "foo"},
		{"dir/", "dir/", ""},
		{"dir/foo", "dir/", "foo"},
		{"a/b/c", "a/b/", "c"},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			dir, name := splitPrefix(tt.prefix)
			assert.Equal(t, tt.wantDir, dir)
			assert.Equal(t, tt.wantName, name)
		})
	}
}
