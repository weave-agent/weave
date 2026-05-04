package jsonlstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	eventbus "weave/bus"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()

	return &Store{cfg: Config{JSONL: JSONLOpts{Dir: dir}}}
}

func TestGenerateID(t *testing.T) {
	id, err := generateID()
	require.NoError(t, err)
	assert.Len(t, id, 32)

	id2, err := generateID()
	require.NoError(t, err)
	assert.NotEqual(t, id, id2)
}

func TestCreate(t *testing.T) {
	s := newTestStore(t)

	sess, err := s.Create("/tmp/project")
	require.NoError(t, err)

	assert.Equal(t, "session", sess.Header.Type)
	assert.Len(t, sess.Header.ID, 32)
	assert.Equal(t, "/tmp/project", sess.Header.CWD)
	assert.False(t, sess.Header.Timestamp.IsZero())

	path := filepath.Join(s.cfg.JSONL.Dir, sess.Header.ID+".jsonl")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var header SessionHeader
	require.NoError(t, json.Unmarshal(data[:len(data)-1], &header))
	assert.Equal(t, sess.Header.ID, header.ID)
	assert.Equal(t, "/tmp/project", header.CWD)
}

func TestAppend(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("/tmp/project")
	require.NoError(t, err)

	entry := Entry{
		Type: "message",
		Turn: 1,
		Data: json.RawMessage(`{"role":"user","content":"hello"}`),
	}
	_, err = s.Append(sess.Header.ID, entry)
	require.NoError(t, err)

	entry2 := Entry{
		Type: "message",
		Turn: 2,
		Data: json.RawMessage(`{"role":"assistant","content":"hi"}`),
	}
	_, err = s.Append(sess.Header.ID, entry2)
	require.NoError(t, err)

	loaded, err := s.Load(sess.Header.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Entries, 2)

	assert.Len(t, loaded.Entries[0].ID, 32)
	assert.False(t, loaded.Entries[0].Created.IsZero())
	assert.Equal(t, 1, loaded.Entries[0].Turn)
	assert.JSONEq(t, `{"role":"user","content":"hello"}`, string(loaded.Entries[0].Data))

	assert.Len(t, loaded.Entries[1].ID, 32)
	assert.Equal(t, 2, loaded.Entries[1].Turn)
}

func TestAppend_PreservesIDAndTimestamp(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("/tmp/project")
	require.NoError(t, err)

	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entry := Entry{
		ID:      "custom-id",
		Type:    "message",
		Turn:    1,
		Data:    json.RawMessage(`{}`),
		Created: ts,
	}
	_, err = s.Append(sess.Header.ID, entry)
	require.NoError(t, err)

	loaded, err := s.Load(sess.Header.ID)
	require.NoError(t, err)
	assert.Equal(t, "custom-id", loaded.Entries[0].ID)
	assert.Equal(t, ts, loaded.Entries[0].Created)
}

func TestLoad_Roundtrip(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("/tmp/project")
	require.NoError(t, err)

	entries := []Entry{
		{Type: "message", Turn: 1, Data: json.RawMessage(`{"role":"user","content":"hello"}`)},
		{Type: "message", Turn: 2, Data: json.RawMessage(`{"role":"assistant","content":"world"}`)},
	}
	for _, e := range entries {
		_, err = s.Append(sess.Header.ID, e)
		require.NoError(t, err)
	}

	loaded, err := s.Load(sess.Header.ID)
	require.NoError(t, err)

	assert.Equal(t, sess.Header.ID, loaded.Header.ID)
	assert.Equal(t, "/tmp/project", loaded.Header.CWD)
	assert.Equal(t, sess.Header.Timestamp, loaded.Header.Timestamp)
	require.Len(t, loaded.Entries, 2)
}

func TestLoad_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Load("nonexistent")
	require.Error(t, err)
}

func TestHistory(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("/tmp/project")
	require.NoError(t, err)

	for i := range 3 {
		_, err = s.Append(sess.Header.ID, Entry{
			Type: "message",
			Turn: i + 1,
			Data: json.RawMessage(`{}`),
		})
		require.NoError(t, err)
	}

	history, err := s.History(sess.Header.ID)
	require.NoError(t, err)
	require.Len(t, history, 3)

	for i, entry := range history {
		assert.Equal(t, i+1, entry.Turn)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)

	sess1, err := s.Create("/tmp/proj1")
	require.NoError(t, err)

	sess2, err := s.Create("/tmp/proj2")
	require.NoError(t, err)

	for i := range 3 {
		_, err = s.Append(sess1.Header.ID, Entry{Type: "message", Turn: i + 1, Data: json.RawMessage(`{}`)})
		require.NoError(t, err)
	}

	_, err = s.Append(sess2.Header.ID, Entry{Type: "message", Turn: 1, Data: json.RawMessage(`{}`)})
	require.NoError(t, err)

	infos, err := s.List()
	require.NoError(t, err)
	require.Len(t, infos, 2)

	byID := map[string]SessionInfo{}
	for _, info := range infos {
		byID[info.ID] = info
	}

	info1 := byID[sess1.Header.ID]
	assert.Equal(t, "/tmp/proj1", info1.CWD)
	assert.Equal(t, 3, info1.EntryCount)
	assert.False(t, info1.CreatedAt.IsZero())
	assert.False(t, info1.UpdatedAt.IsZero())

	info2 := byID[sess2.Header.ID]
	assert.Equal(t, "/tmp/proj2", info2.CWD)
	assert.Equal(t, 1, info2.EntryCount)
}

