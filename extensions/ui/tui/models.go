package tui

import (
	"fmt"
	"os"

	"weave/config"
	"weave/sdk"

	tea "charm.land/bubbletea/v2"
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

// listModels returns model entries for providers that are registered and have
// an API key configured.
func listModels() []ModelEntry {
	registered := sdk.ListProviders()

	regSet := make(map[string]bool, len(registered))
	for _, p := range registered {
		regSet[p] = true
	}

	auth, _ := config.LoadAuth()

	var entries []ModelEntry

	for _, md := range sdk.ListAllModels() {
		if !regSet[md.Provider] {
			continue
		}

		if !providerHasKey(md.Provider, auth) {
			continue
		}

		entries = append(entries, ModelEntry{Provider: md.Provider, Model: md.ID})
	}

	return entries
}

// providerHasKey checks whether a provider has an API key configured
// via environment variable or auth file.
func providerHasKey(providerName string, auth *config.AuthFile) bool {
	envVar := sdk.ProviderEnvVar(providerName)
	if envVar == "" {
		return false
	}

	if os.Getenv(envVar) != "" {
		return true
	}

	if auth != nil && auth.GetProviderKey(providerName) != "" {
		return true
	}

	return false
}

// currentModel returns the startup model entry. It tries persisted settings
// first, then the WEAVE_PROVIDER env var, then falls back to the first
// available entry.
func currentModel(entries []ModelEntry) ModelEntry {
	if prefs, err := config.LoadSettings(); err == nil {
		if prefs.Provider != "" && prefs.Model != "" {
			for _, e := range entries {
				if e.Provider == prefs.Provider && e.Model == prefs.Model {
					return e
				}
			}
		}
	}

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

// initialThinkingLevel returns the startup thinking level. Tries persisted
// settings first, then the WEAVE_THINKING_LEVEL env var, then medium.
func initialThinkingLevel() sdk.ThinkingLevel {
	if prefs, err := config.LoadSettings(); err == nil && prefs.ThinkingLevel != "" {
		if lvl, err := sdk.ParseThinkingLevel(prefs.ThinkingLevel); err == nil {
			return lvl
		}
	}

	return sdk.DefaultThinkingLevel()
}

// saveSettings persists the current model and thinking level to disk.
// Best-effort — errors are silently ignored.
func saveSettings(entry ModelEntry, level sdk.ThinkingLevel) {
	_ = config.SaveSettingsGlobal(&config.Settings{
		Provider:      entry.Provider,
		Model:         entry.Model,
		ThinkingLevel: string(level),
	})
}

// saveSettingsCmd returns a tea.Cmd that persists settings asynchronously.
func saveSettingsCmd(entry ModelEntry, level sdk.ThinkingLevel) tea.Cmd {
	return func() tea.Msg {
		saveSettings(entry, level)
		return nil
	}
}
