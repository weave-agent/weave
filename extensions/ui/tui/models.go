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

// knownModels maps provider names to their default model names.
// Providers may override via env vars (e.g. ANTHROPIC_MODEL).
var knownModels = map[string]string{
	"anthropic": "claude-sonnet-4-20250514",
	"openai":    "gpt-4o",
	"zai":       "glm-4",
}

// listModels returns available model entries by combining registered providers
// with known model names. Env var overrides are respected.
func listModels() []ModelEntry {
	providers := sdk.ListProviders()
	if len(providers) == 0 {
		return nil
	}

	entries := make([]ModelEntry, 0, len(providers))
	for _, p := range providers {
		model := resolveModelName(p)
		entries = append(entries, ModelEntry{Provider: p, Model: model})
	}

	return entries
}

// resolveModelName returns the model name for a provider, checking env overrides.
func resolveModelName(provider string) string {
	envMap := map[string]string{
		"anthropic": "ANTHROPIC_MODEL",
		"openai":    "OPENAI_MODEL",
		"zai":       "ZAI_MODEL",
	}

	if envVar, ok := envMap[provider]; ok {
		if v := os.Getenv(envVar); v != "" {
			return v
		}
	}

	if m, ok := knownModels[provider]; ok {
		return m
	}

	return "default"
}

// currentModel returns the first model entry as a reasonable default,
// or an anthropic default if no providers are registered.
func currentModel(entries []ModelEntry) ModelEntry {
	if len(entries) > 0 {
		return entries[0]
	}
	return ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}
}

// cycleModel returns the next model entry after the current one, wrapping around.
func cycleModel(entries []ModelEntry, current ModelEntry) ModelEntry {
	for i, e := range entries {
		if e.Provider == current.Provider {
			next := (i + 1) % len(entries)
			return entries[next]
		}
	}

	if len(entries) > 0 {
		return entries[0]
	}

	return current
}
