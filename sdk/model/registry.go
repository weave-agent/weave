package model

import (
	"cmp"
	"log"
	"os"
	"slices"

	"weave/sdk/registry"
)

var modelReg = registry.New[ModelDef](
	registry.WithWarn[ModelDef](log.New(os.Stderr, "weave: ", 0), "model"),
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

// ResetModelRegistry clears all registered models. For testing only.
func ResetModelRegistry() {
	modelReg.Reset()
}
