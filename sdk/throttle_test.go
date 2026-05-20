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

	// Multiple calls — only one should fire after interval.
	for range 5 {
		throttled()
		time.Sleep(10 * time.Millisecond)
	}

	assert.Equal(t, int32(1), fireCount.Load())

	// Wait for the single deferred execution.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(2), fireCount.Load())
}

func TestThrottle_MultipleCallsWithinInterval_FiresOnce(t *testing.T) {
	ctx := t.Context()

	var count atomic.Int32

	throttled := Throttle(ctx, func() {
		count.Add(1)
	}, 100*time.Millisecond)

	throttled() // fires at t=0
	assert.Equal(t, int32(1), count.Load())

	// Multiple calls within the interval — only one deferred execution.
	throttled()
	time.Sleep(20 * time.Millisecond)
	throttled()
	time.Sleep(20 * time.Millisecond)
	throttled()
	assert.Equal(t, int32(1), count.Load())

	// Wait for the single deferred execution.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(2), count.Load())
}
