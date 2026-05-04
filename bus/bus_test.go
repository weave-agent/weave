package bus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"weave/sdk"

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

	b.On("extension.panic", func(e sdk.Event) error {
		panicEvent.Store(e)
		return nil
	})

	b.On("crash.topic", func(e sdk.Event) error {
		panic("something broke")
	})

	b.Publish(sdk.NewEvent("crash.topic", nil))

	assert.Eventually(t, func() bool {
		v, ok := panicEvent.Load().(sdk.Event)
		return ok && v.Topic == "extension.panic"
	}, 2*time.Second, 10*time.Millisecond, "expected extension.panic diagnostic event")

	// Bus should still be usable after panic
	require.NoError(t, b.Close())
}

func TestErrorHandler(t *testing.T) {
	b := New()

	var errEvent atomic.Value

	b.On("extension.error", func(e sdk.Event) error {
		errEvent.Store(e)
		return nil
	})

	b.On("fail.topic", func(e sdk.Event) error {
		return assert.AnError
	})

	b.Publish(sdk.NewEvent("fail.topic", nil))

	assert.Eventually(t, func() bool {
		v, ok := errEvent.Load().(sdk.Event)
		return ok && v.Topic == "extension.error"
	}, 2*time.Second, 10*time.Millisecond, "expected extension.error diagnostic event")

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

	// After close, new publishes should return false
	assert.False(t, b.Publish(sdk.NewEvent("x", nil)))
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

	assert.False(t, b.Publish(sdk.NewEvent("x", nil)), "Publish after Close should return false")
}

func TestPublishReturnsFalseWithNoHandlers(t *testing.T) {
	b := New()
	defer b.Close()

	assert.False(t, b.Publish(sdk.NewEvent("nobody", nil)), "expected false with no handlers")
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Publish(sdk.NewEvent("concurrent", i))
		}()
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
