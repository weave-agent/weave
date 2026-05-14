package filemut

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	fm := New()
	assert.NotNil(t, fm)
}

func TestLockUnlock(t *testing.T) {
	fm := New()
	unlock := fm.Lock("/tmp/test.txt")
	assert.NotNil(t, unlock)
	unlock()
}

func TestLockSerializesSamePath(t *testing.T) {
	fm := New()
	path := "/tmp/shared.txt"

	var counter atomic.Int32

	var wg sync.WaitGroup

	for range 100 {
		wg.Go(func() {
			defer fm.Lock(path)()

			counter.Add(1)
			// If not serialized, we'd see races on counter.
		})
	}

	wg.Wait()
	assert.Equal(t, int32(100), counter.Load())
}

func TestLockDifferentPathsAreIndependent(t *testing.T) {
	fm := New()

	// Lock two different paths from the same goroutine.
	unlock1 := fm.Lock("/tmp/a.txt")
	unlock2 := fm.Lock("/tmp/b.txt")

	// Both should be acquirable simultaneously (different paths = different mutexes).
	unlock2()
	unlock1()
}

func TestLockSamePathBlocks(t *testing.T) {
	fm := New()
	path := "/tmp/block.txt"

	var secondAcquired atomic.Bool

	unlock1 := fm.Lock(path)

	var wg sync.WaitGroup

	wg.Go(func() {
		defer fm.Lock(path)()

		secondAcquired.Store(true)
	})

	// Give the goroutine time to try acquiring.
	time.Sleep(10 * time.Millisecond)
	assert.False(t, secondAcquired.Load(), "second lock on same path should block")

	unlock1()
	wg.Wait()
	assert.True(t, secondAcquired.Load(), "second lock should be acquired after first unlock")
}

func TestLockSamePathReusesMutex(t *testing.T) {
	fm := New()
	path := "/tmp/reuse.txt"

	unlock1 := fm.Lock(path)
	unlock1()

	// Second lock on same path should reuse the same underlying mutex.
	unlock2 := fm.Lock(path)
	unlock2()
}
