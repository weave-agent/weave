package bus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weave-agent/weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnSingleHandler(t *testing.T) {
	b := New()
	defer b.Close()

	var received atomic.Value

	b.On("test.topic", func(e sdk.Event) error {
		received.Store(e)
		return nil
	})

	e := sdk.NewEvent("test.topic", "hello")
	b.Publish(e)

	assert.Eventually(t, func() bool {
		v, ok := received.Load().(sdk.Event)
		return ok && v.Topic == "test.topic" && v.Payload == "hello"
	}, time.Second, 10*time.Millisecond)
}

func TestOnMultipleHandlersSameTopic(t *testing.T) {
	b := New()
	defer b.Close()

	var count atomic.Int32

	handler := func(e sdk.Event) error {
		count.Add(1)
		return nil
	}

	b.On("topic", handler)
	// Register a different handler too
	b.On("topic", func(e sdk.Event) error {
		count.Add(1)
		return nil
	})

	b.Publish(sdk.NewEvent("topic", nil))

	assert.Eventually(t, func() bool {
		return count.Load() == 2
	}, time.Second, 10*time.Millisecond)
}

func TestOnAll(t *testing.T) {
	b := New()
	defer b.Close()

	var received atomic.Value

	b.OnAll(func(e sdk.Event) error {
		received.Store(e)
		return nil
	})

	b.Publish(sdk.NewEvent("any.topic", "data"))

	assert.Eventually(t, func() bool {
		v, ok := received.Load().(sdk.Event)
		return ok && v.Topic == "any.topic"
	}, time.Second, 10*time.Millisecond)
}

func TestUnrelatedTopicNotReceived(t *testing.T) {
	b := New()
	defer b.Close()

	called := atomic.Bool{}

	b.On("alpha", func(e sdk.Event) error {
		called.Store(true)
		return nil
	})

	b.Publish(sdk.NewEvent("beta", nil))

	time.Sleep(50 * time.Millisecond)
	assert.False(t, called.Load(), "handler for 'alpha' should not receive 'beta' events")
}

func TestPanicRecovery(t *testing.T) {
	b := New()

	var panicEvent atomic.Value

	panicTopic := b.DiagnosticTopics[0]
	b.On(panicTopic, func(e sdk.Event) error {
		panicEvent.Store(e)
		return nil
	})

	b.On("crash.topic", func(e sdk.Event) error {
		panic("something broke")
	})

	b.Publish(sdk.NewEvent("crash.topic", nil))

	assert.Eventually(t, func() bool {
		v, ok := panicEvent.Load().(sdk.Event)
		return ok && v.Topic == panicTopic
	}, 2*time.Second, 10*time.Millisecond, "expected panic diagnostic event")

	// Bus should still be usable after panic
	require.NoError(t, b.Close())
}

func TestErrorHandler(t *testing.T) {
	b := New()

	var errEvent atomic.Value

	errTopic := b.DiagnosticTopics[1]
	b.On(errTopic, func(e sdk.Event) error {
		errEvent.Store(e)
		return nil
	})

	b.On("fail.topic", func(e sdk.Event) error {
		return assert.AnError
	})

	b.Publish(sdk.NewEvent("fail.topic", nil))

	assert.Eventually(t, func() bool {
		v, ok := errEvent.Load().(sdk.Event)
		return ok && v.Topic == errTopic
	}, 2*time.Second, 10*time.Millisecond, "expected error diagnostic event")

	require.NoError(t, b.Close())
}

func TestOff(t *testing.T) {
	b := New()
	defer b.Close()

	called := atomic.Bool{}
	h := func(e sdk.Event) error {
		called.Store(true)
		return nil
	}

	b.On("topic", h)
	b.Off(h)

	b.Publish(sdk.NewEvent("topic", nil))

	time.Sleep(50 * time.Millisecond)
	assert.False(t, called.Load(), "handler should not be called after Off")
}

