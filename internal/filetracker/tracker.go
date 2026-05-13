package filetracker

import (
	"sync"
	"time"
)

// Tracker tracks which files have been read, storing the last known mod time
// for staleness checks. It is safe for concurrent use.
// It implements the sdk.FileTracker interface.
type Tracker struct {
	mu    sync.RWMutex
	reads map[string]time.Time
}

// New creates a new Tracker.
func New() *Tracker {
	return &Tracker{
		reads: make(map[string]time.Time),
	}
}

// RecordRead records that a file was read at the given mod time.
func (ft *Tracker) RecordRead(path string, modTime time.Time) {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	ft.reads[path] = modTime
}

// WasRead reports whether the given path has been recorded as read.
func (ft *Tracker) WasRead(path string) bool {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	_, ok := ft.reads[path]

	return ok
}

// GetReadTime returns the recorded mod time for a path and whether it was found.
func (ft *Tracker) GetReadTime(path string) (time.Time, bool) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	t, ok := ft.reads[path]

	return t, ok
}
