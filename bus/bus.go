package bus

import (
	"fmt"
	"log"
	"reflect"
	"runtime/debug"
	"sync"

	"weave/sdk"
)

var _ sdk.Bus = (*Bus)(nil)

type Bus struct {
	mu      sync.RWMutex
	topicOn map[string][]sdk.Handler
	allOn   []sdk.Handler
	closed  bool
	closeMu sync.RWMutex
	wg      sync.WaitGroup
}

func New() *Bus {
	return &Bus{
		topicOn: make(map[string][]sdk.Handler),
	}
}

func handlerID(h sdk.Handler) uintptr {
	return reflect.ValueOf(h).Pointer()
}

func (b *Bus) On(topic string, h sdk.Handler) {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return
	}

	b.mu.Lock()
	b.topicOn[topic] = append(b.topicOn[topic], h)
	b.mu.Unlock()
}

func (b *Bus) OnAll(h sdk.Handler) {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return
	}

	b.mu.Lock()
	b.allOn = append(b.allOn, h)
	b.mu.Unlock()
}

func (b *Bus) Off(h sdk.Handler) {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	target := handlerID(h)

	for topic, handlers := range b.topicOn {
		remaining := make([]sdk.Handler, 0, len(handlers))
		for _, existing := range handlers {
			if handlerID(existing) != target {
				remaining = append(remaining, existing)
			}
		}
		if len(remaining) == 0 {
			delete(b.topicOn, topic)
		} else {
			b.topicOn[topic] = remaining
		}
	}

	for i, existing := range b.allOn {
		if handlerID(existing) == target {
			b.allOn = append(b.allOn[:i], b.allOn[i+1:]...)
			break
		}
	}
}

func (b *Bus) Publish(e sdk.Event) bool {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return false
	}

	b.mu.RLock()
	handlers := make([]sdk.Handler, 0, len(b.topicOn[e.Topic])+len(b.allOn))
	handlers = append(handlers, b.topicOn[e.Topic]...)
	handlers = append(handlers, b.allOn...)
	b.mu.RUnlock()

	if len(handlers) == 0 {
		return false
	}

	for _, h := range handlers {
		b.dispatch(e, h)
	}

	return true
}

func (b *Bus) dispatch(e sdk.Event, h sdk.Handler) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Printf("[bus] panic in handler: %v\n%s", r, stack)
				b.publishDiagnostic("extension.panic", fmt.Sprintf("panic: %v", r))
			}
		}()

		if err := h(e); err != nil {
			log.Printf("[bus] handler error: %v", err)
			b.publishDiagnostic("extension.error", err.Error())
		}
	}()
}

func (b *Bus) publishDiagnostic(topic string, msg string) {
	ev := sdk.Event{
		Topic:   topic,
		Payload: msg,
	}

	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return
	}

	b.mu.RLock()
	handlers := make([]sdk.Handler, 0, len(b.topicOn[topic])+len(b.allOn))
	handlers = append(handlers, b.topicOn[topic]...)
	handlers = append(handlers, b.allOn...)
	b.mu.RUnlock()

	for _, h := range handlers {
		b.wg.Add(1)
		go func(handler sdk.Handler) {
			defer b.wg.Done()
			_ = handler(ev)
		}(h)
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

	b.wg.Wait()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.topicOn = make(map[string][]sdk.Handler)
	b.allOn = nil

	return nil
}
