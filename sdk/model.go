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
		ID: "claude-opus-4-20250514", Provider: "anthropic",
		DisplayName: "Claude Opus 4", Reasoning: true, SupportsXHigh: true,
		ContextWindow: 200000, MaxTokens: 16384,
	})
	RegisterModel(ModelDef{
		ID: "claude-sonnet-4-20250514", Provider: "anthropic",
		DisplayName: "Claude Sonnet 4", Reasoning: true, SupportsXHigh: false,
		ContextWindow: 200000, MaxTokens: 16384, Default: true,
	})
	// OpenAI
	RegisterModel(ModelDef{
		ID: "gpt-4o", Provider: "openai",
		DisplayName: "GPT-4o", ContextWindow: 128000, MaxTokens: 16384, Default: true,
	})
	RegisterModel(ModelDef{
		ID: "gpt-4o-mini", Provider: "openai",
		DisplayName: "GPT-4o Mini", ContextWindow: 128000, MaxTokens: 16384,
	})
	// ZAI
	RegisterModel(ModelDef{
		ID: "glm-4", Provider: "zai",
		DisplayName: "GLM-4", ContextWindow: 128000, MaxTokens: 8192, Default: true,
	})
	RegisterModel(ModelDef{
		ID: "glm-4-flash", Provider: "zai",
		DisplayName: "GLM-4 Flash", ContextWindow: 128000, MaxTokens: 8192,
	})
}
