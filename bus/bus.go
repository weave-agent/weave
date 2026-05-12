package bus

import (
	"fmt"
	"log"
	"reflect"
	"runtime/debug"
	"slices"
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
	mu               sync.RWMutex
	topicOn          map[string][]*handlerSlot
	allOn            []*handlerSlot
	closed           bool
	closeMu          sync.RWMutex
	wg               sync.WaitGroup
	DiagnosticTopics []string
}

// New creates a new event bus ready for use.
func New() *Bus {
	return &Bus{
		topicOn:          make(map[string][]*handlerSlot),
		DiagnosticTopics: []string{"extension.panic", "extension.error"},
	}
}

func handlerID(h sdk.Handler) uintptr {
	if h == nil {
		return 0
	}

	return reflect.ValueOf(h).Pointer()
}

// On registers a handler for a specific topic. Each handler runs in its own
// goroutine with a 64-event buffer. Delivery is non-blocking; if the buffer is
// full the event is dropped and a warning is logged. Handler panics are
// recovered and re-published on the configured panic diagnostic topic. No-op if
// called after Close.
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

// OnAll registers a handler that receives every event published on the bus,
// regardless of topic. Uses a 256-event buffer (larger than On because OnAll
// handlers see all traffic). Otherwise identical semantics to On.
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

// Off removes all registrations matching h by function pointer identity
// (reflect.ValueOf(h).Pointer()). Callers must retain the exact Handler
// variable passed to On/OnAll — anonymous closures created inline cannot be
// removed. No-op if called after Close or if h is not registered.
func (b *Bus) Off(h sdk.Handler) {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()

	if b.closed {
		return
	}

	target := handlerID(h)

	var removed []*handlerSlot

	b.mu.Lock()
	for topic, slots := range b.topicOn {
		remaining := make([]*handlerSlot, 0, len(slots))
		for _, slot := range slots {
			if handlerID(slot.h) != target {
				remaining = append(remaining, slot)
			} else {
				removed = append(removed, slot)
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
			removed = append(removed, slot)
		}
	}

	b.allOn = remainingAll
	b.mu.Unlock()

	for _, slot := range removed {
		close(slot.done)
	}
}

func (b *Bus) collectSubscribers(topic string) []*handlerSlot {
	b.mu.RLock()
	slots := make([]*handlerSlot, 0, len(b.topicOn[topic])+len(b.allOn))
	slots = append(slots, b.topicOn[topic]...)
	slots = append(slots, b.allOn...)
	b.mu.RUnlock()

	return slots
}

// Publish sends an event to all subscribers of the event's topic plus any OnAll
// handlers. Delivery is non-blocking: if a handler's internal buffer is full the
// event is dropped and a warning is logged. Publish returns immediately
// regardless of handler speed.
func (b *Bus) Publish(e sdk.Event) {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()

	if b.closed {
		return
	}

	slots := b.collectSubscribers(e.Topic)

	for _, slot := range slots {
		select {
		case slot.ch <- e:
		default:
			log.Printf("[bus] dropped event on topic %q: handler channel full", e.Topic)
		}
	}
}

func (b *Bus) panicTopic() string {
	if len(b.DiagnosticTopics) > 0 {
		return b.DiagnosticTopics[0]
	}

	return "extension.panic"
}

func (b *Bus) errorTopic() string {
	if len(b.DiagnosticTopics) > 1 {
		return b.DiagnosticTopics[1]
	}

	return "extension.error"
}

func (b *Bus) isDiagnosticTopic(topic string) bool {
	if slices.Contains(b.DiagnosticTopics, topic) {
		return true
	}

	// Fallback topics are also diagnostic to prevent recursion when only
	// a subset of DiagnosticTopics is configured.
	return topic == "extension.panic" || topic == "extension.error"
}

// invokeHandler calls h(e) with panic/error recovery. Panics are logged and
// re-published on the configured panic diagnostic topic; errors are logged and
// re-published on the configured error diagnostic topic. Events whose topic is
// in DiagnosticTopics are suppressed to prevent infinite recursion.
func (b *Bus) invokeHandler(e sdk.Event, h sdk.Handler) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Printf("[bus] panic in handler on topic %q: %v\n%s", e.Topic, r, stack)

			if !b.isDiagnosticTopic(e.Topic) {
				b.publishDiagnostic(b.panicTopic(), fmt.Sprintf("panic: %v", r))
			}
		}
	}()

	if err := h(e); err != nil {
		log.Printf("[bus] handler error on topic %q: %v", e.Topic, err)

		if !b.isDiagnosticTopic(e.Topic) {
			b.publishDiagnostic(b.errorTopic(), err.Error())
		}
	}
}

// publishDiagnostic emits a diagnostic event on the given topic. It is only
// called from invokeHandler which already runs inside a recover-wrapped
// context, so the closeMu check is sufficient — no additional recover is
// needed here.
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

	slots := b.collectSubscribers(topic)

	for _, slot := range slots {
		select {
		case slot.ch <- ev:
		default:
			log.Printf("[bus] dropped diagnostic event on topic %q: handler channel full", topic)
		}
	}
}

// Close shuts down the bus. It closes all handler channels and blocks until
// every in-flight handler invocation has completed. After Close, all calls to
// Publish, On, and OnAll are no-ops. Close is idempotent.
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
