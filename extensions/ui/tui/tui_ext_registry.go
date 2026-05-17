package tui

import (
	"fmt"
	"log/slog"

	"github.com/weave-agent/weave/sdk"
	"github.com/weave-agent/weave/sdk/registry"
)

type tuiExtEntry struct {
	factory func(sdk.Config) (TUIExtension, error)
}

var tuiExtReg = registry.New[tuiExtEntry](
	registry.WithWarn[tuiExtEntry](func(name string) {
		slog.Warn("duplicate registration", "name", name, "kind", "tui extension")
	}),
)

// RegisterTUIExtension registers a TUI extension factory.
// The framework will automatically populate the config struct from settings, env vars,
// and CLI flags before calling the factory.
func RegisterTUIExtension[TConfig any](name string, factory func(sdk.Config, sdk.PreferenceReader, TConfig) (TUIExtension, error)) {
	var zero TConfig
	sdk.RegisterExtensionSchema("ui_extensions", name, zero)

	wrapper := func(cfg sdk.Config) (TUIExtension, error) {
		var t TConfig

		if err := cfg.ExtensionConfig("ui_extensions", name, &t); err != nil {
			return nil, fmt.Errorf("load tui extension config: %w", err)
		}

		return factory(sdk.ConfigReadOnly(cfg), sdk.PreferenceStoreFrom(cfg), t)
	}

	tuiExtReg.Register(name, tuiExtEntry{factory: wrapper})
}

// GetTUIExtension instantiates a TUI extension by name with the given config.
func GetTUIExtension(name string, cfg sdk.Config) (TUIExtension, error) {
	entry, ok := tuiExtReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("tui extension %q: %w", name, sdk.ErrNotRegistered)
	}

	return entry.factory(sdk.ConfigOrDefault(cfg))
}

// TUIExtensionRegistered reports whether a TUI extension with the given name
// is registered.
func TUIExtensionRegistered(name string) bool {
	return tuiExtReg.Exists(name)
}

// ListTUIExtensions returns the names of all registered TUI extensions in sorted order.
func ListTUIExtensions() []string {
	return tuiExtReg.List()
}

// GetTUIExtensions instantiates all registered TUI extensions with the given config.
func GetTUIExtensions(cfg sdk.Config) []TUIExtension {
	names := tuiExtReg.List()
	exts := make([]TUIExtension, 0, len(names))

	for _, name := range names {
		ext, err := GetTUIExtension(name, cfg)
		if err != nil {
			sdk.Logger("tui").Error("failed to instantiate TUI extension", "name", name, "error", err)

			continue
		}

		exts = append(exts, ext)
	}

	return exts
}

// ResetTUIExtensionRegistry clears all registered TUI extensions.
func ResetTUIExtensionRegistry() {
	tuiExtReg.Reset()
}
