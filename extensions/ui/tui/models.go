package tui

import (
	"fmt"
	"os"

	"weave/sdk"
)

// ModelEntry describes a provider + model combination.
type ModelEntry struct {
	Provider string
	Model    string
}

// Display returns a human-readable label for the model entry.
func (e ModelEntry) Display() string {
	return fmt.Sprintf("%s/%s", e.Provider, e.Model)
}

// DisplayName returns the human-friendly name from the model registry,
// falling back to provider/model format.
func (e ModelEntry) DisplayName() string {
	if def, ok := sdk.GetModel(e.Model); ok && def.DisplayName != "" {
		return def.DisplayName
	}

	return e.Display()
}

// listModels returns available model entries by combining registered providers
// with model registry entries. Env var overrides are respected.
func listModels() []ModelEntry {
	allModels := sdk.ListAllModels()
	if len(allModels) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	entries := make([]ModelEntry, 0, len(allModels))
	for _, md := range allModels {
		model := resolveModelName(md)
		key := md.Provider + ":" + model
		if seen[key] {
			continue
		}
		seen[key] = true
		entries = append(entries, ModelEntry{Provider: md.Provider, Model: model})
	}

	return entries
}

// resolveModelName returns the model name for a provider, checking env overrides.
func resolveModelName(md sdk.ModelDef) string {
	envMap := map[string]string{
		"anthropic": "ANTHROPIC_MODEL",
		"openai":    "OPENAI_MODEL",
		"zai":       "ZAI_MODEL",
	}

	if envVar, ok := envMap[md.Provider]; ok {
		if v := os.Getenv(envVar); v != "" {
			return v
		}
	}

	return md.ID
}

// currentModel returns the model entry matching the configured provider
// (from WEAVE_PROVIDER env var), the first available entry as fallback,
// or an anthropic default if no providers are registered.
func currentModel(entries []ModelEntry) ModelEntry {
	if provider := os.Getenv("WEAVE_PROVIDER"); provider != "" {
		for _, e := range entries {
			if e.Provider == provider {
				return e
			}
		}
	}

	if len(entries) > 0 {
		return entries[0]
	}

	return ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}
}

// cycleModel returns the next model entry after the current one, wrapping around.
func cycleModel(entries []ModelEntry, current ModelEntry) ModelEntry {
	for i, e := range entries {
		if e.Provider == current.Provider && e.Model == current.Model {
			next := (i + 1) % len(entries)
			return entries[next]
		}
	}

	if len(entries) > 0 {
		return entries[0]
	}

	return current
}

// modelReasoning returns whether the given model supports reasoning.
func modelReasoning(modelID string) bool {
	if def, ok := sdk.GetModel(modelID); ok {
		return def.Reasoning
	}
	return false
}
