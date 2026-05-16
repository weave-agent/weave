package sdk

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSessionStore is a minimal SessionStore implementation for testing.
type mockSessionStore struct {
	listSessionsResult []SessionInfo
	listSessionsErr    error
	loadHistoryResult  []Message
	loadHistoryErr     error
	loadedSessionID    string
}

func (m *mockSessionStore) ListSessions() ([]SessionInfo, error) {
	return m.listSessionsResult, m.listSessionsErr
}

func (m *mockSessionStore) LoadHistory(sessionID string) ([]Message, error) {
	m.loadedSessionID = sessionID

	return m.loadHistoryResult, m.loadHistoryErr
}

func TestGetSessionStore_NilDefault(t *testing.T) {
	SetSessionStore(nil)

	assert.Nil(t, GetSessionStore())
}

func TestSetSessionStore_SetAndGet(t *testing.T) {
	mock := &mockSessionStore{listSessionsResult: []SessionInfo{{ID: "s1"}}}

	SetSessionStore(mock)
	defer SetSessionStore(nil)

	got := GetSessionStore()
	assert.NotNil(t, got)

	sessions, err := got.ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "s1", sessions[0].ID)
}

func TestSetSessionStore_Overwrite(t *testing.T) {
	first := &mockSessionStore{listSessionsResult: []SessionInfo{{ID: "first"}}}
	second := &mockSessionStore{listSessionsResult: []SessionInfo{{ID: "second"}}}

	SetSessionStore(first)

	sessions, _ := GetSessionStore().ListSessions()
	assert.Equal(t, "first", sessions[0].ID)

	SetSessionStore(second)

	sessions, _ = GetSessionStore().ListSessions()
	assert.Equal(t, "second", sessions[0].ID)

	SetSessionStore(nil)

	assert.Nil(t, GetSessionStore())
}

func TestSessionStoreInterfaceMethods(t *testing.T) {
	mock := &mockSessionStore{}

	SetSessionStore(mock)
	defer SetSessionStore(nil)

	ss := GetSessionStore()
	a := assert.New(t)

	mock.listSessionsResult = []SessionInfo{
		{ID: "abc", CWD: "/tmp", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	sessions, err := ss.ListSessions()
	require.NoError(t, err)
	a.Len(sessions, 1)
	a.Equal("abc", sessions[0].ID)
	a.Equal("/tmp", sessions[0].CWD)

	mock.listSessionsErr = errors.New("list error")

	_, err = ss.ListSessions()
	require.EqualError(t, err, "list error")

	mock.listSessionsErr = nil
	mock.loadHistoryResult = []Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "hi"},
	}

	msgs, err := ss.LoadHistory("abc")
	require.NoError(t, err)
	a.Len(msgs, 2)
	a.Equal("abc", mock.loadedSessionID)
	a.Equal(RoleUser, msgs[0].Role)

	mock.loadHistoryErr = errors.New("load error")

	_, err = ss.LoadHistory("abc")
	require.EqualError(t, err, "load error")
}

func TestNoopSessionStore_ListSessions(t *testing.T) {
	noop := NoopSessionStore{}

	sessions, err := noop.ListSessions()
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestNoopSessionStore_LoadHistory(t *testing.T) {
	noop := NoopSessionStore{}

	msgs, err := noop.LoadHistory("any-id")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestSessionResumePayload(t *testing.T) {
	payload := SessionResumePayload{
		SessionID: "session-1",
		Messages: []Message{
			{Role: RoleUser, Content: "hello"},
		},
	}

	assert.Equal(t, "session-1", payload.SessionID)
	assert.Len(t, payload.Messages, 1)
}
