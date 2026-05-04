package sdk

import (
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
)

var toolWarnLog = log.New(os.Stderr, "weave: ", 0)

var (
	toolMu  sync.RWMutex
	toolReg = make(map[string]func(Config) (Tool, error))
)

func RegisterTool(name string, factory func(Config) (Tool, error)) {
	toolMu.Lock()
	defer toolMu.Unlock()

	if _, dup := toolReg[name]; dup {
		toolWarnLog.Printf("warning: tool %q already registered; first registration wins", name)
		return
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

func ToolRegistered(name string) bool {
	toolMu.RLock()

	ok := toolReg[name] != nil

	toolMu.RUnlock()

	return ok
}

func ListTools() []string {
	toolMu.RLock()
	defer toolMu.RUnlock()

	names := make([]string, 0, len(toolReg))
	for name := range toolReg {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func ResetToolRegistry() {
	toolMu.Lock()
	defer toolMu.Unlock()

	toolReg = make(map[string]func(Config) (Tool, error))
}
