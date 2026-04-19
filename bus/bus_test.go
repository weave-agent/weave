package bus

import (
	"sync"
	"testing"
	"time"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPubSub(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("test.topic")
	e := sdk.NewEvent("test.topic", "hello")
	b.Publish(e)

	select {
	case got := <-ch:
		assert.Equal(t, "test.topic", got.Topic)
		assert.Equal(t, "hello", got.Payload)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	b := New()
	defer b.Close()

	ch1 := b.Subscribe("topic")
	ch2 := b.Subscribe("topic")

	b.Publish(sdk.NewEvent("topic", nil))

	for i, ch := range []<-chan sdk.Event{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d did not receive event", i+1)
		}
	}
}

func TestSubscribeMultipleTopics(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("a", "b")
	b.Publish(sdk.NewEvent("a", nil))
	b.Publish(sdk.NewEvent("b", nil))

	for i := range 2 {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("event %d not received", i+1)
		}
	}
}

func TestSubscribeAll(t *testing.T) {
	b := New()
	defer b.Close()

	all := b.SubscribeAll()
	b.Publish(sdk.NewEvent("any.topic", "data"))

	select {
	case got := <-all:
		assert.Equal(t, "any.topic", got.Topic)
	case <-time.After(time.Second):
		t.Fatal("SubscribeAll did not receive event")
	}
}

func TestUnrelatedTopicNotReceived(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("alpha")
	b.Publish(sdk.NewEvent("beta", nil))

	select {
	case <-ch:
		t.Fatal("received event for wrong topic")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBufferOverflow(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("flood")
	for i := range topicBufSize + 20 {
		b.Publish(sdk.NewEvent("flood", i))
	}

	count := 0

	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}

done:
	assert.LessOrEqual(t, count, topicBufSize, "received more events than buffer size")

	assert.NotZero(t, count, "expected some events to be buffered")
}

func TestClose(t *testing.T) {
	b := New()
	ch := b.Subscribe("x")
	_ = b.Close()

	_, ok := <-ch
	assert.False(t, ok, "expected channel to be closed")
}

func TestPublishAfterClose(t *testing.T) {
	b := New()
	_ = b.Close()

	ch := b.Subscribe("x")
	b.Publish(sdk.NewEvent("x", nil))

	_, ok := <-ch
	assert.False(t, ok, "subscribe after close should return closed channel")
}

func TestCloseAllSubscriber(t *testing.T) {
	b := New()
	all := b.SubscribeAll()
	_ = b.Close()

	_, ok := <-all
	assert.False(t, ok, "expected SubscribeAll channel to be closed")
}

func TestConcurrentPublish(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("concurrent")

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			b.Publish(sdk.NewEvent("concurrent", i))
		})
	}

	wg.Wait()

	drained := 0

	for {
		select {
		case <-ch:
			drained++
		default:
			require.NotZero(t, drained, "expected at least some events from concurrent publishes")
			return
		}
	}
}

func TestConcurrentPublishAndClose(t *testing.T) {
	for range 50 {
		b := New()
		_ = b.Subscribe("race")

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

func TestSubscribeAfterClose(t *testing.T) {
	b := New()
	_ = b.Close()

	ch := b.Subscribe("x")
	_, ok := <-ch
	assert.False(t, ok, "Subscribe after Close should return closed channel")

	allCh := b.SubscribeAll()
	_, ok = <-allCh
	assert.False(t, ok, "SubscribeAll after Close should return closed channel")
}

func TestUnsubscribe(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("topic")
	b.Unsubscribe(ch)

	_, ok := <-ch
	assert.False(t, ok, "unsubscribed channel should be closed")

	b.Publish(sdk.NewEvent("topic", "data"))
}

func TestUnsubscribeAllSubscriber(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.SubscribeAll()
	b.Unsubscribe(ch)

	_, ok := <-ch
	assert.False(t, ok, "unsubscribed channel should be closed")
}

func TestUnsubscribeDoubleSafe(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("x")
	b.Unsubscribe(ch)
	b.Unsubscribe(ch)
}

func TestUnsubscribeUnknownChannel(t *testing.T) {
	b := New()
	defer b.Close()

	ch := make(chan sdk.Event)
	b.Unsubscribe(ch)
}

func TestUnsubscribeMultiTopic(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("a", "b")
	b.Unsubscribe(ch)

	_, ok := <-ch
	assert.False(t, ok, "unsubscribed channel should be closed")

	b.Publish(sdk.NewEvent("a", nil))
	b.Publish(sdk.NewEvent("b", nil))
}

func TestPublishReturnsFalseOnDrop(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("full")
	for i := range topicBufSize {
		require.True(t, b.Publish(sdk.NewEvent("full", i)), "Publish %d returned false before buffer full", i)
	}

	assert.False(t, b.Publish(sdk.NewEvent("full", "overflow")), "expected Publish to return false when subscriber buffer is full")

	<-ch

	assert.True(t, b.Publish(sdk.NewEvent("full", "after-drain")), "expected Publish to return true after drain")
}

func TestPublishReturnsFalseWhenClosed(t *testing.T) {
	b := New()
	_ = b.Close()

	assert.False(t, b.Publish(sdk.NewEvent("x", nil)), "expected Publish to return false on closed bus")
}

func TestPublishReturnsFalseWithNoSubscribers(t *testing.T) {
	b := New()
	defer b.Close()

	assert.False(t, b.Publish(sdk.NewEvent("nobody", nil)), "expected Publish to return false with no subscribers")
}
