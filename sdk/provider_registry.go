package sdk

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"weave/sdk/registry"
)

var providerReg = registry.New[func(Config) (Provider, error)](
	registry.WithWarn[func(Config) (Provider, error)](log.New(os.Stderr, "weave: ", 0), "provider"),
)

// RegisterProvider registers a provider factory with a typed configuration struct.
// The framework will automatically populate the config struct from settings, env vars,
// and CLI flags before calling the factory.
func RegisterProvider[T any](name string, factory func(Config, T) (Provider, error)) {
	var zero T

	schema := extractSchema(reflect.TypeOf(zero))
	storeSchema("providers", name, schema)

	wrapper := func(cfg Config) (Provider, error) {
		var t T

		envPrefix := "WEAVE_" + strings.ToUpper(name)
		if err := cfg.ExtensionConfig("providers", name, &t, envPrefix); err != nil {
			return nil, fmt.Errorf("load provider config: %w", err)
		}

		return factory(configOrDefault(cfg), t)
	}

	providerReg.Register(name, wrapper)
}

func ProviderRegistered(name string) bool {
	return providerReg.Exists(name)
}

func GetProvider(name string, cfg Config) (Provider, error) {
	factory, ok := providerReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}

	return factory(configOrDefault(cfg))
}

func ListProviders() []string {
	return providerReg.List()
}

func ResetProviderRegistry() {
	providerReg.Reset()
}
