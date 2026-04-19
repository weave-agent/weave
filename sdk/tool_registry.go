package sdk

import (
	"fmt"
	"sync"
)

var (
	toolMu  sync.RWMutex
	toolReg = make(map[string]func(Config) (Tool, error))
)

func RegisterTool(name string, factory func(Config) (Tool, error)) {
	toolMu.Lock()
	defer toolMu.Unlock()

	if _, dup := toolReg[name]; dup {
		panic("sdk: RegisterTool called twice for " + name)
	}

	toolReg[name] = factory
}

func GetTool(name string, cfg Config) (Tool, error) {
	toolMu.RLock()

	factory, ok := toolReg[name]

	toolMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool %q not registered", name)
	}

	return factory(configOrDefault(cfg))
}

func ListTools() []string {
	toolMu.RLock()
	defer toolMu.RUnlock()

	names := make([]string, 0, len(toolReg))
	for name := range toolReg {
		names = append(names, name)
	}

	return names
}

func ResetToolRegistry() {
	toolMu.Lock()
	defer toolMu.Unlock()

	toolReg = make(map[string]func(Config) (Tool, error))
}
