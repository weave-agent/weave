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

func TestProviderEnvVarRegistry(t *testing.T) {
	ResetProviderEnvVarRegistry()
	defer ResetProviderEnvVarRegistry()

	RegisterProviderEnvVar("test-provider", "TEST_API_KEY")
	assert.Equal(t, "TEST_API_KEY", ProviderEnvVar("test-provider"))
	assert.Empty(t, ProviderEnvVar("nonexistent"))
}

func TestProviderEnvVarRegistry_DuplicateOverwrites(t *testing.T) {
	ResetProviderEnvVarRegistry()
	defer ResetProviderEnvVarRegistry()

	RegisterProviderEnvVar("test", "FIRST_KEY")
	RegisterProviderEnvVar("test", "SECOND_KEY")
	assert.Equal(t, "FIRST_KEY", ProviderEnvVar("test"), "first registration should win")
}
