package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProviderAuth holds stored credentials for a single provider.
type ProviderAuth struct {
	APIKey string `json:"api_key,omitempty"`
}

// File represents ~/.weave/auth.json.
type File struct {
	Providers map[string]ProviderAuth `json:"providers"`
}

// Path returns the path to the auth file (~/.weave/auth.json).
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("auth path: %w", err)
	}

	return filepath.Join(home, ".weave", "auth.json"), nil
}

// Load reads and parses the auth file. Returns an empty File if not found.
func Load() (*File, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{Providers: make(map[string]ProviderAuth)}, nil
		}

		return nil, fmt.Errorf("read auth: %w", err)
	}

	var auth File
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("parse auth: %w", err)
	}

	if auth.Providers == nil {
		auth.Providers = make(map[string]ProviderAuth)
	}

	return &auth, nil
}

// Save writes the auth file with 0600 permissions.
func Save(auth *File) error {
	p, err := Path()
	if err != nil {
		return err
	}

	dir := filepath.Dir(p)
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return fmt.Errorf("create auth dir: %w", mkdirErr)
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}

	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("write auth: %w", err)
	}

	return nil
}

// GetProviderKey returns the stored API key for a provider, or "" if not set.
func (a *File) GetProviderKey(providerName string) string {
	p, ok := a.Providers[providerName]
	if !ok {
		return ""
	}

	return p.APIKey
}

// SetProviderKey updates or adds a provider key in the auth file and saves.
func SetProviderKey(providerName, apiKey string) error {
	auth, err := Load()
	if err != nil {
		return err
	}

	auth.Providers[providerName] = ProviderAuth{APIKey: apiKey}

	return Save(auth)
}
