package sdk

import (
	"fmt"
	"log"
	"os"
	"sync"

	"weave/sdk/registry"
)

var toolReg = registry.New[func(Config) (Tool, error)](
	registry.WithWarn[func(Config) (Tool, error)](log.New(os.Stderr, "weave: ", 0), "tool"),
)

func RegisterTool(name string, factory func(Config) (Tool, error)) {
	toolReg.Register(name, factory)
}

func GetTool(name string, cfg Config) (Tool, error) {
	toolFilterMu.RLock()

	filter := toolFilter
	active := toolFilter != nil

	toolFilterMu.RUnlock()

	if active && !filter[name] {
		return nil, fmt.Errorf("tool %q not in allowed list", name)
	}

	factory, ok := toolReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool %q not registered", name)
	}

	return factory(configOrDefault(cfg))
}

func ToolRegistered(name string) bool {
	return toolReg.Exists(name)
}

var (
	toolFilter   map[string]bool
	toolFilterMu sync.RWMutex
)

func SetToolFilter(names []string) {
	toolFilterMu.Lock()
	defer toolFilterMu.Unlock()

	if names == nil {
		toolFilter = nil
		return
	}

	toolFilter = make(map[string]bool, len(names))
	for _, name := range names {
		toolFilter[name] = true
	}
}

func ListTools() []string {
	toolFilterMu.RLock()

	filter := toolFilter
	active := toolFilter != nil

	toolFilterMu.RUnlock()

	all := toolReg.List()
	if !active {
		return all
	}

	filtered := make([]string, 0, len(filter))
	for _, name := range all {
		if filter[name] {
			filtered = append(filtered, name)
		}
	}

	return filtered
}

func ResetToolRegistry() {
	toolReg.Reset()

	toolFilterMu.Lock()
	toolFilter = nil
	toolFilterMu.Unlock()
}
