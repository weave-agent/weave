package sdk

import "sync"

// FileMuter provides per-file locking for serializing concurrent mutations.
type FileMuter interface {
	// Lock acquires the mutex for the given path and returns an unlock function.
	Lock(path string) func()
}

var (
	fileMutexMu sync.RWMutex
	fileMutex   FileMuter
)

// SetFileMutex registers the global FileMuter instance.
func SetFileMutex(fm FileMuter) {
	fileMutexMu.Lock()
	fileMutex = fm
	fileMutexMu.Unlock()
}

// GetFileMutex returns the global FileMuter, or nil if none is registered.
func GetFileMutex() FileMuter {
	fileMutexMu.RLock()
	defer fileMutexMu.RUnlock()

	return fileMutex
}
