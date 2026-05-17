package model

import (
	"cmp"
	"log/slog"
	"slices"
	"sync"

	"weave/sdk/registry"
)

var (
	modelReg = registry.New(
		registry.WithWarn[ModelDef](func(name string) {
			slog.Warn("duplicate registration", "name", name, "kind", "model")
		}),
	)

	allModels   []ModelDef
	allModelsMu sync.RWMutex

	authStatus = make(map[string]bool)
	authMu     sync.RWMutex
)

// RegisterModel adds a model definition to the global registry.
// It warns on duplicate ID, keeping the first registration for bare-ID lookup,
// but preserves all models so that provider-scoped lookups remain correct.
func RegisterModel(def ModelDef) {
	modelReg.Register(def.ID, def)

	allModelsMu.Lock()
	defer allModelsMu.Unlock()

	for _, m := range allModels {
		if m.ID == def.ID && m.Provider == def.Provider {
			return
		}
	}

	allModels = append(allModels, def)
}

// GetModel returns a model definition by ID.
// On duplicate IDs across providers, returns the first-registered one.
func GetModel(id string) (ModelDef, bool) {
	return modelReg.Get(id)
}

// GetModelForProvider returns a model definition by ID scoped to a specific
// provider. Use this when duplicate model IDs exist across providers.
func GetModelForProvider(id, provider string) (ModelDef, bool) {
	allModelsMu.RLock()
	defer allModelsMu.RUnlock()

	for _, m := range allModels {
		if m.ID == id && m.Provider == provider {
			return m, true
		}
	}

	return ModelDef{}, false
}

// ListModelsForProvider returns all models for a given provider, sorted by ID.
func ListModelsForProvider(provider string) []ModelDef {
	allModelsMu.RLock()
	defer allModelsMu.RUnlock()

	var result []ModelDef

	for _, m := range allModels {
		if m.Provider == provider {
			result = append(result, m)
		}
	}

	slices.SortFunc(result, func(a, b ModelDef) int { return cmp.Compare(a.ID, b.ID) })

	return result
}

// ListAllModels returns all registered models, sorted by ID.
func ListAllModels() []ModelDef {
	allModelsMu.RLock()
	defer allModelsMu.RUnlock()

	result := make([]ModelDef, len(allModels))
	copy(result, allModels)

	slices.SortFunc(result, func(a, b ModelDef) int { return cmp.Compare(a.ID, b.ID) })

	return result
}

// ListAvailableModels returns models only for providers that have auth,
// sorted by provider then ID.
func ListAvailableModels() []ModelDef {
	authMu.RLock()
	defer authMu.RUnlock()

	allModelsMu.RLock()
	defer allModelsMu.RUnlock()

	var result []ModelDef

	for _, m := range allModels {
		if authStatus[m.Provider] {
			result = append(result, m)
		}
	}

	slices.SortFunc(result, func(a, b ModelDef) int {
		if a.Provider != b.Provider {
			return cmp.Compare(a.Provider, b.Provider)
		}

		return cmp.Compare(a.ID, b.ID)
	})

	return result
}

// ModelProviderCount returns the number of distinct providers that have
// registered a model with the given ID. Used to detect ambiguous model overrides.
func ModelProviderCount(id string) int {
	allModelsMu.RLock()
	defer allModelsMu.RUnlock()

	seen := make(map[string]struct{})

	for _, m := range allModels {
		if m.ID == id {
			seen[m.Provider] = struct{}{}
		}
	}

	return len(seen)
}

// DefaultModelForProvider returns the default model for the provider.
func DefaultModelForProvider(provider string) (ModelDef, bool) {
	models := ListModelsForProvider(provider)
	if len(models) == 0 {
		return ModelDef{}, false
	}

	for _, m := range models {
		if m.Default {
			return m, true
		}
	}

	return models[0], true
}

// SetProviderAuth sets the auth status for a provider.
func SetProviderAuth(provider string, hasAuth bool) {
	authMu.Lock()
	defer authMu.Unlock()

	authStatus[provider] = hasAuth
}

// ProviderHasAuth returns whether the provider has valid auth credentials.
func ProviderHasAuth(provider string) bool {
	authMu.RLock()
	defer authMu.RUnlock()

	return authStatus[provider]
}

// ResetModelRegistry clears all registered models. For testing only.
func ResetModelRegistry() {
	modelReg.Reset()

	allModelsMu.Lock()
	allModels = nil
	allModelsMu.Unlock()
}

// ResetAuthRegistry clears all auth status entries. For testing only.
func ResetAuthRegistry() {
	authMu.Lock()
	defer authMu.Unlock()

	authStatus = make(map[string]bool)
}
