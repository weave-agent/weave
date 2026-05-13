package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRegistry(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	RegisterModel(ModelDef{ID: "test-model", Provider: "testprov", DisplayName: "Test Model"})

	m, ok := GetModel("test-model")
	require.True(t, ok)
	assert.Equal(t, "test-model", m.ID)
	assert.Equal(t, "Test Model", m.DisplayName)

	_, ok = GetModel("nonexistent")
	assert.False(t, ok)
}

func TestModelRegistryDuplicateWarns(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	RegisterModel(ModelDef{ID: "dup"})
	// Second registration should NOT panic — it warns and keeps the first.
	assert.NotPanics(t, func() {
		RegisterModel(ModelDef{ID: "dup"})
	})

	// Original registration should be preserved.
	m, ok := GetModel("dup")
	require.True(t, ok)
	assert.Empty(t, m.DisplayName) // first registration had no DisplayName
}

func TestListModelsForProvider(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	RegisterModel(ModelDef{ID: "b-model", Provider: "prov-a"})
	RegisterModel(ModelDef{ID: "a-model", Provider: "prov-a"})
	RegisterModel(ModelDef{ID: "c-model", Provider: "prov-b"})

	models := ListModelsForProvider("prov-a")
	require.Len(t, models, 2)
	assert.Equal(t, "a-model", models[0].ID)
	assert.Equal(t, "b-model", models[1].ID)

	assert.Empty(t, ListModelsForProvider("nonexistent"))
}

func TestListAllModels(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	RegisterModel(ModelDef{ID: "z-model", Provider: "prov"})
	RegisterModel(ModelDef{ID: "a-model", Provider: "prov"})

	all := ListAllModels()
	require.Len(t, all, 2)
	assert.Equal(t, "a-model", all[0].ID)
	assert.Equal(t, "z-model", all[1].ID)
}

func TestDefaultModelForProvider(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	RegisterModel(ModelDef{ID: "first", Provider: "prov"})
	RegisterModel(ModelDef{ID: "second", Provider: "prov"})

	m, ok := DefaultModelForProvider("prov")
	require.True(t, ok)
	assert.Equal(t, "first", m.ID)

	_, ok = DefaultModelForProvider("nonexistent")
	assert.False(t, ok)
}

func TestDefaultModelForProvider_ExplicitDefault(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	RegisterModel(ModelDef{ID: "a", Provider: "prov"})
	RegisterModel(ModelDef{ID: "b", Provider: "prov", Default: true})

	m, ok := DefaultModelForProvider("prov")
	require.True(t, ok)
	assert.Equal(t, "b", m.ID)
}

func TestProviderHasAuth(t *testing.T) {
	ResetAuthRegistry()
	defer ResetAuthRegistry()

	assert.False(t, ProviderHasAuth("anthropic"))

	SetProviderAuth("anthropic", true)
	assert.True(t, ProviderHasAuth("anthropic"))
	assert.False(t, ProviderHasAuth("openai"))

	SetProviderAuth("anthropic", false)
	assert.False(t, ProviderHasAuth("anthropic"))
}

func TestProviderHasAuth_NeverSet(t *testing.T) {
	ResetAuthRegistry()
	defer ResetAuthRegistry()

	assert.False(t, ProviderHasAuth("never-configured-provider"))
}

func TestListAvailableModels(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	ResetAuthRegistry()
	defer ResetAuthRegistry()

	RegisterModel(ModelDef{ID: "z-model", Provider: "prov-a"})
	RegisterModel(ModelDef{ID: "a-model", Provider: "prov-a"})
	RegisterModel(ModelDef{ID: "b-model", Provider: "prov-b"})

	// No auth set — should return empty.
	assert.Empty(t, ListAvailableModels())

	// Auth for prov-a only.
	SetProviderAuth("prov-a", true)

	available := ListAvailableModels()
	require.Len(t, available, 2)
	assert.Equal(t, "a-model", available[0].ID)
	assert.Equal(t, "z-model", available[1].ID)

	// Auth for both providers.
	SetProviderAuth("prov-b", true)

	available = ListAvailableModels()
	require.Len(t, available, 3)
	assert.Equal(t, "prov-a", available[0].Provider)
	assert.Equal(t, "a-model", available[0].ID)
	assert.Equal(t, "prov-a", available[1].Provider)
	assert.Equal(t, "z-model", available[1].ID)
	assert.Equal(t, "prov-b", available[2].Provider)
	assert.Equal(t, "b-model", available[2].ID)
}

func TestListAvailableModels_ProviderSorted(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	ResetAuthRegistry()
	defer ResetAuthRegistry()

	RegisterModel(ModelDef{ID: "m1", Provider: "z-prov"})
	RegisterModel(ModelDef{ID: "m2", Provider: "a-prov"})

	SetProviderAuth("z-prov", true)
	SetProviderAuth("a-prov", true)

	available := ListAvailableModels()
	require.Len(t, available, 2)
	assert.Equal(t, "a-prov", available[0].Provider)
	assert.Equal(t, "z-prov", available[1].Provider)
}
