package edit

import "sync"

// testMutex provides per-file locking to serialize concurrent mutations in tests.
// It mirrors the behavior of the production filemut.Mutex.
type testMutex struct {
	mu sync.Map // string -> *sync.Mutex
}

// newTestMutex creates a new testMutex.
func newTestMutex() *testMutex {
	return &testMutex{}
}

// Lock acquires the mutex for the given path and returns an unlock function.
func (fm *testMutex) Lock(path string) func() {
	actual, _ := fm.mu.LoadOrStore(path, &sync.Mutex{})
	mtx := actual.(*sync.Mutex)
	mtx.Lock()

	return mtx.Unlock
}