func TestOffAllHandler(t *testing.T) {
	b := New()
	defer b.Close()

	called := atomic.Bool{}
	h := func(e sdk.Event) error {
		called.Store(true)
		return nil
	}

	b.OnAll(h)
	b.Off(h)

	b.Publish(sdk.NewEvent("any", nil))

	time.Sleep(50 * time.Millisecond)
	assert.False(t, called.Load(), "OnAll handler should not be called after Off")
}

func TestOffUnknownHandler(t *testing.T) {
	b := New()
	defer b.Close()

	h := func(e sdk.Event) error { return nil }
	b.Off(h) // should not panic
}

func TestCloseWaitsForHandlers(t *testing.T) {
	b := New()

	started := make(chan struct{})
	completed := atomic.Bool{}

	b.On("slow", func(e sdk.Event) error {
		close(started)
		time.Sleep(100 * time.Millisecond)
		completed.Store(true)

		return nil
	})

	b.Publish(sdk.NewEvent("slow", nil))
	<-started // wait for handler to start

	require.NoError(t, b.Close())
	assert.True(t, completed.Load(), "Close should wait for handlers to finish")
}

func TestCloseDrainsAndStops(t *testing.T) {
	b := New()

	called := atomic.Bool{}

	b.On("x", func(e sdk.Event) error {
		called.Store(true)
		return nil
	})

	require.NoError(t, b.Close())

	// After close, new publishes should be no-ops (not panic)
	b.Publish(sdk.NewEvent("x", nil))
	assert.False(t, called.Load(), "handler should not be called after Close")
}

func TestOnAfterClose(t *testing.T) {
	b := New()
	require.NoError(t, b.Close())

	called := atomic.Bool{}

	b.On("x", func(e sdk.Event) error {
		called.Store(true)
		return nil
	})

	b.Publish(sdk.NewEvent("x", nil))
	time.Sleep(20 * time.Millisecond)
	assert.False(t, called.Load(), "On after Close should not register")
}

func TestPublishAfterClose(t *testing.T) {
	b := New()
	require.NoError(t, b.Close())

	// Publish after close should not panic
	b.Publish(sdk.NewEvent("x", nil))
}

func TestPublishWithNoHandlers(t *testing.T) {
	b := New()
	defer b.Close()

	// Publish with no subscribers should not panic
	b.Publish(sdk.NewEvent("nobody", nil))
}

func TestConcurrentPublish(t *testing.T) {
	b := New()
	defer b.Close()

	var count atomic.Int32

	b.On("concurrent", func(e sdk.Event) error {
		count.Add(1)
		return nil
	})

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			b.Publish(sdk.NewEvent("concurrent", i))
		})
	}

	wg.Wait()

	assert.Eventually(t, func() bool {
		return count.Load() > 0
	}, time.Second, 10*time.Millisecond, "expected some events from concurrent publishes")
}

func TestConcurrentPublishAndClose(t *testing.T) {
	for range 50 {
		b := New()
		b.On("race", func(e sdk.Event) error { return nil })

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()

			for range 200 {
				b.Publish(sdk.NewEvent("race", nil))
			}
		}()

		go func() {
			defer wg.Done()

			_ = b.Close()
		}()

		wg.Wait()
	}
}

func TestCloseIdempotent(t *testing.T) {
	b := New()
	require.NoError(t, b.Close())
	require.NoError(t, b.Close())
}

func TestOffConcurrentPublish(t *testing.T) {
	for range 50 {
		b := New()

		var count atomic.Int32

		h := func(_ sdk.Event) error { //nolint:unparam // bus handler signature
			count.Add(1)
			return nil
		}

		b.On("topic", h)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()

			for range 200 {
				b.Publish(sdk.NewEvent("topic", "data"))
			}
		}()

		go func() {
			defer wg.Done()

			b.Off(h)
		}()

		wg.Wait()
		require.NoError(t, b.Close())
	}
}

