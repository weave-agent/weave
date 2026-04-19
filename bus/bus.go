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
	defer b.closeMu.RUnlock()

	if b.closed {
		ch := make(chan sdk.Event)
		close(ch)

		return ch
	}

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
	defer b.closeMu.RUnlock()

	if b.closed {
		ch := make(chan sdk.Event)
		close(ch)

		return ch
	}

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
	defer b.mu.RUnlock()

	delivered := false

	for _, ch := range b.topicSubs[e.Topic] {
		select {
		case ch <- e:
			delivered = true
		default:
		}
	}

	for _, ch := range b.allSubs {
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

	closed := false

	for topic, subs := range b.topicSubs {
		remaining := make([]chan sdk.Event, 0, len(subs))
		for _, s := range subs {
			if s == ch {
				if !closed {
					close(s)

					closed = true
				}
			} else {
				remaining = append(remaining, s)
			}
		}

		if len(remaining) == 0 {
			delete(b.topicSubs, topic)
		} else {
			b.topicSubs[topic] = remaining
		}
	}

	for i, s := range b.allSubs {
		if s == ch {
			if !closed {
				close(s)
			}

			b.allSubs = append(b.allSubs[:i], b.allSubs[i+1:]...)

			break
		}
	}
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
