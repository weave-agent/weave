package sdk

import (
	"fmt"
	"log"
	"os"
	"reflect"

	"weave/internal/auth"
	"weave/sdk/registry"
)

type providerEntry struct {
	factory     func(Config) (Provider, error)
	authChecker func(Config) (bool, error)
}

var providerReg = registry.New[providerEntry](
	registry.WithWarn[providerEntry](log.New(os.Stderr, "weave: ", 0), "provider"),
)

// RegisterProvider registers a provider factory with typed configuration and auth structs.
// The framework will automatically populate the config struct from settings, env vars,
// and CLI flags before calling the factory. Auth is loaded from ~/.weave/auth.json
// and environment variables defined by the auth struct's env tags.
func RegisterProvider[TConfig, TAuth any](name string, factory func(Config, TConfig, TAuth) (Provider, error)) {
	var zeroConfig TConfig

	schema := extractSchema(reflect.TypeOf(zeroConfig))
	storeSchema("providers", name, schema)

	wrapper := func(cfg Config) (Provider, error) {
		var tc TConfig

		if err := cfg.ExtensionConfig("providers", name, &tc, ""); err != nil {
			return nil, fmt.Errorf("load provider config: %w", err)
		}

		var ta TAuth

		return factory(configOrDefault(cfg), tc, ta)
	}

	authChecker := makeAuthChecker[TAuth](name)

	providerReg.Register(name, providerEntry{factory: wrapper, authChecker: authChecker})
}

func makeAuthChecker[TAuth any](name string) func(Config) (bool, error) {
	return func(_ Config) (bool, error) {
		authFile, err := auth.Load()
		if err != nil {
			return false, err
		}

		if authFile.GetProviderKey(name) != "" {
			return true, nil
		}

		var zero TAuth

		t := reflect.TypeOf(zero)
		if t == nil || t.Kind() != reflect.Struct {
			return false, nil
		}

		for field := range t.Fields() {
			field := field

			envTag := field.Tag.Get("env")
			if envTag != "" && os.Getenv(envTag) != "" {
				return true, nil
			}
		}

		return false, nil
	}
}

func ProviderRegistered(name string) bool {
	return providerReg.Exists(name)
}

func GetProvider(name string, cfg Config) (Provider, error) {
	entry, ok := providerReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}

	return entry.factory(configOrDefault(cfg))
}

// CheckProviderAuth returns whether the provider has auth credentials available
// (either in ~/.weave/auth.json or via environment variables).
func CheckProviderAuth(name string, cfg Config) (bool, error) {
	entry, ok := providerReg.Get(name)
	if !ok {
		return false, fmt.Errorf("provider %q not registered", name)
	}

	return entry.authChecker(configOrDefault(cfg))
}

func ListProviders() []string {
	return providerReg.List()
}

func ResetProviderRegistry() {
	providerReg.Reset()
	ResetSchemasForScope("providers")
}
