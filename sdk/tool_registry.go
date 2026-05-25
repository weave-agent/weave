package sdk

import (
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"sync"

	"github.com/weave-agent/weave/sdk/registry"
)

var toolReg = registry.New[func(Config) (Tool, error)](
	registry.WithWarn[func(Config) (Tool, error)](func(name string) {
		slog.Warn("duplicate registration", "name", name, "kind", "tool")
	}),
)

// RegisterTool registers a tool factory with a typed configuration struct.
// The framework will automatically populate the config struct from settings, env vars,
// and CLI flags before calling the factory.
func RegisterTool[T any](name string, factory func(Config, PreferenceReader, T) (Tool, error)) {
	var zero T

	typ := reflect.TypeOf(zero)
	schema := extractSchema(typ)
	storeSchema("tools", name, schema, typ)

	wrapper := func(cfg Config) (Tool, error) {
		var t T

		if err := cfg.ExtensionConfig("tools", name, &t); err != nil {
			return nil, fmt.Errorf("load tool config: %w", err)
		}

		return factory(ConfigReadOnly(cfg), PreferenceStoreFrom(cfg), t)
	}

	toolReg.Register(name, wrapper)
}

func GetTool(name string, cfg Config) (Tool, error) {
	toolFilterMu.RLock()

	filter := toolFilter

	toolFilterMu.RUnlock()

	if filter != nil && !filter[name] {
		return nil, fmt.Errorf("tool %q not in allowed list", name)
	}

	factory, ok := toolReg.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool %q: %w", name, ErrNotRegistered)
	}

	return factory(ConfigOrDefault(cfg))
}

func ToolRegistered(name string) bool {
	return toolReg.Exists(name)
}

var (
	toolFilter   map[string]bool
	toolFilterMu sync.RWMutex
)

func SetToolFilter(names []string) {
	toolFilterMu.Lock()
	defer toolFilterMu.Unlock()

	if names == nil {
		toolFilter = nil
		return
	}

	toolFilter = make(map[string]bool, len(names))
	for _, name := range names {
		toolFilter[name] = true
	}
}

func GetToolFilter() []string {
	toolFilterMu.RLock()
	defer toolFilterMu.RUnlock()

	if toolFilter == nil {
		return nil
	}

	result := make([]string, 0, len(toolFilter))
	for name := range toolFilter {
		result = append(result, name)
	}

	sort.Strings(result)

	return result
}

func ListTools() []string {
	toolFilterMu.RLock()

	filter := toolFilter

	toolFilterMu.RUnlock()

	all := toolReg.List()
	if filter == nil {
		return all
	}

	filtered := make([]string, 0, len(filter))
	for _, name := range all {
		if filter[name] {
			filtered = append(filtered, name)
		}
	}

	return filtered
}

func ResetToolRegistry() {
	toolReg.Reset()

	toolFilterMu.Lock()
	toolFilter = nil
	toolFilterMu.Unlock()

	ResetSchemasForScope("tools")
}
