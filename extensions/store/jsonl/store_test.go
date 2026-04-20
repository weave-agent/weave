package jsonlstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return &Store{cfg: Config{Dir: dir}}
}

func TestGenerateID(t *testing.T) {
	id, err := generateID()
	require.NoError(t, err)
	assert.Len(t, id, 8)

	id2, err := generateID()
	require.NoError(t, err)
	assert.NotEqual(t, id, id2)
}

func TestCreate(t *testing.T) {
	s := newTestStore(t)

	sess, err := s.Create("/tmp/project")
	require.NoError(t, err)

	assert.Equal(t, "session", sess.Header.Type)
	assert.Len(t, sess.Header.ID, 8)
	assert.Equal(t, "/tmp/project", sess.Header.CWD)
	assert.False(t, sess.Header.Timestamp.IsZero())

	path := filepath.Join(s.cfg.Dir, sess.Header.ID+".jsonl")
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
	require.NoError(t, s.Append(sess.Header.ID, entry))

	entry2 := Entry{
		Type: "message",
		Turn: 2,
		Data: json.RawMessage(`{"role":"assistant","content":"hi"}`),
	}
	require.NoError(t, s.Append(sess.Header.ID, entry2))

	loaded, err := s.Load(sess.Header.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Entries, 2)

	assert.Len(t, loaded.Entries[0].ID, 8)
	assert.False(t, loaded.Entries[0].Created.IsZero())
	assert.Equal(t, 1, loaded.Entries[0].Turn)
	assert.Equal(t, `{"role":"user","content":"hello"}`, string(loaded.Entries[0].Data))

	assert.Len(t, loaded.Entries[1].ID, 8)
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
	require.NoError(t, s.Append(sess.Header.ID, entry))

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
		require.NoError(t, s.Append(sess.Header.ID, e))
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

	for i := 0; i < 3; i++ {
		require.NoError(t, s.Append(sess.Header.ID, Entry{
			Type: "message",
			Turn: i + 1,
			Data: json.RawMessage(`{}`),
		}))
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

	for i := 0; i < 3; i++ {
		require.NoError(t, s.Append(sess1.Header.ID, Entry{Type: "message", Turn: i + 1, Data: json.RawMessage(`{}`)}))
	}
	require.NoError(t, s.Append(sess2.Header.ID, Entry{Type: "message", Turn: 1, Data: json.RawMessage(`{}`)}))

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

	for i := 0; i < 5; i++ {
		require.NoError(t, s.Append(sess.Header.ID, Entry{
			Type: "message",
			Turn: i + 1,
			Data: json.RawMessage(`{"role":"user","content":"msg"}`),
		}))
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

	for i := 0; i < 4; i++ {
		require.NoError(t, s.Append(sess.Header.ID, Entry{
			Type: "message",
			Turn: i + 1,
			Data: json.RawMessage(`{}`),
		}))
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

	for i := 0; i < 3; i++ {
		require.NoError(t, s.Append(sess.Header.ID, Entry{
			Type: "message",
			Turn: i + 1,
			Data: json.RawMessage(`{}`),
		}))
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

	for i := 0; i < 3; i++ {
		require.NoError(t, s.Append(sess.Header.ID, Entry{Type: "message", Turn: i + 1, Data: json.RawMessage(`{}`)}))
	}

	require.NoError(t, s.Compact(sess.Header.ID, 1))

	loaded, err := s.Load(sess.Header.ID)
	require.NoError(t, err)
	assert.Equal(t, sess.Header.ID, loaded.Header.ID)
	assert.Equal(t, "/tmp/my-project", loaded.Header.CWD)
}

func TestList_IgnoresNonJSONL(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, os.WriteFile(filepath.Join(s.cfg.Dir, "other.txt"), []byte("data"), 0o644))

	infos, err := s.List()
	require.NoError(t, err)
	assert.Nil(t, infos)
}
