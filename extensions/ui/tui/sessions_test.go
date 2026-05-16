package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSessions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	sessions, err := listSessions("")
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestListSessions_NoDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	t.Setenv("WEAVE_JSONL_DIR", dir)

	sessions, err := listSessions("")
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestListSessions_ReadsHeaders(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	now := time.Now().UTC().Truncate(time.Second)

	writeSessionFile(t, dir, "bbb11122233344455566677788899900", "/project/beta", now.Add(-time.Hour))
	writeSessionFile(t, dir, "aaa11122233344455566677788899900", "/project/alpha", now)

	sessions, err := listSessions("")
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	// Sorted by most recent first
	assert.Equal(t, "aaa11122233344455566677788899900", sessions[0].ID)
	assert.Equal(t, "/project/alpha", sessions[0].CWD)
	assert.Equal(t, now, sessions[0].CreatedAt)

	assert.Equal(t, "bbb11122233344455566677788899900", sessions[1].ID)
	assert.Equal(t, "/project/beta", sessions[1].CWD)
}

func TestListSessions_SkipsNonJSONL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	writeSessionFile(t, dir, "aaa11122233344455566677788899900", "/project", time.Now())

	// Non-JSONL file should be skipped
	err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644)
	require.NoError(t, err)

	// Invalid JSONL (no header) should be skipped
	err = os.WriteFile(filepath.Join(dir, "bad.jsonl"), []byte("not json\n"), 0o644)
	require.NoError(t, err)

	sessions, err := listSessions("")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "aaa11122233344455566677788899900", sessions[0].ID)
}

func TestLoadSessionEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	sessionID := "aaa11122233344455566677788899900"
	now := time.Now().UTC()

	// Write header + message entries
	header := sessionHeader{Type: "session", ID: sessionID, Timestamp: now, CWD: "/test"}
	headerJSON, _ := json.Marshal(header)

	entry1 := map[string]any{
		"type": "message",
		"data": map[string]any{"role": "user", "content": "hello"},
	}
	entry2 := map[string]any{
		"type": "message",
		"data": map[string]any{"role": "assistant", "content": "hi there"},
	}
	entry3 := map[string]any{
		"type": "summary",
		"data": map[string]any{"removed_count": 5},
	}

	e1JSON, _ := json.Marshal(entry1)
	e2JSON, _ := json.Marshal(entry2)
	e3JSON, _ := json.Marshal(entry3)

	content := string(headerJSON) + "\n" + string(e1JSON) + "\n" + string(e2JSON) + "\n" + string(e3JSON) + "\n"
	err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(content), 0o644)
	require.NoError(t, err)

	entries, err := loadSessionEntries("", sessionID)
	require.NoError(t, err)
	require.Len(t, entries, 2) // summary entry skipped

	assert.Equal(t, "user", entries[0].Role)
	assert.Equal(t, "hello", entries[0].Content)
	assert.Equal(t, "assistant", entries[1].Role)
	assert.Equal(t, "hi there", entries[1].Content)
}

func TestLoadSessionEntries_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	_, err := loadSessionEntries("", "nonexistent0000000000000000000")
	assert.Error(t, err)
}

func TestLoadSessionEntries_EmptySession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	sessionID := "aaa11122233344455566677788899900"
	header := sessionHeader{Type: "session", ID: sessionID, Timestamp: time.Now().UTC(), CWD: "/test"}
	headerJSON, _ := json.Marshal(header)

	err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(string(headerJSON)+"\n"), 0o644)
	require.NoError(t, err)

	entries, err := loadSessionEntries("", sessionID)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestLoadSessionEntries_PathTraversalRejected(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
	}{
		{"parent traversal", "../etc/passwd"},
		{"forward slash", "foo/bar"},
		{"backslash", "foo\\bar"},
		{"mixed traversal", "..\\..\\secrets"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadSessionEntries("", tt.sessionID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid session ID")
		})
	}
}

func TestShortenCWD(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"home prefix", home + "/projects/myapp", "~/projects/myapp"},
		{"no home prefix", "/opt/projects", "/opt/projects"},
		{"exact home", home, "~"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shortenCWD(tt.cwd))
		})
	}
}

func TestListSessionsCmd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEAVE_JSONL_DIR", dir)

	writeSessionFile(t, dir, "aaa11122233344455566677788899900", "/test", time.Now())

	cmd := listSessionsCmd("")
	msg := cmd()

	result, ok := msg.(SessionListResultMsg)
	require.True(t, ok)
	require.NoError(t, result.Err)
	require.Len(t, result.Sessions, 1)
	assert.Equal(t, "aaa11122233344455566677788899900", result.Sessions[0].ID)
}

func TestSessionDir_EnvOverride(t *testing.T) {
	custom := "/tmp/weave-test-sessions"
	t.Setenv("WEAVE_JSONL_DIR", custom)

	dir, err := sessionDir()
	require.NoError(t, err)
	assert.Equal(t, custom, dir)
}

func writeSessionFile(t *testing.T, dir, sessionID, cwd string, ts time.Time) {
	t.Helper()

	header := sessionHeader{Type: "session", ID: sessionID, Timestamp: ts, CWD: cwd}
	headerJSON, err := json.Marshal(header)
	require.NoError(t, err)

	path := filepath.Join(dir, sessionID+".jsonl")
	line := fmt.Appendf(nil, "%s\n", headerJSON)
	err = os.WriteFile(path, line, 0o644)
	require.NoError(t, err)
}
