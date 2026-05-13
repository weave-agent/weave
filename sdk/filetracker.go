package sdk

import (
	"sync"
	"time"
)

// FileTracker tracks which files have been read and their mod times at the time
// of reading. Tools use it to enforce read-before-edit policies.
type FileTracker interface {
	// RecordRead records that a file was read at the given mod time.
	RecordRead(path string, modTime time.Time)

	// WasRead reports whether the given path has been recorded as read.
	WasRead(path string) bool

	// GetReadTime returns the recorded mod time for a path and whether it was found.
	GetReadTime(path string) (time.Time, bool)
}

var (
	fileTrackerMu sync.RWMutex
	fileTracker   FileTracker
)

// SetFileTracker registers the global FileTracker instance.
func SetFileTracker(ft FileTracker) {
	fileTrackerMu.Lock()
	fileTracker = ft
	fileTrackerMu.Unlock()
}

// GetFileTracker returns the global FileTracker, or nil if none is registered.
func GetFileTracker() FileTracker {
	fileTrackerMu.RLock()
	defer fileTrackerMu.RUnlock()

	return fileTracker
}
