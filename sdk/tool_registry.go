package sdk

import (
	"fmt"
	"log"
	"os"

	"weave/sdk/registry"
)

var toolReg = registry.New[func(Config) (Tool, error)](
	registry.WithWarn[func(Config) (Tool, error)](log.New(os.Stderr, "weave: ", 0), "tool"),
)

func RegisterTool(name string, factory func(Config) (Tool, error)) {
	toolReg.Register(name, factory)
}

func GetTool(name string, cfg Config) (Tool, error) {
	factory, ok := toolReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool %q not registered", name)
	}

	return factory(configOrDefault(cfg))
}

func ToolRegistered(name string) bool {
	return toolReg.Exists(name)
}

var toolFilter map[string]bool

func SetToolFilter(names []string) {
	toolFilter = make(map[string]bool, len(names))
	for _, name := range names {
		toolFilter[name] = true
	}
}

func ListTools() []string {
	all := toolReg.List()
	if len(toolFilter) == 0 {
		return all
	}

	filtered := make([]string, 0, len(toolFilter))
	for _, name := range all {
		if toolFilter[name] {
			filtered = append(filtered, name)
		}
	}

	return filtered
}

func ResetToolRegistry() {
	toolReg.Reset()

	toolFilter = nil
}
