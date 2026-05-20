package sdk

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestThrottle_FirstCallFiresImmediately(t *testing.T) {
	ctx := t.Context()

	var count atomic.Int32

	throttled := Throttle(ctx, func() {
		count.Add(1)
	}, 100*time.Millisecond)

	throttled()
	assert.Equal(t, int32(1), count.Load())
}

func TestThrottle_DeduplicatesWithinInterval(t *testing.T) {
	ctx := t.Context()

	var count atomic.Int32

	throttled := Throttle(ctx, func() {
		count.Add(1)
	}, 100*time.Millisecond)

	// First call fires immediately.
	throttled()
	assert.Equal(t, int32(1), count.Load())

	// Rapid calls within the interval should be deduplicated.
	throttled()
	throttled()
	throttled()
	assert.Equal(t, int32(1), count.Load())

	// Wait for the deferred call to fire.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(2), count.Load())
}

func TestThrottle_ResetsAfterInterval(t *testing.T) {
	ctx := t.Context()

	var count atomic.Int32

	throttled := Throttle(ctx, func() {
		count.Add(1)
	}, 50*time.Millisecond)

	throttled()
	assert.Equal(t, int32(1), count.Load())

	// Wait for the interval to pass, then call again.
	time.Sleep(60 * time.Millisecond)
	throttled()
	assert.Equal(t, int32(2), count.Load())
}

func TestThrottle_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var count atomic.Int32

	throttled := Throttle(ctx, func() {
		count.Add(1)
	}, 100*time.Millisecond)

	// First call fires.
	throttled()
	assert.Equal(t, int32(1), count.Load())

	// Queue a pending call.
	throttled()
	assert.Equal(t, int32(1), count.Load())

	// Cancel context before the deferred call fires.
	cancel()
	time.Sleep(150 * time.Millisecond)

	// The deferred call should not have fired.
	assert.Equal(t, int32(1), count.Load())

	// Subsequent calls are no-ops.
	throttled()
	assert.Equal(t, int32(1), count.Load())
}

func TestThrottle_ConcurrentCalls(t *testing.T) {
	ctx := t.Context()

	var count atomic.Int32

	throttled := Throttle(ctx, func() {
		count.Add(1)
	}, 100*time.Millisecond)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			throttled()
		})
	}

	wg.Wait()

	// Only one call should have fired immediately; others deduplicated.
	assert.Equal(t, int32(1), count.Load())

	// Wait for the deferred call.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(2), count.Load())
}

func TestThrottle_LatestCallWins(t *testing.T) {
	ctx := t.Context()

	var fireCount atomic.Int32

	throttled := Throttle(ctx, func() {
		fireCount.Add(1)
	}, 100*time.Millisecond)

	throttled()
	assert.Equal(t, int32(1), fireCount.Load())

	// Multiple calls spaced within the interval — only one deferred execution.
	for range 5 {
		throttled()
		time.Sleep(10 * time.Millisecond)
	}

	assert.Equal(t, int32(1), fireCount.Load())

	// Wait for the single deferred execution.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(2), fireCount.Load())
}

func TestThrottle_ZeroInterval(t *testing.T) {
	ctx := t.Context()

	var count atomic.Int32

	throttled := Throttle(ctx, func() {
		count.Add(1)
	}, 0)

	throttled()
	throttled()
	throttled()

	assert.Equal(t, int32(3), count.Load())
}

func TestThrottle_PanicInTimerCallback(t *testing.T) {
	ctx := t.Context()

	var count atomic.Int32

	throttled := Throttle(ctx, func() {
		count.Add(1)

		if count.Load() == 2 {
			panic("intentional panic")
		}
	}, 50*time.Millisecond)

	throttled() // fires immediately, count=1
	assert.Equal(t, int32(1), count.Load())

	throttled() // scheduled, will panic when fired
	time.Sleep(200 * time.Millisecond)

	// The panic was swallowed; throttle should still work.
	assert.Equal(t, int32(2), count.Load())

	// Enough time has passed; should fire immediately.
	throttled()
	assert.Equal(t, int32(3), count.Load())
}
