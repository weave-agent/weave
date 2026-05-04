package sdk

import (
	"fmt"
	"os"
	"sort"
	"sync"
)

// ThinkingLevel represents the reasoning depth for a model request.
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"

	// ProviderAnthropic is the Anthropic provider name.
	ProviderAnthropic = "anthropic"
	// ProviderOpenAI is the OpenAI provider name.
	ProviderOpenAI = "openai"
)

// AllThinkingLevels is the ordered list of all thinking levels.
var AllThinkingLevels = []ThinkingLevel{
	ThinkingOff, ThinkingMinimal, ThinkingLow,
	ThinkingMedium, ThinkingHigh, ThinkingXHigh,
}

// ModelDef describes a model's metadata and capabilities.
type ModelDef struct {
	ID            string
	Provider      string
	DisplayName   string
	Reasoning     bool
	SupportsXHigh bool
	ContextWindow int
	MaxTokens     int
	Default       bool
}

// StreamOptions configures per-request behavior for provider streaming.
type StreamOptions struct {
	Model         string
	ThinkingLevel ThinkingLevel
	MaxTokens     int64
}

// ClampForModel returns the level capped to what the model supports.
func ClampForModel(level ThinkingLevel, model ModelDef) ThinkingLevel {
	if level == ThinkingXHigh && !model.SupportsXHigh {
		return ThinkingHigh
	}

	return level
}

// DefaultThinkingLevel reads the initial thinking level from WEAVE_THINKING_LEVEL,
// falling back to ThinkingMedium.
func DefaultThinkingLevel() ThinkingLevel {
	if v := os.Getenv("WEAVE_THINKING_LEVEL"); v != "" {
		if lvl, err := ParseThinkingLevel(v); err == nil {
			return lvl
		}
	}

	return ThinkingMedium
}

// ParseThinkingLevel converts a string to a ThinkingLevel, returning an error if invalid.
func ParseThinkingLevel(s string) (ThinkingLevel, error) {
	for _, l := range AllThinkingLevels {
		if string(l) == s {
			return l, nil
		}
	}

	return "", fmt.Errorf("invalid thinking level %q (valid: off, minimal, low, medium, high, xhigh)", s)
}

// provider env var registry — maps provider names to their API key env vars.

var (
	providerEnvMu  sync.RWMutex
	providerEnvMap = make(map[string]string)
)

// RegisterProviderEnvVar registers the environment variable name for a provider's API key.
func RegisterProviderEnvVar(providerName, envVar string) {
	providerEnvMu.Lock()
	defer providerEnvMu.Unlock()

	providerEnvMap[providerName] = envVar
}

// ProviderEnvVar returns the environment variable name for a provider's API key.
func ProviderEnvVar(providerName string) string {
	providerEnvMu.RLock()
	defer providerEnvMu.RUnlock()

	return providerEnvMap[providerName]
}

// ResetProviderEnvVarRegistry clears all registered env var mappings. For testing only.
func ResetProviderEnvVarRegistry() {
	providerEnvMu.Lock()
	defer providerEnvMu.Unlock()

	providerEnvMap = make(map[string]string)
}

// model registry

var (
	modelMu  sync.RWMutex
	modelReg = make(map[string]ModelDef)
)

// RegisterModel adds a model definition to the global registry.
// It panics on duplicate ID.
func RegisterModel(def ModelDef) {
	modelMu.Lock()
	defer modelMu.Unlock()

	if _, dup := modelReg[def.ID]; dup {
		panic("sdk: RegisterModel called twice for " + def.ID)
	}

	modelReg[def.ID] = def
}

// GetModel returns a model definition by ID.
func GetModel(id string) (ModelDef, bool) {
	modelMu.RLock()
	defer modelMu.RUnlock()

	m, ok := modelReg[id]

	return m, ok
}

// ListModelsForProvider returns all models for a given provider, sorted by ID.
func ListModelsForProvider(provider string) []ModelDef {
	modelMu.RLock()
	defer modelMu.RUnlock()

	var result []ModelDef

	for _, m := range modelReg {
		if m.Provider == provider {
			result = append(result, m)
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })

	return result
}

// ListAllModels returns all registered models, sorted by ID.
func ListAllModels() []ModelDef {
	modelMu.RLock()
	defer modelMu.RUnlock()

	result := make([]ModelDef, 0, len(modelReg))
	for _, m := range modelReg {
		result = append(result, m)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })

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

