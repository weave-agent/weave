package model

import (
	"cmp"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/weave-agent/weave/sdk/registry"
)

var (
	modelReg = registry.New(
		registry.WithWarn[ModelDef](func(name string) {
			slog.Warn("duplicate registration", "name", name, "kind", "model")
		}),
	)

	authStatus = make(map[string]bool)
	authMu     sync.RWMutex
)

// modelKey returns the canonical registry key for a provider/model pair.
func modelKey(provider, id string) string {
	return provider + "/" + id
}

// ParseModelKey splits a "provider/model" key into its components.
func ParseModelKey(key string) (provider, id string) {
	before, after, found := strings.Cut(key, "/")
	if !found {
		return "", key
	}

	return before, after
}

// RegisterModel adds a model definition to the global registry, keyed by
// provider/model. Same-ID models from different providers are first-class entries.
func RegisterModel(def ModelDef) {
	modelReg.Register(modelKey(def.Provider, def.ID), def)
}

// GetModel returns a model definition by bare ID. When the same ID exists
// across multiple providers, it returns the first match sorted by provider name.
func GetModel(id string) (ModelDef, bool) {
	for _, m := range modelReg.All() {
		if m.ID == id {
			return m, true
		}
	}

	return ModelDef{}, false
}

// GetModelForProvider returns a model definition by ID scoped to a specific provider.
func GetModelForProvider(id, provider string) (ModelDef, bool) {
	return modelReg.Get(modelKey(provider, id))
}

// ListModelsForProvider returns all models for a given provider, sorted by ID.
func ListModelsForProvider(provider string) []ModelDef {
	var result []ModelDef

	for _, m := range modelReg.All() {
		if m.Provider == provider {
			result = append(result, m)
		}
	}

	slices.SortFunc(result, func(a, b ModelDef) int { return cmp.Compare(a.ID, b.ID) })

	return result
}

// ListAllModels returns all registered models, sorted by provider then ID.
func ListAllModels() []ModelDef {
	return modelReg.All()
}

// ListAvailableModels returns models only for providers that have auth,
// sorted by provider then ID.
func ListAvailableModels() []ModelDef {
	authMu.RLock()
	defer authMu.RUnlock()

	var result []ModelDef

	for _, m := range modelReg.All() {
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
	seen := make(map[string]struct{})

	for _, m := range modelReg.All() {
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
}

// ResetAuthRegistry clears all auth status entries. For testing only.
func ResetAuthRegistry() {
	authMu.Lock()
	defer authMu.Unlock()

	authStatus = make(map[string]bool)
}
