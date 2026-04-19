package sdk

import (
	"fmt"
	"sync"
)

var (
	providerMu  sync.RWMutex
	providerReg = make(map[string]func(Config) (Provider, error))
)

func RegisterProvider(name string, factory func(Config) (Provider, error)) {
	providerMu.Lock()
	defer providerMu.Unlock()

	if _, dup := providerReg[name]; dup {
		panic("sdk: RegisterProvider called twice for " + name)
	}

	providerReg[name] = factory
}

func GetProvider(name string, cfg Config) (Provider, error) {
	providerMu.RLock()

	factory, ok := providerReg[name]

	providerMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}

	return factory(configOrDefault(cfg))
}

func ListProviders() []string {
	providerMu.RLock()
	defer providerMu.RUnlock()

	names := make([]string, 0, len(providerReg))
	for name := range providerReg {
		names = append(names, name)
	}

	return names
}

func ResetProviderRegistry() {
	providerMu.Lock()
	defer providerMu.Unlock()

	providerReg = make(map[string]func(Config) (Provider, error))
}