// ResetModelRegistry clears all registered models. For testing only.
func ResetModelRegistry() {
	modelMu.Lock()
	defer modelMu.Unlock()

	modelReg = make(map[string]ModelDef)
}

func init() { //nolint:gochecknoinits // required to populate model registry before extensions access it
	RegisterBuiltinModels()
}

// RegisterBuiltinModels registers the curated model entries for built-in providers.
func RegisterBuiltinModels() {
	// Anthropic
	RegisterModel(ModelDef{
		ID: "claude-opus-4-7", Provider: ProviderAnthropic,
		DisplayName: "Claude Opus 4.7", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 1000000, MaxTokens: 128000,
	})
	RegisterModel(ModelDef{
		ID: "claude-sonnet-4-6", Provider: ProviderAnthropic,
		DisplayName: "Claude Sonnet 4.6", Reasoning: true, SupportsXHigh: false,
		ContextWindow: 1000000, MaxTokens: 64000, Default: true,
	})
	RegisterModel(ModelDef{
		ID: "claude-opus-4-5", Provider: ProviderAnthropic,
		DisplayName: "Claude Opus 4.5", Reasoning: true, SupportsXHigh: false,
		ContextWindow: 200000, MaxTokens: 64000,
	})
	RegisterModel(ModelDef{
		ID: "claude-sonnet-4-5", Provider: ProviderAnthropic,
		DisplayName: "Claude Sonnet 4.5", Reasoning: true, SupportsXHigh: false,
		ContextWindow: 200000, MaxTokens: 64000,
	})
	RegisterModel(ModelDef{
		ID: "claude-haiku-4-5", Provider: ProviderAnthropic,
		DisplayName: "Claude Haiku 4.5", Reasoning: true, SupportsXHigh: false,
		ContextWindow: 200000, MaxTokens: 64000,
	})
	// OpenAI
	RegisterModel(ModelDef{
		ID: "gpt-5.5", Provider: ProviderOpenAI,
		DisplayName: "GPT-5.5", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 1050000, MaxTokens: 128000, Default: true,
	})
	RegisterModel(ModelDef{
		ID: "gpt-5.4", Provider: ProviderOpenAI,
		DisplayName: "GPT-5.4", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 272000, MaxTokens: 128000,
	})
	RegisterModel(ModelDef{
		ID: "gpt-5.2", Provider: ProviderOpenAI,
		DisplayName: "GPT-5.2", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 400000, MaxTokens: 128000,
	})
	RegisterModel(ModelDef{
		ID: "gpt-4.1", Provider: ProviderOpenAI,
		DisplayName: "GPT-4.1", ContextWindow: 1047576, MaxTokens: 32768,
	})
	RegisterModel(ModelDef{
		ID: "o4-mini", Provider: ProviderOpenAI,
		DisplayName: "o4-mini", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 100000,
	})
	RegisterModel(ModelDef{
		ID: "o3", Provider: ProviderOpenAI,
		DisplayName: "o3", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 100000,
	})
	// ZAI
	RegisterModel(ModelDef{
		ID: "glm-5.1", Provider: "zai",
		DisplayName: "GLM-5.1", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 131072, Default: true,
	})
	RegisterModel(ModelDef{
		ID: "glm-5", Provider: "zai",
		DisplayName: "GLM-5", Reasoning: true,
		ContextWindow: 204800, MaxTokens: 131072,
	})
	RegisterModel(ModelDef{
		ID: "glm-4.7", Provider: "zai",
		DisplayName: "GLM-4.7", Reasoning: true,
		ContextWindow: 204800, MaxTokens: 131072,
	})
	RegisterModel(ModelDef{
		ID: "glm-4.7-flash", Provider: "zai",
		DisplayName: "GLM-4.7 Flash", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 131072,
	})
	RegisterModel(ModelDef{
		ID: "glm-4.7-flashx", Provider: "zai",
		DisplayName: "GLM-4.7 FlashX", Reasoning: true,
		ContextWindow: 200000, MaxTokens: 131072,
	})
	RegisterModel(ModelDef{
		ID: "glm-4.5-air", Provider: "zai",
		DisplayName: "GLM-4.5 Air", Reasoning: true,
		ContextWindow: 131072, MaxTokens: 98304,
	})
}
