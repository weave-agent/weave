package sdk

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"weave/sdk/registry"
)

var extReg = registry.New[func(Config) (Extension, error)](
	registry.WithWarn[func(Config) (Extension, error)](log.New(os.Stderr, "weave: ", 0), "extension"),
)

// RegisterExtension registers an extension factory with a typed configuration struct.
// The framework will automatically populate the config struct from settings, env vars,
// and CLI flags before calling the factory.
func RegisterExtension[T any](name string, factory func(Config, T) (Extension, error)) {
	var zero T

	schema := extractSchema(reflect.TypeOf(zero))
	storeSchema("extensions", name, schema)

	wrapper := func(cfg Config) (Extension, error) {
		var t T

		envPrefix := "WEAVE_" + strings.ToUpper(name)
		if err := cfg.ExtensionConfig("extensions", name, &t, envPrefix); err != nil {
			return nil, fmt.Errorf("load extension config: %w", err)
		}

		return factory(configOrDefault(cfg), t)
	}

	extReg.Register(name, wrapper)
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
