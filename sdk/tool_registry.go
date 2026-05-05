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

func ListTools() []string {
	return toolReg.List()
}

func ResetToolRegistry() {
	toolReg.Reset()
}
