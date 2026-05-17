package edit

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/weave-agent/weave/sdk"

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
				// Verify the intermediate directory was created
				require.DirExists(t, filepath.Dir(path))

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
		{
			name: "replace_all with multiple occurrences",
			setup: func(t *testing.T) string {
				p := filepath.Join(tmpDir, "replaceall.txt")
				require.NoError(t, os.WriteFile(p, []byte("aaa bbb aaa"), 0o644))

				return p
			},
			args: map[string]any{
				"edits": []any{
					map[string]any{"oldText": "aaa", "newText": "ccc", "replace_all": true},
				},
			},
			wantError: false,
			check: func(t *testing.T, result sdk.ToolResult, path string) {
				assert.Contains(t, result.Content, "-aaa bbb aaa")
				assert.Contains(t, result.Content, "+ccc bbb ccc")

				data, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "ccc bbb ccc", string(data))
			},
		},
		{
			name: "replace_all=false with multiple occurrences error",
			setup: func(t *testing.T) string {
				p := filepath.Join(tmpDir, "noreplaceall.txt")
				require.NoError(t, os.WriteFile(p, []byte("aaa bbb aaa"), 0o644))

				return p
			},
			args: map[string]any{
				"edits": []any{
					map[string]any{"oldText": "aaa", "newText": "ccc", "replace_all": false},
				},
			},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult, _ string) {
				assert.Contains(t, result.Content, "matched 2 times")
			},
		},
		{
			name: "replace_all with no matches error",
			setup: func(t *testing.T) string {
				p := filepath.Join(tmpDir, "replaceall_nomatch.txt")
				require.NoError(t, os.WriteFile(p, []byte("hello world"), 0o644))

				return p
			},
			args: map[string]any{
				"edits": []any{
					map[string]any{"oldText": "notfound", "newText": "ccc", "replace_all": true},
				},
			},
			wantError: true,
			check: func(t *testing.T, result sdk.ToolResult, _ string) {
				assert.Contains(t, result.Content, "oldText not found")
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

	sb := &testSandboxer{
		allowWriteFn: func(p string) bool { return false },
	}
	setSandboxer(sb)

	t.Cleanup(func() { setSandboxer(nil) })

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

	sb := &testSandboxer{
		allowWriteFn: func(p string) bool { return true },
	}
	setSandboxer(sb)

	t.Cleanup(func() { setSandboxer(nil) })

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

	setSandboxer(nil)

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

func TestExecuteWithoutReadFails(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "unread.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	tracker := newMockFileTracker()
	tool := &tool{tracker: tracker}

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "original", "newText": "modified"},
		},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "file must be read before editing")

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(data))
}

func TestExecuteAfterReadSucceeds(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "read.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	info, err := os.Stat(path)
	require.NoError(t, err)

	tracker := newMockFileTracker()
	tracker.RecordRead(path, info.ModTime())

	tool := &tool{tracker: tracker}

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "original", "newText": "modified"},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "-original")
	assert.Contains(t, result.Content, "+modified")

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "modified", string(data))
}

func TestExecuteAfterExternalModificationFails(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stale.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	info, err := os.Stat(path)
	require.NoError(t, err)

	// Record read time from before modification
	tracker := newMockFileTracker()
	tracker.RecordRead(path, info.ModTime())

	// Modify file after recording read time
	require.NoError(t, os.WriteFile(path, []byte("externally modified"), 0o644))

	tool := &tool{tracker: tracker}

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "externally modified", "newText": "should fail"},
		},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "file was modified externally since last read")

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "externally modified", string(data))
}

func TestExecuteNewFileSkipsTracker(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "newdir", "newfile.txt")

	tracker := newMockFileTracker()
	tool := &tool{tracker: tracker}

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "", "newText": "fresh content"},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "fresh content", string(data))
}

// mockFileTracker is a test-double for sdk.FileTracker.
type mockFileTracker struct {
	mu    sync.RWMutex
	reads map[string]time.Time
}

func newMockFileTracker() *mockFileTracker {
	return &mockFileTracker{
		reads: make(map[string]time.Time),
	}
}

func (m *mockFileTracker) RecordRead(path string, modTime time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reads[path] = modTime
}