func TestList_EmptyDir(t *testing.T) {
	s := newTestStore(t)
	infos, err := s.List()
	require.NoError(t, err)
	assert.Nil(t, infos)
}

func TestCompact_Truncation(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("/tmp/project")
	require.NoError(t, err)

	for i := range 5 {
		_, err = s.Append(sess.Header.ID, Entry{
			Type: "message",
			Turn: i + 1,
			Data: json.RawMessage(`{"role":"user","content":"msg"}`),
		})
		require.NoError(t, err)
	}

	require.NoError(t, s.Compact(sess.Header.ID, 2))

	loaded, err := s.Load(sess.Header.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Entries, 3)

	assert.Equal(t, "summary", loaded.Entries[0].Type)
	assert.Equal(t, 4, loaded.Entries[1].Turn)
	assert.Equal(t, 5, loaded.Entries[2].Turn)

	assert.Equal(t, loaded.Entries[0].ID, loaded.Entries[1].ParentID,
		"first kept entry should reference summary as parent")
}

func TestCompact_SummaryEntry(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("/tmp/project")
	require.NoError(t, err)

	for i := range 4 {
		_, err = s.Append(sess.Header.ID, Entry{
			Type: "message",
			Turn: i + 1,
			Data: json.RawMessage(`{}`),
		})
		require.NoError(t, err)
	}

	require.NoError(t, s.Compact(sess.Header.ID, 1))

	loaded, err := s.Load(sess.Header.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Entries, 2)

	summary := loaded.Entries[0]
	assert.Equal(t, "summary", summary.Type)
	assert.NotEmpty(t, summary.ID)
	assert.False(t, summary.Created.IsZero())
	assert.Contains(t, string(summary.Data), `"removed_count":3`)
	assert.Contains(t, string(summary.Data), `"first_turn":1`)
	assert.Contains(t, string(summary.Data), `"last_turn":3`)
}

func TestCompact_KeepLastExceedsTotal(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("/tmp/project")
	require.NoError(t, err)

	for i := range 3 {
		_, err = s.Append(sess.Header.ID, Entry{
			Type: "message",
			Turn: i + 1,
			Data: json.RawMessage(`{}`),
		})
		require.NoError(t, err)
	}

	require.NoError(t, s.Compact(sess.Header.ID, 10))

	loaded, err := s.Load(sess.Header.ID)
	require.NoError(t, err)
	assert.Len(t, loaded.Entries, 3, "should not modify file when keepLast >= total")
}

func TestCompact_PreservesHeader(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("/tmp/my-project")
	require.NoError(t, err)

	for i := range 3 {
		_, err = s.Append(sess.Header.ID, Entry{Type: "message", Turn: i + 1, Data: json.RawMessage(`{}`)})
		require.NoError(t, err)
	}

	require.NoError(t, s.Compact(sess.Header.ID, 1))

	loaded, err := s.Load(sess.Header.ID)
	require.NoError(t, err)
	assert.Equal(t, sess.Header.ID, loaded.Header.ID)
	assert.Equal(t, "/tmp/my-project", loaded.Header.CWD)
}

func TestList_IgnoresNonJSONL(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, os.WriteFile(filepath.Join(s.cfg.JSONL.Dir, "other.txt"), []byte("data"), 0o644))

	infos, err := s.List()
	require.NoError(t, err)
	assert.Nil(t, infos)
}

func TestSessionPath_InvalidID(t *testing.T) {
	s := newTestStore(t)
	_, err := s.sessionPath("../../etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid session ID")
}

func TestLoad_EmptyFile(t *testing.T) {
	s := newTestStore(t)
	path := filepath.Join(s.cfg.JSONL.Dir, "empty.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))
	_, err := loadFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty session file")
}

func TestList_SkipsCorruptedFile(t *testing.T) {
	s := newTestStore(t)

	sess, err := s.Create("/tmp/valid")
	require.NoError(t, err)
	_, err = s.Append(sess.Header.ID, Entry{Type: "message", Turn: 1, Data: json.RawMessage(`{}`)})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(s.cfg.JSONL.Dir, "corrupt.jsonl"), []byte("not json\n"), 0o644))

	infos, err := s.List()
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, sess.Header.ID, infos[0].ID)
}

