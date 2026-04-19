package bus

import (
	"sync"

	"weave/sdk"
)

const (
	topicBufSize = 64
	allBufSize   = 256
)

var _ sdk.Bus = (*Bus)(nil)

type Bus struct {
	mu        sync.RWMutex
	topicSubs map[string][]chan sdk.Event
	allSubs   []chan sdk.Event
	closed    bool
	closeMu   sync.RWMutex
}

func New() *Bus {
	return &Bus{
		topicSubs: make(map[string][]chan sdk.Event),
	}
}

func (b *Bus) Subscribe(topics ...string) <-chan sdk.Event {
	b.closeMu.RLock()

	if b.closed {
		b.closeMu.RUnlock()

		ch := make(chan sdk.Event)
		close(ch)

		return ch
	}

	b.closeMu.RUnlock()

	ch := make(chan sdk.Event, topicBufSize)

	b.mu.Lock()
	for _, t := range topics {
		b.topicSubs[t] = append(b.topicSubs[t], ch)
	}
	b.mu.Unlock()

	return ch
}

func (b *Bus) SubscribeAll() <-chan sdk.Event {
	b.closeMu.RLock()

	if b.closed {
		b.closeMu.RUnlock()

		ch := make(chan sdk.Event)
		close(ch)

		return ch
	}

	b.closeMu.RUnlock()

	ch := make(chan sdk.Event, allBufSize)

	b.mu.Lock()
	b.allSubs = append(b.allSubs, ch)
	b.mu.Unlock()

	return ch
}

func (b *Bus) Publish(e sdk.Event) bool {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return false
	}

	b.mu.RLock()
	subs := b.topicSubs[e.Topic]
	allSubs := b.allSubs
	b.mu.RUnlock()

	delivered := false

	for _, ch := range subs {
		select {
		case ch <- e:
			delivered = true
		default:
		}
	}

	for _, ch := range allSubs {
		select {
		case ch <- e:
			delivered = true
		default:
		}
	}

	return delivered
}

func (b *Bus) Unsubscribe(ch <-chan sdk.Event) {
	b.closeMu.RLock()

	if b.closed {
		b.closeMu.RUnlock()
		return
	}

	b.closeMu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	for topic, subs := range b.topicSubs {
		b.topicSubs[topic] = removeSub(subs, ch)
		if len(b.topicSubs[topic]) == 0 {
			delete(b.topicSubs, topic)
		}
	}

	b.allSubs = removeSub(b.allSubs, ch)
}

func removeSub(subs []chan sdk.Event, target <-chan sdk.Event) []chan sdk.Event {
	for i, s := range subs {
		if s == target {
			close(s)
			return append(subs[:i], subs[i+1:]...)
		}
	}

	return subs
}

func (b *Bus) Close() error {
	b.closeMu.Lock()
	if b.closed {
		b.closeMu.Unlock()
		return nil
	}

	b.closed = true

	b.closeMu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	seen := make(map[chan sdk.Event]struct{})

	for _, subs := range b.topicSubs {
		for _, ch := range subs {
			if _, ok := seen[ch]; !ok {
				close(ch)
				seen[ch] = struct{}{}
			}
		}
	}

	for _, ch := range b.allSubs {
		if _, ok := seen[ch]; !ok {
			close(ch)
			seen[ch] = struct{}{}
		}
	}

	b.topicSubs = make(map[string][]chan sdk.Event)
	b.allSubs = nil

	return nil
}
