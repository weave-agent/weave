package sdk

import (
	"context"
	"sync"
	"time"
)

// Throttle returns a throttled version of fn.
// The first call fires immediately. Subsequent calls within interval are
// deduplicated so that only the most recent call executes after the interval
// elapses. Calls after context cancellation are no-ops.
func Throttle(ctx context.Context, fn func(), interval time.Duration) func() {
	var (
		mu       sync.Mutex
		pending  bool
		lastExec time.Time
		timer    *time.Timer
		gen      int
	)

	return func() {
		mu.Lock()

		if ctx.Err() != nil {
			mu.Unlock()
			return
		}

		if lastExec.IsZero() || time.Since(lastExec) >= interval {
			lastExec = time.Now()
			pending = false
			mu.Unlock()
			fn()

			return
		}

		pending = true
		gen++
		myGen := gen

		if timer != nil {
			timer.Stop()
		}

		timer = time.AfterFunc(interval-time.Since(lastExec), func() {
			mu.Lock()
			defer mu.Unlock()

			if ctx.Err() != nil || !pending || gen != myGen {
				return
			}

			pending = false
			lastExec = time.Now()

			fn()
		})

		mu.Unlock()
	}
}
