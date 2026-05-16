package sdk

import (
	"fmt"
	"log/slog"
	"reflect"

	"weave/sdk/registry"
)

var extReg = registry.New[func(Config) (Extension, error)](
	registry.WithWarn[func(Config) (Extension, error)](func(name string) {
		slog.Warn("duplicate registration", "name", name, "kind", "extension")
	}),
)

// RegisterExtension registers an extension factory with a typed configuration struct.
// The framework will automatically populate the config struct from settings, env vars,
// and CLI flags before calling the factory.
// Config is loaded from the "extensions" scope.
func RegisterExtension[T any](name string, factory func(Config, PreferenceReader, T) (Extension, error)) {
	RegisterExtensionWithScope[T](name, "extensions", factory)
}

// RegisterExtensionWithScope registers an extension factory with a typed configuration
// struct and a custom config scope. The scope determines which settings subtree is
// used to populate the config struct (e.g. "ui" for TUI, "sandbox" for sandbox).
func RegisterExtensionWithScope[T any](name, scope string, factory func(Config, PreferenceReader, T) (Extension, error)) {
	var zero T

	schema := extractSchema(reflect.TypeOf(zero))
	storeSchema(scope, name, schema)

	wrapper := func(cfg Config) (Extension, error) {
		var t T

		if err := cfg.ExtensionConfig(scope, name, &t); err != nil {
			return nil, fmt.Errorf("load extension config: %w", err)
		}

		return factory(ConfigOrDefault(cfg), PreferenceStoreFrom(cfg), t)
	}

	extReg.Register(name, wrapper)
}

// ExtensionRegistered reports whether an extension with the given name has been
// registered without instantiating it.
func ExtensionRegistered(name string) bool {
	return extReg.Exists(name)
}

func GetExtension(name string, cfg Config) (Extension, error) {
	factory, ok := extReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("extension %q: %w", name, ErrNotRegistered)
	}

	return factory(ConfigOrDefault(cfg))
}

func ListExtensions() []string {
	return extReg.List()
}

func ResetExtensionRegistry() {
	extReg.Reset()
	ResetSchemas()
}