func TestPublishDiagnosticRecoverOnPanic(t *testing.T) {
	b := New()

	var panicCount atomic.Int32

	// Diagnostic handler that panics
	errTopic := b.DiagnosticTopics[1]
	b.On(errTopic, func(e sdk.Event) error {
		panicCount.Add(1)
		panic("diagnostic handler broke")
	})

	// Another diagnostic handler that should still be reached despite the panic
	// (the panic is recovered by invokeHandler, not publishDiagnostic)
	var secondReceived atomic.Bool

	b.On(errTopic, func(e sdk.Event) error {
		secondReceived.Store(true)
		return nil
	})

	// Normal handler that returns an error, triggering publishDiagnostic
	b.On("fail.topic", func(e sdk.Event) error {
		return assert.AnError
	})

	b.Publish(sdk.NewEvent("fail.topic", nil))

	// Bus should not deadlock and the second handler should eventually receive
	assert.Eventually(t, secondReceived.Load, 2*time.Second, 10*time.Millisecond, "second error diagnostic handler should receive event despite first handler panicking")

	require.NoError(t, b.Close())
}

func TestPublishDiagnosticRecoverOnClosedBus(t *testing.T) {
	b := New()

	// Verifies that publishDiagnostic handles a closed bus gracefully
	// when a handler triggers close and then returns an error.
	// Close() blocks until the handler finishes, so we close in a goroutine
	// to avoid deadlock.
	completed := atomic.Bool{}

	b.On("test", func(e sdk.Event) error {
		go b.Close()

		completed.Store(true)
		// Error triggers publishDiagnostic on a now-closed bus
		return assert.AnError
	})

	b.Publish(sdk.NewEvent("test", nil))

	assert.Eventually(t, completed.Load, 2*time.Second, 10*time.Millisecond,
		"handler should have completed without deadlocking")
}

