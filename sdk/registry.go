package sdk

import (
	"fmt"
	"log/slog"
	"reflect"

	"github.com/weave-agent/weave/sdk/registry"
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

	typ := reflect.TypeOf(zero)
	schema := extractSchema(typ)
	storeSchema(scope, name, schema, typ)

	wrapper := func(cfg Config) (Extension, error) {
		var t T

		if err := cfg.ExtensionConfig(scope, name, &t); err != nil {
			return nil, fmt.Errorf("load extension config: %w", err)
		}

		return factory(ConfigReadOnly(cfg), PreferenceStoreFrom(cfg), t)
	}

	extReg.Register(name, wrapper)
}

// RegisterExtensionWithWriter registers an extension factory that receives
// PreferenceWriter instead of PreferenceReader. This is a declarative API
// choice for extensions that need write access; it does not provide security
// isolation because all extensions run in the same process with full system
// access. The framework treats all installed extensions as fully trusted.
func RegisterExtensionWithWriter[T any](name string, factory func(Config, PreferenceWriter, T) (Extension, error)) {
	RegisterExtensionWithScopeAndWriter[T](name, "extensions", factory)
}

// RegisterExtensionWithScopeAndWriter registers an extension factory with a
// custom config scope and write access to preferences. The factory receives
// PreferenceWriter instead of PreferenceReader, allowing it to save preferences
// and provider keys. This path is exported so built-in extensions in other
// packages can use it; there is no runtime allowlist because the framework
// treats all extensions as fully trusted.
func RegisterExtensionWithScopeAndWriter[T any](name, scope string, factory func(Config, PreferenceWriter, T) (Extension, error)) {
	var zero T

	typ := reflect.TypeOf(zero)
	schema := extractSchema(typ)
	storeSchema(scope, name, schema, typ)

	wrapper := func(cfg Config) (Extension, error) {
		var t T

		if err := cfg.ExtensionConfig(scope, name, &t); err != nil {
			return nil, fmt.Errorf("load extension config: %w", err)
		}

		ps := PreferenceStoreFrom(cfg)

		writer, ok := asPreferenceWriter(ps)
		if !ok {
			writer = NoopPreferenceStore{}
		}

		return factory(ConfigReadOnly(cfg), writer, t)
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
