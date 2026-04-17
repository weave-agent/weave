package sdk

import (
	"fmt"
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]func() Extension)
)

func RegisterExtension(name string, factory func() Extension) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

func GetExtension(name string) (Extension, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("extension %q not registered", name)
	}
	return factory(), nil
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

func resetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = make(map[string]func() Extension)
}
