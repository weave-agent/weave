package tui

import (
	"sort"

	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"
)

// ProviderEntry describes a provider with its API key status.
type ProviderEntry struct {
	Name   string
	HasKey bool
}

// Display returns a human-readable label showing provider name and key status.
func (e ProviderEntry) Display() string {
	if e.HasKey {
		return e.Name + "  key set"
	}

	return e.Name + "  no key"
}

// listProviders builds a list of all known providers with their API key status.
// Combines registered providers from sdk.ListProviders() with the model registry
// to include providers that may not be registered yet but have known models.
func listProviders() []ProviderEntry {
	seen := make(map[string]bool)

	var entries []ProviderEntry

	for _, name := range sdk.ListProviders() {
		seen[name] = true
		entries = append(entries, ProviderEntry{
			Name:   name,
			HasKey: sdkmodel.ProviderHasAuth(name),
		})
	}

	for _, md := range sdkmodel.ListAllModels() {
		if !seen[md.Provider] {
			seen[md.Provider] = true
			entries = append(entries, ProviderEntry{
				Name:   md.Provider,
				HasKey: sdkmodel.ProviderHasAuth(md.Provider),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries
}
