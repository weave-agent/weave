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

type handlerSlot struct {
	ch chan sdk.Event
	h  sdk.Handler
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
		ch: make(chan sdk.Event, 64),
		h:  h,
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
		ch: make(chan sdk.Event, 256),
		h:  h,
	}

	b.mu.Lock()
	b.allOn = append(b.allOn, slot)
	b.mu.Unlock()

	b.wg.Add(1)
	go b.runSlot(slot)
}

func (b *Bus) runSlot(slot *handlerSlot) {
	defer b.wg.Done()
	for ev := range slot.ch {
		b.invokeHandler(ev, slot.h)
	}
}

func (b *Bus) Off(h sdk.Handler) {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

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
				close(slot.ch)
			}
		}
		if len(remaining) == 0 {
			delete(b.topicOn, topic)
		} else {
			b.topicOn[topic] = remaining
		}
	}

	for i, slot := range b.allOn {
		if handlerID(slot.h) == target {
			close(slot.ch)
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
			b.publishDiagnostic("extension.panic", fmt.Sprintf("panic: %v", r))
		}
	}()

	if err := h(e); err != nil {
		log.Printf("[bus] handler error: %v", err)
		b.publishDiagnostic("extension.error", err.Error())
	}
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
