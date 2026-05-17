package tui

import (
	"sort"
	"strings"

	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"
)

// LoginProviderEntry describes a provider available for login.
type LoginProviderEntry struct {
	Name    string
	ID      string
	IsOAuth bool
	HasAuth bool
}

// buildLoginProviders returns all providers available for login: registered
// providers (API key) plus OAuth providers.
func buildLoginProviders() []LoginProviderEntry {
	seen := make(map[string]bool)
	entries := make([]LoginProviderEntry, 0)

	// Add OAuth providers first
	for _, oauth := range sdk.ListOAuthProviders() {
		seen[oauth.ID] = true
		entries = append(entries, LoginProviderEntry{
			Name:    oauth.Name,
			ID:      oauth.ID,
			IsOAuth: true,
			HasAuth: sdkmodel.ProviderHasAuth(oauth.ID),
		})
	}

	// Add regular providers
	for _, name := range sdk.ListProviders() {
		if seen[name] {
			continue
		}

		seen[name] = true
		entries = append(entries, LoginProviderEntry{
			Name:    displayNameForProvider(name),
			ID:      name,
			IsOAuth: false,
			HasAuth: sdkmodel.ProviderHasAuth(name),
		})
	}

	// Add providers from model registry that aren't registered yet
	for _, md := range sdkmodel.ListAllModels() {
		if seen[md.Provider] {
			continue
		}

		seen[md.Provider] = true
		_, isOAuth := sdk.GetOAuthProvider(md.Provider)
		entries = append(entries, LoginProviderEntry{
			Name:    displayNameForProvider(md.Provider),
			ID:      md.Provider,
			IsOAuth: isOAuth,
			HasAuth: sdkmodel.ProviderHasAuth(md.Provider),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return entries
}

// displayNameForProvider returns a human-readable name for a provider ID.
func displayNameForProvider(id string) string {
	if md, ok := sdkmodel.GetModel(id); ok && md.DisplayName != "" {
		return md.DisplayName
	}

	if id != "" {
		return strings.ToUpper(id[:1]) + id[1:]
	}

	return id
}

// LogoutProviderEntry describes a provider with configured auth.
type LogoutProviderEntry struct {
	Name string
	ID   string
}

// buildLogoutProviders returns only providers that have auth configured.
func buildLogoutProviders() []LogoutProviderEntry {
	seen := make(map[string]bool)
	entries := make([]LogoutProviderEntry, 0)

	for _, name := range sdk.ListProviders() {
		if !sdkmodel.ProviderHasAuth(name) {
			continue
		}

		seen[name] = true
		entries = append(entries, LogoutProviderEntry{
			Name: displayNameForProvider(name),
			ID:   name,
		})
	}

	for _, oauth := range sdk.ListOAuthProviders() {
		if seen[oauth.ID] {
			continue
		}

		if !sdkmodel.ProviderHasAuth(oauth.ID) {
			continue
		}

		seen[oauth.ID] = true
		entries = append(entries, LogoutProviderEntry{
			Name: oauth.Name,
			ID:   oauth.ID,
		})
	}

	for _, md := range sdkmodel.ListAllModels() {
		if seen[md.Provider] {
			continue
		}

		if !sdkmodel.ProviderHasAuth(md.Provider) {
			continue
		}

		seen[md.Provider] = true
		entries = append(entries, LogoutProviderEntry{
			Name: displayNameForProvider(md.Provider),
			ID:   md.Provider,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return entries
}