func TestCollectSubscribers(t *testing.T) {
	b := New()
	defer b.Close()

	// Register topic handlers and OnAll handlers
	var topicCount, allCount atomic.Int32

	b.On("alpha", func(e sdk.Event) error {
		topicCount.Add(1)
		return nil
	})

	b.On("alpha", func(e sdk.Event) error {
		topicCount.Add(1)
		return nil
	})

	b.OnAll(func(e sdk.Event) error {
		allCount.Add(1)
		return nil
	})

	// Publish to alpha should reach both topic handlers + OnAll
	b.Publish(sdk.NewEvent("alpha", nil))

	assert.Eventually(t, func() bool {
		return topicCount.Load() == 2 && allCount.Load() == 1
	}, time.Second, 10*time.Millisecond)

	// Publish to unknown topic should only reach OnAll
	topicCount.Store(0)
	allCount.Store(0)

	b.Publish(sdk.NewEvent("beta", nil))

	assert.Eventually(t, func() bool {
		return topicCount.Load() == 0 && allCount.Load() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestOffPreventsNewDeliveries(t *testing.T) {
	b := New()
	defer b.Close()

	var count atomic.Int32

	h := func(e sdk.Event) error {
		count.Add(1)
		return nil
	}

	b.On("topic", h)

	// Publish before Off
	b.Publish(sdk.NewEvent("topic", nil))
	assert.Eventually(t, func() bool {
		return count.Load() >= 1
	}, time.Second, 10*time.Millisecond)

	b.Off(h)

	// Publish after Off
	b.Publish(sdk.NewEvent("topic", nil))
	time.Sleep(50 * time.Millisecond)

	finalCount := count.Load()
	assert.Equal(t, int32(1), finalCount, "handler should receive exactly 1 event (before Off), not %d", finalCount)
}

func TestPublishDropsWhenBufferFull(t *testing.T) {
	b := New()

	var received atomic.Int32

	// Handler that blocks, filling its channel buffer
	block := make(chan struct{})

	b.On("flood", func(e sdk.Event) error {
		<-block
		received.Add(1)

		return nil
	})

	// Publish more events than the buffer can hold (topicHandlerBufSize = 64)
	for i := range topicHandlerBufSize + 20 {
		b.Publish(sdk.NewEvent("flood", i))
	}

	// Unblock the handler — it should only have processed buffer + in-flight
	close(block)

	assert.Eventually(t, func() bool {
		return received.Load() > 0
	}, time.Second, 10*time.Millisecond, "handler should process at least some events")

	require.NoError(t, b.Close())
}

func TestPanicOnDiagnosticTopicNoRecurse(t *testing.T) {
	b := New()

	var panicCount atomic.Int32

	panicTopic := b.DiagnosticTopics[0]

	// Handler on panic diagnostic topic that itself panics
	b.On(panicTopic, func(e sdk.Event) error {
		panicCount.Add(1)
		panic("diagnostic panic")
	})

	// Trigger a panic that generates a panic diagnostic event
	b.On("trigger", func(e sdk.Event) error {
		panic("original panic")
	})

	b.Publish(sdk.NewEvent("trigger", nil))

	// Should receive exactly one panic diagnostic event — no recursion
	assert.Eventually(t, func() bool {
		return panicCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "expected exactly 1 panic diagnostic event, no recursion")

	// Bus should still be usable
	var after atomic.Bool

	b.On("after", func(e sdk.Event) error {
		after.Store(true)
		return nil
	})

	b.Publish(sdk.NewEvent("after", nil))
	assert.Eventually(t, after.Load, time.Second, 10*time.Millisecond)

	require.NoError(t, b.Close())
}

func TestErrorOnDiagnosticTopicNoRecurse(t *testing.T) {
	b := New()

	var errCount atomic.Int32

	errTopic := b.DiagnosticTopics[1]

	// Handler on error diagnostic topic that itself returns an error
	b.On(errTopic, func(e sdk.Event) error {
		errCount.Add(1)
		return assert.AnError
	})

	// Trigger an error that generates an error diagnostic event
	b.On("trigger", func(e sdk.Event) error {
		return assert.AnError
	})

	b.Publish(sdk.NewEvent("trigger", nil))

	assert.Eventually(t, func() bool {
		return errCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "expected exactly 1 error diagnostic event, no recursion")

	require.NoError(t, b.Close())
}

func TestOffDuringHandlerExecution(t *testing.T) {
	b := New()

	started := make(chan struct{})

	var completed atomic.Bool

	h := func(e sdk.Event) error {
		close(started)
		time.Sleep(100 * time.Millisecond)
		completed.Store(true)

		return nil
	}

	b.On("topic", h)
	b.Publish(sdk.NewEvent("topic", nil))
	<-started

	// Off while handler is running should return immediately
	b.Off(h)

	assert.Eventually(t, completed.Load, 2*time.Second, 10*time.Millisecond,
		"in-flight handler should complete after Off")

	require.NoError(t, b.Close())
}

func TestOnAllAfterClose(t *testing.T) {
	b := New()
	require.NoError(t, b.Close())

	called := atomic.Bool{}

	b.OnAll(func(e sdk.Event) error {
		called.Store(true)
		return nil
	})

	b.Publish(sdk.NewEvent("any", nil))
	time.Sleep(20 * time.Millisecond)
	assert.False(t, called.Load(), "OnAll after Close should not register")
}

func TestOffAfterClose(t *testing.T) {
	b := New()

	h := func(e sdk.Event) error { return nil }
	b.On("topic", h)

	require.NoError(t, b.Close())

	// Off after Close should not panic
	b.Off(h)
}

func TestMultipleOnAllOffOne(t *testing.T) {
	b := New()
	defer b.Close()

	var count1, count2 atomic.Int32

	h1 := func(e sdk.Event) error {
		count1.Add(1)
		return nil
	}

	h2 := func(e sdk.Event) error {
		count2.Add(1)
		return nil
	}

	b.OnAll(h1)
	b.OnAll(h2)

	b.Publish(sdk.NewEvent("topic", nil))

	assert.Eventually(t, func() bool {
		return count1.Load() >= 1 && count2.Load() >= 1
	}, time.Second, 10*time.Millisecond)

	b.Off(h1)

	count1.Store(0)
	count2.Store(0)

	b.Publish(sdk.NewEvent("topic", nil))
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, int32(0), count1.Load(), "removed OnAll handler should not receive events")
	assert.Equal(t, int32(1), count2.Load(), "remaining OnAll handler should still receive events")
}

func TestSameHandlerMultipleTopics(t *testing.T) {
	b := New()
	defer b.Close()

	var count atomic.Int32

	h := func(e sdk.Event) error {
		count.Add(1)
		return nil
	}

	b.On("alpha", h)
	b.On("beta", h)

	b.Publish(sdk.NewEvent("alpha", nil))
	b.Publish(sdk.NewEvent("beta", nil))

	assert.Eventually(t, func() bool {
		return count.Load() == 2
	}, time.Second, 10*time.Millisecond)

	b.Off(h)

	var afterOff atomic.Int32

	b.On("gamma", func(e sdk.Event) error {
		afterOff.Add(1)
		return nil
	})

	b.Publish(sdk.NewEvent("alpha", nil))
	b.Publish(sdk.NewEvent("beta", nil))
	b.Publish(sdk.NewEvent("gamma", nil))

	assert.Eventually(t, func() bool {
		return afterOff.Load() == 1
	}, time.Second, 10*time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(2), count.Load(), "removed handler should not receive events after Off")
}

func TestPublishWithOnlyOnAll(t *testing.T) {
	b := New()
	defer b.Close()

	var received atomic.Value

	b.OnAll(func(e sdk.Event) error {
		received.Store(e)
		return nil
	})

	b.Publish(sdk.NewEvent("no.topic.handlers", "data"))

	assert.Eventually(t, func() bool {
		v, ok := received.Load().(sdk.Event)
		return ok && v.Topic == "no.topic.handlers" && v.Payload == "data"
	}, time.Second, 10*time.Millisecond)
}

func TestConcurrentOnAndPublish(t *testing.T) {
	b := New()
	defer b.Close()

	var count atomic.Int32

	var wg sync.WaitGroup

	// Concurrently register handlers and publish
	for i := range 20 {
		wg.Add(2)

		go func() {
			defer wg.Done()

			b.On("concurrent", func(e sdk.Event) error {
				count.Add(1)
				return nil
			})
		}()

		go func() {
			defer wg.Done()

			b.Publish(sdk.NewEvent("concurrent", i))
		}()
	}

	wg.Wait()

	assert.Eventually(t, func() bool {
		return count.Load() > 0
	}, time.Second, 10*time.Millisecond, "expected some events from concurrent On+Publish")
}

func TestNilHandlerOn(t *testing.T) {
	b := New()

	var panicEvent atomic.Bool

	panicTopic := b.DiagnosticTopics[0]
	b.On(panicTopic, func(e sdk.Event) error {
		panicEvent.Store(true)
		return nil
	})

	b.On("nil.topic", nil)
	b.Publish(sdk.NewEvent("nil.topic", nil))

	assert.Eventually(t, panicEvent.Load, 2*time.Second, 10*time.Millisecond,
		"nil handler should trigger panic diagnostic event")

	require.NoError(t, b.Close())
}

func TestEmptyTopicPublish(t *testing.T) {
	b := New()
	defer b.Close()

	var received atomic.Value

	b.On("", func(e sdk.Event) error {
		received.Store(e)
		return nil
	})

	b.Publish(sdk.NewEvent("", "empty topic"))

	assert.Eventually(t, func() bool {
		v, ok := received.Load().(sdk.Event)
		return ok && v.Topic == "" && v.Payload == "empty topic"
	}, time.Second, 10*time.Millisecond)
}

func TestOnAllReceivesDiagnosticEvents(t *testing.T) {
	b := New()

	var (
		mu     sync.Mutex
		events []string
	)

	b.OnAll(func(e sdk.Event) error {
		mu.Lock()

		events = append(events, e.Topic)
		mu.Unlock()

		return nil
	})

	b.On("crash", func(e sdk.Event) error {
		panic("boom")
	})

	b.Publish(sdk.NewEvent("crash", nil))

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()

		return len(events) >= 2
	}, 2*time.Second, 10*time.Millisecond, "OnAll should receive both original and diagnostic events")

	mu.Lock()
	topics := make([]string, len(events))
	copy(topics, events)
	mu.Unlock()

	panicTopic := b.DiagnosticTopics[0]

	assert.Contains(t, topics, "crash")
	assert.Contains(t, topics, panicTopic)

	require.NoError(t, b.Close())
}

func TestCustomDiagnosticTopics(t *testing.T) {
	b := New()
	b.DiagnosticTopics = []string{"custom.panic", "custom.error"}

	var panicEvent, errEvent atomic.Value

	b.On("custom.panic", func(e sdk.Event) error {
		panicEvent.Store(e)
		return nil
	})

	b.On("custom.error", func(e sdk.Event) error {
		errEvent.Store(e)
		return nil
	})

	b.On("crash.topic", func(e sdk.Event) error {
		panic("custom panic")
	})

	b.On("fail.topic", func(e sdk.Event) error {
		return assert.AnError
	})

	b.Publish(sdk.NewEvent("crash.topic", nil))
	b.Publish(sdk.NewEvent("fail.topic", nil))

	assert.Eventually(t, func() bool {
		v, ok := panicEvent.Load().(sdk.Event)
		return ok && v.Topic == "custom.panic"
	}, 2*time.Second, 10*time.Millisecond, "expected custom.panic diagnostic event")

	assert.Eventually(t, func() bool {
		v, ok := errEvent.Load().(sdk.Event)
		return ok && v.Topic == "custom.error"
	}, 2*time.Second, 10*time.Millisecond, "expected custom.error diagnostic event")

	require.NoError(t, b.Close())
}

func TestCustomDiagnosticTopicNoRecurse(t *testing.T) {
	b := New()
	b.DiagnosticTopics = []string{"diag.panic", "diag.error"}

	var panicCount atomic.Int32

	// Handler on custom panic diagnostic topic that itself panics
	b.On("diag.panic", func(e sdk.Event) error {
		panicCount.Add(1)
		panic("recursive panic")
	})

	// Trigger a panic
	b.On("trigger", func(e sdk.Event) error {
		panic("original")
	})

	b.Publish(sdk.NewEvent("trigger", nil))

	// Should receive exactly one diag.panic — no recursion
	assert.Eventually(t, func() bool {
		return panicCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "expected exactly 1 diag.panic event, no recursion")

	require.NoError(t, b.Close())
}

func TestPartialDiagnosticTopicsNoRecurseOnFallback(t *testing.T) {
	b := New()
	// Only configure the panic topic — error topic falls back to "extension.error"
	b.DiagnosticTopics = []string{"custom.panic"}

	var errCount atomic.Int32

	// Handler on fallback error diagnostic topic that itself returns an error
	b.On("extension.error", func(e sdk.Event) error {
		errCount.Add(1)
		return assert.AnError
	})

	// Trigger an error
	b.On("trigger", func(e sdk.Event) error {
		return assert.AnError
	})

	b.Publish(sdk.NewEvent("trigger", nil))

	// Should receive exactly one extension.error — no recursion
	assert.Eventually(t, func() bool {
		return errCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "expected exactly 1 extension.error event, no recursion")

	require.NoError(t, b.Close())
}

func TestEventFieldsPreserved(t *testing.T) {
	b := New()
	defer b.Close()

	var received atomic.Value

	b.On("fields", func(e sdk.Event) error {
		received.Store(e)
		return nil
	})

	ts := time.Now()
	evt := sdk.Event{
		Topic:     "fields",
		Payload:   map[string]int{"count": 42},
		Timestamp: ts,
		TraceID:   "trace-123",
	}

	b.Publish(evt)

	assert.Eventually(t, func() bool {
		v, ok := received.Load().(sdk.Event)
		if !ok {
			return false
		}

		return v.Topic == "fields" &&
			v.TraceID == "trace-123" &&
			!v.Timestamp.IsZero()
	}, time.Second, 10*time.Millisecond)

	v, ok := received.Load().(sdk.Event)
	require.True(t, ok)
	payload, ok := v.Payload.(map[string]int)
	require.True(t, ok)
	assert.Equal(t, 42, payload["count"])
}