func TestSubscribe_CreatesSessionOnPrompt(t *testing.T) {
	s := newTestStore(t)
	b := eventbus.New()

	s.Subscribe(b)
	defer s.Close()

	b.Publish(sdk.NewEvent("agent.prompt", "hello world"))
	time.Sleep(50 * time.Millisecond)

	s.mu.Lock()
	sessionID := s.sessionID
	s.mu.Unlock()

	require.NotEmpty(t, sessionID)

	sess, err := s.Load(sessionID)
	require.NoError(t, err)
	require.Len(t, sess.Entries, 1)

	assert.Equal(t, "message", sess.Entries[0].Type)
	assert.Equal(t, 1, sess.Entries[0].Turn)

	var data map[string]any
	require.NoError(t, json.Unmarshal(sess.Entries[0].Data, &data))
	assert.Equal(t, "user", data["role"])
	assert.Equal(t, "hello world", data["content"])
}

func TestSubscribe_AppendsAssistantOnMsgEnd(t *testing.T) {
	s := newTestStore(t)
	b := eventbus.New()

	s.Subscribe(b)
	defer s.Close()

	b.Publish(sdk.NewEvent("agent.prompt", "hello"))
	time.Sleep(50 * time.Millisecond)

	b.Publish(sdk.NewEvent("agent.turn_start", 1))
	b.Publish(sdk.NewEvent("agent.message_end", "hi there"))
	time.Sleep(50 * time.Millisecond)

	s.mu.Lock()
	sessionID := s.sessionID
	s.mu.Unlock()

	sess, err := s.Load(sessionID)
	require.NoError(t, err)
	require.Len(t, sess.Entries, 2)

	assert.Equal(t, "message", sess.Entries[1].Type)

	var data map[string]any
	require.NoError(t, json.Unmarshal(sess.Entries[1].Data, &data))
	assert.Equal(t, "assistant", data["role"])
	assert.Equal(t, "hi there", data["content"])

	assert.Equal(t, sess.Entries[0].ID, sess.Entries[1].ParentID)
}

func TestSubscribe_AppendsToolResult(t *testing.T) {
	s := newTestStore(t)
	b := eventbus.New()

	s.Subscribe(b)
	defer s.Close()

	b.Publish(sdk.NewEvent("agent.prompt", "list files"))
	time.Sleep(50 * time.Millisecond)

	b.Publish(sdk.NewEvent("agent.turn_start", 1))
	b.Publish(sdk.NewEvent("agent.message_end", "let me check"))
	time.Sleep(50 * time.Millisecond)

	b.Publish(sdk.NewEvent("agent.tool_result", map[string]any{
		"id":     "call-123",
		"tool":   "bash",
		"result": "file1.txt\nfile2.txt",
	}))
	time.Sleep(50 * time.Millisecond)

	s.mu.Lock()
	sessionID := s.sessionID
	s.mu.Unlock()

	sess, err := s.Load(sessionID)
	require.NoError(t, err)
	require.Len(t, sess.Entries, 3)

	assert.Equal(t, "message", sess.Entries[2].Type)

	var data map[string]any
	require.NoError(t, json.Unmarshal(sess.Entries[2].Data, &data))
	assert.Equal(t, "tool_result", data["role"])

	assert.Equal(t, sess.Entries[1].ID, sess.Entries[2].ParentID)
}

func TestSubscribe_EndStopsGoroutine(t *testing.T) {
	s := newTestStore(t)
	b := eventbus.New()

	s.Subscribe(b)

	b.Publish(sdk.NewEvent("agent.prompt", "hello"))
	time.Sleep(50 * time.Millisecond)

	b.Publish(sdk.NewEvent("agent.end", nil))

	require.NoError(t, s.Close())
}

func TestSubscribe_CloseCancelsGoroutine(t *testing.T) {
	s := newTestStore(t)
	b := eventbus.New()

	s.Subscribe(b)

	b.Publish(sdk.NewEvent("agent.prompt", "test"))
	time.Sleep(50 * time.Millisecond)

	require.NoError(t, s.Close())

	s.mu.Lock()
	sessionID := s.sessionID
	s.mu.Unlock()
	require.NotEmpty(t, sessionID)
}

type mockConfig struct {
	path string
}

func (m *mockConfig) FilePath() string                               { return m.path }
func (m *mockConfig) ProviderConfig(string) *sdk.ProviderConfigEntry { return nil }
func (m *mockConfig) ResolveKey(_, envVar string) (string, error)    { return os.Getenv(envVar), nil }
func (m *mockConfig) ToolConfig(string, any) error                   { return nil }
func (m *mockConfig) UIConfig(any) error                             { return nil }
func (m *mockConfig) IsHeadless() bool                               { return false }

func TestNewStore_LoadsNestedDir(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".weave.yaml")
	sessionDir := filepath.Join(dir, "sessions")
	configContent := "extensions:\n  - jsonl\njsonl:\n  dir: " + sessionDir + "\n"
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	s, err := NewStore(&mockConfig{path: configPath})
	require.NoError(t, err)

	got, err := s.sessionDir()
	require.NoError(t, err)
	assert.Equal(t, sessionDir, got)
}

func TestNewStore_DefaultDir(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".weave.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("extensions:\n  - jsonl\n"), 0o644))

	s, err := NewStore(&mockConfig{path: configPath})
	require.NoError(t, err)

	got, err := s.sessionDir()
	require.NoError(t, err)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".weave", "sessions"), got)
}