func (m *mockFileTracker) WasRead(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.reads[path]

	return ok
}

func (m *mockFileTracker) GetReadTime(path string) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, ok := m.reads[path]

	return t, ok
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

func TestConcurrentEditsToSameFileAreSerialized(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "concurrent.txt")

	// Start with placeholders that each goroutine will replace.
	placeholders := []string{"A", "B", "C", "D", "E"}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(placeholders, " ")), 0o644))

	// Use the real filemut.Mutex to verify actual serialization.
	fm := newTestMutex()
	tool := &tool{fileMutex: fm}

	var wg sync.WaitGroup

	results := make([]sdk.ToolResult, len(placeholders))

	for i, ph := range placeholders {
		wg.Add(1)

		go func(idx int, p string) {
			defer wg.Done()

			result, err := tool.Execute(context.Background(), map[string]any{
				"path": path,
				"edits": []any{
					map[string]any{"oldText": p, "newText": "done" + p},
				},
			})
			assert.NoError(t, err)

			results[idx] = result
		}(i, ph)
	}

	wg.Wait()

	// All edits should succeed because the mutex serializes them.
	successCount := 0

	for _, r := range results {
		if !r.IsError {
			successCount++
		}
	}

	assert.Equal(t, len(placeholders), successCount, "all concurrent edits should succeed")

	// Verify the file contains all replacements.
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	for _, ph := range placeholders {
		assert.Contains(t, string(data), "done"+ph, "placeholder %s should be replaced", ph)
	}
}

func TestConcurrentEditsToDifferentFilesRunInParallel(t *testing.T) {
	tmpDir := t.TempDir()

	paths := []string{
		filepath.Join(tmpDir, "file1.txt"),
		filepath.Join(tmpDir, "file2.txt"),
	}

	for _, p := range paths {
		require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))
	}

	// Use the real filemut.Mutex — different paths use different locks.
	fm := newTestMutex()
	tool := &tool{fileMutex: fm}

	var wg sync.WaitGroup

	results := make([]sdk.ToolResult, len(paths))

	for i, p := range paths {
		wg.Add(1)

		go func(idx int, filePath string) {
			defer wg.Done()

			result, err := tool.Execute(context.Background(), map[string]any{
				"path": filePath,
				"edits": []any{
					map[string]any{"oldText": "hello", "newText": "world"},
				},
			})
			assert.NoError(t, err)

			results[idx] = result
		}(i, p)
	}

	wg.Wait()

	for _, r := range results {
		assert.False(t, r.IsError, "edit should succeed")
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		require.NoError(t, err)

		assert.Equal(t, "world", string(data))
	}
}

func TestEditWithNilFileMutex(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nilmutex.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	// Tool with nil fileMutex should still work.
	tool := &tool{fileMutex: nil}

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "original", "newText": "modified"},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "modified", string(data))
}

func TestExecutePreservesCRLF(t *testing.T) {
	tool := &tool{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "crlf.txt")

	// File with Windows-style CRLF line endings.
	original := "line1\r\nline2\r\nline3\r\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	// Edit uses LF in oldText/newText; file should retain CRLF.
	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "line2", "newText": "replaced"},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "-line2")
	assert.Contains(t, result.Content, "+replaced")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "line1\r\nreplaced\r\nline3\r\n", string(data))
}

func TestExecutePreservesLF(t *testing.T) {
	tool := &tool{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "lf.txt")

	// File with Unix-style LF line endings.
	original := "line1\nline2\nline3\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "line2", "newText": "replaced"},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "line1\nreplaced\nline3\n", string(data))
}

func TestExecuteMixedEndingsNormalizedToFirstDetected(t *testing.T) {
	tool := &tool{}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "mixed.txt")

	// Mixed endings: CRLF appears first, so entire file should become CRLF.
	original := "line1\r\nline2\nline3\r\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
		"edits": []any{
			map[string]any{"oldText": "line2", "newText": "replaced"},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// After normalization+restore, all line endings should be CRLF.
	assert.Equal(t, "line1\r\nreplaced\r\nline3\r\n", string(data))
}

func (ts *testSandboxer) Mode() string   { return "auto" }
func (ts *testSandboxer) SetMode(string) {}
