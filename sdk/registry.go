package sdk

import (
	"fmt"
	"log"
	"os"

	"weave/sdk/registry"
)

var extReg = registry.New[func(Config) (Extension, error)](
	registry.WithWarn[func(Config) (Extension, error)](log.New(os.Stderr, "weave: ", 0), "extension"),
)

func RegisterExtension(name string, factory func(Config) (Extension, error)) {
	extReg.Register(name, factory)
}

func GetExtension(name string, cfg Config) (Extension, error) {
	factory, ok := extReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("extension %q not registered", name)
	}

	return factory(configOrDefault(cfg))
}

func ListExtensions() []string {
	return extReg.List()
}

func ResetRegistry() {
	extReg.Reset()
}
