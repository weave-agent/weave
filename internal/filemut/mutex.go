// Package filemut provides per-file mutexes for serializing concurrent
// file mutations (edits, writes) to the same path.
package filemut

import "sync"

// Mutex provides per-file locking to serialize concurrent mutations.
// It uses a sync.Map of *sync.Mutex, creating mutexes lazily per path.
// Mutexes are never removed (bounded by number of unique files in session).
type Mutex struct {
	mu sync.Map // string -> *sync.Mutex
}

// New creates a new Mutex.
func New() *Mutex {
	return &Mutex{}
}

// Lock acquires the mutex for the given path and returns an unlock function.
// The caller should typically defer the unlock:
//
//	defer fm.Lock(path)()
func (fm *Mutex) Lock(path string) func() {
	actual, _ := fm.mu.LoadOrStore(path, &sync.Mutex{})
	mtx := actual.(*sync.Mutex)
	mtx.Lock()

	return mtx.Unlock
}
