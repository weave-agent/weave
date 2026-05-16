package sdk

import (
	"fmt"
	"log/slog"
	"reflect"

	"weave/sdk/registry"
)

type uiExtEntry struct {
	factory func(Config) (UIExtension, error)
}

var uiExtReg = registry.New[uiExtEntry](
	registry.WithWarn[uiExtEntry](func(name string) {
		slog.Warn("duplicate registration", "name", name, "kind", "ui extension")
	}),
)

// RegisterUIExtension registers a UI extension factory with a typed configuration struct.
// The framework will automatically populate the config struct from settings, env vars,
// and CLI flags before calling the factory.
func RegisterUIExtension[TConfig any](name string, factory func(Config, PreferenceReader, TConfig) (UIExtension, error)) {
	var zero TConfig

	schema := extractSchema(reflect.TypeOf(zero))
	storeSchema("ui_extensions", name, schema)

	wrapper := func(cfg Config) (UIExtension, error) {
		var t TConfig

		if err := cfg.ExtensionConfig("ui_extensions", name, &t); err != nil {
			return nil, fmt.Errorf("load ui extension config: %w", err)
		}

		return factory(ConfigOrDefault(cfg), PreferenceStoreFrom(cfg), t)
	}

	uiExtReg.Register(name, uiExtEntry{factory: wrapper})
}

// GetUIExtension instantiates a UI extension by name with the given config.
func GetUIExtension(name string, cfg Config) (UIExtension, error) {
	entry, ok := uiExtReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("ui extension %q: %w", name, ErrNotRegistered)
	}

	return entry.factory(ConfigOrDefault(cfg))
}

// UIExtensionRegistered reports whether a UI extension with the given name
// is registered.
func UIExtensionRegistered(name string) bool {
	return uiExtReg.Exists(name)
}

// ListUIExtensions returns the names of all registered UI extensions in sorted order.
func ListUIExtensions() []string {
	return uiExtReg.List()
}

// GetUIExtensions instantiates all registered UI extensions with the given config.
func GetUIExtensions(cfg Config) []UIExtension {
	names := uiExtReg.List()
	exts := make([]UIExtension, 0, len(names))

	for _, name := range names {
		ext, err := GetUIExtension(name, cfg)
		if err != nil {
			continue
		}

		exts = append(exts, ext)
	}

	return exts
}

func ResetUIExtensionRegistry() {
	uiExtReg.Reset()
	ResetSchemasForScope("ui_extensions")
}
