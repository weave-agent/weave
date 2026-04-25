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

// providerModelEnv maps provider names to their model override env vars.
var providerModelEnv = map[string]string{
	"anthropic": "ANTHROPIC_MODEL",
	"openai":    "OPENAI_MODEL",
	"zai":       "ZAI_MODEL",
}

// listModels returns model entries for providers that are actually registered
// (i.e. compiled into the binary).
func listModels() []ModelEntry {
	registered := sdk.ListProviders()

	regSet := make(map[string]bool, len(registered))
	for _, p := range registered {
		regSet[p] = true
	}

	var entries []ModelEntry

	for _, md := range sdk.ListAllModels() {
		if !regSet[md.Provider] {
			continue
		}

		entries = append(entries, ModelEntry{Provider: md.Provider, Model: md.ID})
	}

	return entries
}

// currentModel returns the default model entry for the configured provider
// (from WEAVE_PROVIDER env var, defaulting to "anthropic"), falling back to
// the first registry entry, or an anthropic default if no providers are registered.
func currentModel(entries []ModelEntry) ModelEntry {
	provider := os.Getenv("WEAVE_PROVIDER")
	if provider == "" {
		provider = "anthropic"
	}

	if def, ok := sdk.DefaultModelForProvider(provider); ok {
		for _, e := range entries {
			if e.Provider == provider {
				return ModelEntry{Provider: provider, Model: def.ID}
			}
		}
	}

	for _, e := range entries {
		if e.Provider == provider {
			return e
		}
	}

	if len(entries) > 0 {
		return entries[0]
	}

	return ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}
}

// initialModel returns the model entry to use at TUI startup. It applies
// provider-specific env var overrides (ANTHROPIC_MODEL, OPENAI_MODEL, ZAI_MODEL)
// so the display matches what the provider will actually use for the first request.
func initialModel(entries []ModelEntry) ModelEntry {
	cur := currentModel(entries)

	if envKey, ok := providerModelEnv[cur.Provider]; ok {
		if m := os.Getenv(envKey); m != "" {
			cur.Model = m
		}
	}

	return cur
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
