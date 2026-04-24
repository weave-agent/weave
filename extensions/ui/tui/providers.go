package tui

import (
	"fmt"
	"sort"

	"weave/config"
	"weave/sdk"
)

// ProviderEntry describes a provider with its API key status.
type ProviderEntry struct {
	Name   string
	HasKey bool
}

// Display returns a human-readable label showing provider name and key status.
func (e ProviderEntry) Display() string {
	if e.HasKey {
		return fmt.Sprintf("%s  key set", e.Name)
	}

	return fmt.Sprintf("%s  no key", e.Name)
}

// listProviders builds a list of all known providers with their API key status.
// Combines registered providers from sdk.ListProviders() with the model registry
// to include providers that may not be registered yet but have known models.
func listProviders() []ProviderEntry {
	auth, err := config.LoadAuth()
	if err != nil {
		auth = &config.AuthFile{Providers: make(map[string]config.ProviderAuth)}
	}

	seen := make(map[string]bool)
	var entries []ProviderEntry

	for _, name := range sdk.ListProviders() {
		seen[name] = true
		entries = append(entries, ProviderEntry{
			Name:   name,
			HasKey: auth.GetProviderKey(name) != "",
		})
	}

	for _, md := range sdk.ListAllModels() {
		if !seen[md.Provider] {
			seen[md.Provider] = true
			entries = append(entries, ProviderEntry{
				Name:   md.Provider,
				HasKey: auth.GetProviderKey(md.Provider) != "",
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries
}
