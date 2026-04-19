package sdk

import (
	"fmt"
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]func(Config) (Extension, error))
)

func RegisterExtension(name string, factory func(Config) (Extension, error)) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, dup := registry[name]; dup {
		panic("sdk: RegisterExtension called twice for " + name)
	}

	registry[name] = factory
}

func GetExtension(name string, cfg Config) (Extension, error) {
	registryMu.RLock()

	factory, ok := registry[name]

	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("extension %q not registered", name)
	}

	return factory(cfg)
}

func ListExtensions() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}

	return names
}

func ResetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()

	registry = make(map[string]func(Config) (Extension, error))
}
