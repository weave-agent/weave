package sdk

import (
	"sync"
	"time"
)

// SessionStore provides session persistence and retrieval operations.
type SessionStore interface {
	// ListSessions returns all available sessions sorted by recency.
	ListSessions() ([]SessionInfo, error)

	// LoadHistory loads the message history for the given session ID.
	LoadHistory(sessionID string) ([]Message, error)
}

// SessionInfo describes a persisted session.
type SessionInfo struct {
	ID        string
	CWD       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SessionResumePayload is the payload for the "session.resume" bus event.
type SessionResumePayload struct {
	SessionID string
	Messages  []Message
}

var (
	sessionStoreMu sync.RWMutex
	sessionStore   SessionStore
)

// SetSessionStore registers the global SessionStore instance.
func SetSessionStore(ss SessionStore) {
	sessionStoreMu.Lock()
	sessionStore = ss
	sessionStoreMu.Unlock()
}

// GetSessionStore returns the global SessionStore, or nil if none is registered.
func GetSessionStore() SessionStore {
	sessionStoreMu.RLock()
	defer sessionStoreMu.RUnlock()

	return sessionStore
}

// NoopSessionStore is a zero-value SessionStore that returns empty results.
type NoopSessionStore struct{}

// ListSessions returns an empty slice.
func (NoopSessionStore) ListSessions() ([]SessionInfo, error) {
	return nil, nil
}

// LoadHistory returns an empty slice.
func (NoopSessionStore) LoadHistory(_ string) ([]Message, error) {
	return nil, nil
}
