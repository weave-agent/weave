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
		lastExec time.Time
		timer    *time.Timer
		gen      int
	)

	return func() {
		mu.Lock()
		defer mu.Unlock()

		if ctx.Err() != nil {
			return
		}

		elapsed := time.Since(lastExec)
		if lastExec.IsZero() || elapsed >= interval {
			lastExec = time.Now()

			fn()

			return
		}

		gen++
		myGen := gen

		if timer != nil {
			timer.Stop()
		}

		timer = time.AfterFunc(interval-elapsed, func() {
			mu.Lock()
			defer mu.Unlock()

			if ctx.Err() != nil || gen != myGen {
				return
			}

			defer func() {
				if r := recover(); r != nil {
					_ = r // Swallow panic to prevent crashing the process
					// from a timer goroutine.
				}
			}()

			lastExec = time.Now()

			fn()
		})
	}
}
