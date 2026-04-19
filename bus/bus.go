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

func (b *Bus) Publish(e sdk.Event) {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return
	}

	b.mu.RLock()
	subs := b.topicSubs[e.Topic]
	allSubs := b.allSubs
	b.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}

	for _, ch := range allSubs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (b *Bus) Close() {
	b.closeMu.Lock()
	if b.closed {
		b.closeMu.Unlock()
		return
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
}
