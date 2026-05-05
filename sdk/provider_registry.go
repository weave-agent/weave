package sdk

import (
	"fmt"
	"log"
	"os"

	"weave/sdk/registry"
)

var providerReg = registry.New[func(Config) (Provider, error)](
	registry.WithWarn[func(Config) (Provider, error)](log.New(os.Stderr, "weave: ", 0), "provider"),
)

func RegisterProvider(name string, factory func(Config) (Provider, error)) {
	providerReg.Register(name, factory)
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
