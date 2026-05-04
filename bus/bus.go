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

// Buffer sizes for per-handler dispatch channels. OnAll handlers see every
// event published on the bus, so they need a larger buffer than topic-specific
// handlers.
const (
	topicHandlerBufSize = 64
	allHandlerBufSize   = 256
)

type handlerSlot struct {
	ch   chan sdk.Event
	h    sdk.Handler
	done chan struct{}
}

type Bus struct {
	mu      sync.RWMutex
	topicOn map[string][]*handlerSlot
	allOn   []*handlerSlot
	closed  bool
	closeMu sync.RWMutex
	wg      sync.WaitGroup
}

func New() *Bus {
	return &Bus{
		topicOn: make(map[string][]*handlerSlot),
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

	slot := &handlerSlot{
		ch:   make(chan sdk.Event, topicHandlerBufSize),
		h:    h,
		done: make(chan struct{}),
	}

	b.mu.Lock()
	b.topicOn[topic] = append(b.topicOn[topic], slot)
	b.mu.Unlock()

	b.wg.Add(1)
	go b.runSlot(slot)
}

func (b *Bus) OnAll(h sdk.Handler) {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return
	}

	slot := &handlerSlot{
		ch:   make(chan sdk.Event, allHandlerBufSize),
		h:    h,
		done: make(chan struct{}),
	}

	b.mu.Lock()
	b.allOn = append(b.allOn, slot)
	b.mu.Unlock()

	b.wg.Add(1)
	go b.runSlot(slot)
}

func (b *Bus) runSlot(slot *handlerSlot) {
	defer b.wg.Done()

	for {
		select {
		case ev, ok := <-slot.ch:
			if !ok {
				return
			}

			b.invokeHandler(ev, slot.h)
		case <-slot.done:
			return
		}
	}
}

func (b *Bus) Off(h sdk.Handler) {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()

	if b.closed {
		return
	}

	target := handlerID(h)

	b.mu.Lock()
	defer b.mu.Unlock()

	for topic, slots := range b.topicOn {
		remaining := make([]*handlerSlot, 0, len(slots))
		for _, slot := range slots {
			if handlerID(slot.h) != target {
				remaining = append(remaining, slot)
			} else {
				close(slot.done)
			}
		}

		if len(remaining) == 0 {
			delete(b.topicOn, topic)
		} else {
			b.topicOn[topic] = remaining
		}
	}

	remainingAll := make([]*handlerSlot, 0, len(b.allOn))
	for _, slot := range b.allOn {
		if handlerID(slot.h) != target {
			remainingAll = append(remainingAll, slot)
		} else {
			close(slot.done)
		}
	}

	b.allOn = remainingAll
}

func (b *Bus) Publish(e sdk.Event) bool {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return false
	}

	b.mu.RLock()
	slots := make([]*handlerSlot, 0, len(b.topicOn[e.Topic])+len(b.allOn))
	slots = append(slots, b.topicOn[e.Topic]...)
	slots = append(slots, b.allOn...)
	b.mu.RUnlock()

	if len(slots) == 0 {
		return false
	}

	for _, slot := range slots {
		select {
		case slot.ch <- e:
		default:
		}
	}

	return true
}

func (b *Bus) invokeHandler(e sdk.Event, h sdk.Handler) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Printf("[bus] panic in handler: %v\n%s", r, stack)

			if e.Topic != "extension.panic" && e.Topic != "extension.error" {
				b.publishDiagnostic("extension.panic", fmt.Sprintf("panic: %v", r))
			}
		}
	}()

	if err := h(e); err != nil {
		log.Printf("[bus] handler error: %v", err)

		if e.Topic != "extension.panic" && e.Topic != "extension.error" {
			b.publishDiagnostic("extension.error", err.Error())
		}
	}
}

func (b *Bus) publishDiagnostic(topic, msg string) {
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
	slots := make([]*handlerSlot, 0, len(b.topicOn[topic])+len(b.allOn))
	slots = append(slots, b.topicOn[topic]...)
	slots = append(slots, b.allOn...)
	b.mu.RUnlock()

	for _, slot := range slots {
		select {
		case slot.ch <- ev:
		default:
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
	for _, slots := range b.topicOn {
		for _, slot := range slots {
			close(slot.ch)
		}
	}

	for _, slot := range b.allOn {
		close(slot.ch)
	}
	b.mu.Unlock()

	b.wg.Wait()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.topicOn = make(map[string][]*handlerSlot)
	b.allOn = nil

	return nil
}
