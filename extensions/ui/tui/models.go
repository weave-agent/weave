package tui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"
	"github.com/weave-agent/weave/settings"
)

// preferences mirrors the settings fields used by the TUI for model/thinking persistence.
type preferences struct {
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	ThinkingLevel string `json:"thinking_level,omitempty"`
}

func preferenceStoreOrDefault(ps sdk.PreferenceStore) sdk.PreferenceStore {
	if ps != nil {
		return ps
	}

	return sdk.NoopPreferenceStore{}
}

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
	if def, ok := sdkmodel.GetModelForProvider(e.Model, e.Provider); ok && def.DisplayName != "" {
		return def.DisplayName
	}

	return e.Display()
}

// listModels returns model entries for providers that are registered and have
// valid auth credentials.
func listModels() []ModelEntry {
	registered := sdk.ListProviders()

	regSet := make(map[string]bool, len(registered))
	for _, p := range registered {
		regSet[p] = true
	}

	var entries []ModelEntry

	for _, md := range sdkmodel.ListAvailableModels() {
		if !regSet[md.Provider] {
			continue
		}

		entries = append(entries, ModelEntry{Provider: md.Provider, Model: md.ID})
	}

	return entries
}

// modelFromSettings resolves a model entry from persisted preferences. When
// the model field is empty or stale it falls back to the provider's default.
// Only returns entries whose provider has a configured key (entries is pre-filtered).
func modelFromSettings(entries []ModelEntry, prefs preferences) ModelEntry {
	if prefs.Model != "" {
		for _, e := range entries {
			if e.Provider == prefs.Provider && e.Model == prefs.Model {
				return e
			}
		}
	}

	for _, e := range entries {
		if e.Provider == prefs.Provider {
			return e
		}
	}

	return ModelEntry{}
}

// currentModel returns the startup model entry using the same priority as the
// loop's provider resolver: WEAVE_PROVIDER env var > settings > first registered > fallback.
func currentModel(entries []ModelEntry, ps sdk.PreferenceStore) ModelEntry {
	ps = preferenceStoreOrDefault(ps)

	// 1. WEAVE_PROVIDER env var — highest priority (matches loop resolver).
	if provider := os.Getenv("WEAVE_PROVIDER"); provider != "" {
		if def, ok := sdkmodel.DefaultModelForProvider(provider); ok {
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
	}

	// 2. Persisted settings.
	var prefs preferences
	if ps.Preferences(&prefs) == nil && prefs.Provider != "" {
		if e := modelFromSettings(entries, prefs); e.Provider != "" {
			return e
		}
	}

	// 3. First registered provider.
	if providers := sdk.ListProviders(); len(providers) > 0 {
		provider := providers[0]
		if def, ok := sdkmodel.DefaultModelForProvider(provider); ok {
			for _, e := range entries {
				if e.Provider == provider {
					return ModelEntry{Provider: provider, Model: def.ID}
				}
			}
		}
	}

	// 4. First available entry.
	if len(entries) > 0 {
		return entries[0]
	}

	return ModelEntry{}
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

// modelReasoning returns whether the given model entry supports reasoning.
func modelReasoning(entry ModelEntry) bool {
	if def, ok := sdkmodel.GetModelForProvider(entry.Model, entry.Provider); ok {
		return def.Reasoning
	}

	return false
}

// initialThinkingLevel returns the startup thinking level. Tries persisted
// settings first, then the WEAVE_THINKING_LEVEL env var, then medium.
func initialThinkingLevel(ps sdk.PreferenceStore) sdkmodel.ThinkingLevel {
	ps = preferenceStoreOrDefault(ps)

	var prefs preferences
	if ps.Preferences(&prefs) == nil && prefs.ThinkingLevel != "" {
		if lvl, err := sdkmodel.ParseThinkingLevel(prefs.ThinkingLevel); err == nil {
			return lvl
		}
	}

	return settings.DefaultThinkingLevel()
}

// saveSettings persists the current model and thinking level to the global
// settings file via PreferenceStore. Best-effort — errors are silently ignored.
func saveSettings(ps sdk.PreferenceStore, entry ModelEntry, level sdkmodel.ThinkingLevel) {
	ps = preferenceStoreOrDefault(ps)
	prefs := preferences{
		Provider:      entry.Provider,
		Model:         entry.Model,
		ThinkingLevel: string(level),
	}

	_ = ps.SavePreferences(&prefs)
}

// saveSettingsCmd returns a tea.Cmd that persists settings asynchronously.
func saveSettingsCmd(ps sdk.PreferenceStore, entry ModelEntry, level sdkmodel.ThinkingLevel) tea.Cmd {
	return func() tea.Msg {
		saveSettings(ps, entry, level)
		return nil
	}
}
