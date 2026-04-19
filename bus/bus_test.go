package bus

import (
	"sync"
	"testing"
	"time"

	"weave/sdk"
)

func TestPubSub(t *testing.T) {
	b := New()
	defer b.Close()

	ch := b.Subscribe("test.topic")
	e := sdk.NewEvent("test.topic", "hello")
	b.Publish(e)

	select {
	case got := <-ch:
		if got.Topic != "test.topic" {
			t.Errorf("expected topic %q, got %q", "test.topic", got.Topic)
		}

		if got.Payload != "hello" {
			t.Errorf("expected payload %q, got %q", "hello", got.Payload)
		}
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
		if got.Topic != "any.topic" {
			t.Errorf("expected topic %q, got %q", "any.topic", got.Topic)
		}
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
	if count > topicBufSize {
		t.Errorf("received %d events, expected at most %d (buffer overflow)", count, topicBufSize)
	}

	if count == 0 {
		t.Error("expected some events to be buffered")
	}
}

func TestClose(t *testing.T) {
	b := New()
	ch := b.Subscribe("x")
	b.Close()

	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestPublishAfterClose(t *testing.T) {
	b := New()
	b.Close()

	ch := b.Subscribe("x")
	b.Publish(sdk.NewEvent("x", nil))

	_, ok := <-ch
	if ok {
		t.Fatal("subscribe after close should return closed channel")
	}
}

func TestCloseAllSubscriber(t *testing.T) {
	b := New()
	all := b.SubscribeAll()
	b.Close()

	_, ok := <-all
	if ok {
		t.Error("expected SubscribeAll channel to be closed")
	}
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

	// Drain what we can — non-blocking sends mean some may be dropped at buffer limit.
	drained := 0

	for {
		select {
		case <-ch:
			drained++
		default:
			if drained == 0 {
				t.Fatal("expected at least some events from concurrent publishes")
			}

			return
		}
	}
}

func TestConcurrentPublishAndClose(t *testing.T) {
	for range 50 {
		b := New()
		_ = b.Subscribe("race")

		var wg sync.WaitGroup

		wg.Go(func() {
			for range 200 {
				b.Publish(sdk.NewEvent("race", nil))
			}
		})

		wg.Go(func() {
			b.Close()
		})

		wg.Wait()
	}
}
