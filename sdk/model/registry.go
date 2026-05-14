package model

import (
	"cmp"
	"log/slog"
	"slices"
	"sync"

	"weave/sdk/registry"
)

var modelReg = registry.New[ModelDef](
	registry.WithWarn[ModelDef](func(name string) {
		slog.Warn("duplicate registration", "name", name, "kind", "model")
	}),
)

var (
	authStatus = make(map[string]bool)
	authMu     sync.RWMutex
)

// RegisterModel adds a model definition to the global registry.
// It warns on duplicate ID, keeping the first registration.
func RegisterModel(def ModelDef) {
	modelReg.Register(def.ID, def)
}

// GetModel returns a model definition by ID.
func GetModel(id string) (ModelDef, bool) {
	return modelReg.Get(id)
}

// ListModelsForProvider returns all models for a given provider, sorted by ID.
func ListModelsForProvider(provider string) []ModelDef {
	all := modelReg.All()

	var result []ModelDef

	for _, m := range all {
		if m.Provider == provider {
			result = append(result, m)
		}
	}

	slices.SortFunc(result, func(a, b ModelDef) int { return cmp.Compare(a.ID, b.ID) })

	return result
}

// ListAllModels returns all registered models, sorted by ID.
func ListAllModels() []ModelDef {
	return modelReg.All()
}

// ListAvailableModels returns models only for providers that have auth,
// sorted by provider then ID.
func ListAvailableModels() []ModelDef {
	authMu.RLock()
	defer authMu.RUnlock()

	all := modelReg.All()

	var result []ModelDef

	for _, m := range all {
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
}

// ResetAuthRegistry clears all auth status entries. For testing only.
func ResetAuthRegistry() {
	authMu.Lock()
	defer authMu.Unlock()

	authStatus = make(map[string]bool)
}
