package registry

import (
	"log"
	"sort"
	"sync"
)

// Option configures a Registry's behavior.
type Option[T any] func(*Registry[T])

// WithWarn configures first-wins behavior with a warning logged on duplicate registration.
func WithWarn[T any](logger *log.Logger, label string) Option[T] {
	return func(r *Registry[T]) {
		r.onDup = func(name string) {
			logger.Printf("warning: %s %q already registered; first registration wins", label, name)
		}
	}
}

// Registry is a concurrency-safe, generic named-item registry.
type Registry[T any] struct {
	mu    sync.RWMutex
	items map[string]T
	onDup func(name string)
}

// New creates a Registry with the given options.
func New[T any](opts ...Option[T]) *Registry[T] {
	r := &Registry[T]{
		items: make(map[string]T),
	}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Register adds an item. If name is empty, Register panics.
// On duplicate, behavior depends on configured options (warn, panic, or silent first-wins).
func (r *Registry[T]) Register(name string, item T) {
	if name == "" {
		panic("registry: Register called with empty name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, dup := r.items[name]; dup {
		if r.onDup != nil {
			r.onDup(name)
		}

		return
	}

	r.items[name] = item
}

// Get returns the item by name. The second return value reports whether it was found.
func (r *Registry[T]) Get(name string) (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[name]

	return item, ok
}

// Exists reports whether an item with the given name is registered.
func (r *Registry[T]) Exists(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.items[name]

	return ok
}

// List returns all registered names, sorted alphabetically.
func (r *Registry[T]) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.items))
	for name := range r.items {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// All returns all registered items in sorted name order.
func (r *Registry[T]) All() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.items))
	for name := range r.items {
		names = append(names, name)
	}

	sort.Strings(names)

	items := make([]T, 0, len(names))
	for _, name := range names {
		items = append(items, r.items[name])
	}

	return items
}

// Reset clears all registered items.
func (r *Registry[T]) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	clear(r.items)
}
