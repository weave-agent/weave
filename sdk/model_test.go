package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllThinkingLevels(t *testing.T) {
	assert.Len(t, AllThinkingLevels, 6)
	assert.Equal(t, ThinkingOff, AllThinkingLevels[0])
	assert.Equal(t, ThinkingXHigh, AllThinkingLevels[5])
}

func TestParseThinkingLevel(t *testing.T) {
	for _, l := range AllThinkingLevels {
		got, err := ParseThinkingLevel(string(l))
		require.NoError(t, err)
		assert.Equal(t, l, got)
	}

	_, err := ParseThinkingLevel("invalid")
	assert.Error(t, err)
}

func TestIsValidThinkingLevel(t *testing.T) {
	assert.True(t, IsValidThinkingLevel("off"))
	assert.True(t, IsValidThinkingLevel("xhigh"))
	assert.False(t, IsValidThinkingLevel("unknown"))
}

func TestClampForModel(t *testing.T) {
	modelWithXHigh := ModelDef{ID: "test", SupportsXHigh: true}
	modelNoXHigh := ModelDef{ID: "test", SupportsXHigh: false}

	assert.Equal(t, ThinkingXHigh, ClampForModel(ThinkingXHigh, modelWithXHigh))
	assert.Equal(t, ThinkingHigh, ClampForModel(ThinkingXHigh, modelNoXHigh))
	assert.Equal(t, ThinkingMedium, ClampForModel(ThinkingMedium, modelNoXHigh))
	assert.Equal(t, ThinkingOff, ClampForModel(ThinkingOff, modelNoXHigh))
}

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

func TestModelRegistryDuplicatePanics(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	RegisterModel(ModelDef{ID: "dup"})
	assert.Panics(t, func() {
		RegisterModel(ModelDef{ID: "dup"})
	})
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

func TestRegisterBuiltinModels(t *testing.T) {
	ResetModelRegistry()
	defer ResetModelRegistry()

	RegisterBuiltinModels()

	all := ListAllModels()
	assert.Len(t, all, 6)

	// Anthropic
	m, ok := GetModel("claude-opus-4-20250514")
	require.True(t, ok)
	assert.True(t, m.Reasoning)
	assert.True(t, m.SupportsXHigh)
	assert.Equal(t, "anthropic", m.Provider)

	m, ok = GetModel("claude-sonnet-4-20250514")
	require.True(t, ok)
	assert.True(t, m.Reasoning)
	assert.False(t, m.SupportsXHigh)

	// OpenAI
	models := ListModelsForProvider("openai")
	assert.Len(t, models, 2)

	// ZAI
	models = ListModelsForProvider("zai")
	assert.Len(t, models, 2)

	// Default for anthropic is Sonnet (marked Default: true)
	def, ok := DefaultModelForProvider("anthropic")
	require.True(t, ok)
	assert.Equal(t, "claude-sonnet-4-20250514", def.ID)
}

func TestStreamOptions(t *testing.T) {
	opts := StreamOptions{
		Model:         "gpt-4o",
		ThinkingLevel: ThinkingHigh,
		MaxTokens:     4096,
	}
	assert.Equal(t, "gpt-4o", opts.Model)
	assert.Equal(t, ThinkingHigh, opts.ThinkingLevel)
	assert.Equal(t, int64(4096), opts.MaxTokens)
}

func TestNewStreamOptions_Defaults(t *testing.T) {
	opts := NewStreamOptions()
	assert.Empty(t, opts.Model)
	assert.Equal(t, ThinkingOff, opts.ThinkingLevel)
	assert.Equal(t, int64(0), opts.MaxTokens)
}

func TestNewStreamOptions_FunctionalOptions(t *testing.T) {
	opts := NewStreamOptions(
		WithModel("claude-opus-4-20250514"),
		WithThinkingLevel(ThinkingHigh),
		WithMaxTokens(8192),
	)
	assert.Equal(t, "claude-opus-4-20250514", opts.Model)
	assert.Equal(t, ThinkingHigh, opts.ThinkingLevel)
	assert.Equal(t, int64(8192), opts.MaxTokens)
}

func TestNewStreamOptions_PartialOptions(t *testing.T) {
	opts := NewStreamOptions(WithThinkingLevel(ThinkingMedium))
	assert.Empty(t, opts.Model)
	assert.Equal(t, ThinkingMedium, opts.ThinkingLevel)
	assert.Equal(t, int64(0), opts.MaxTokens)
}
