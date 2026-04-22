package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var commandCache sync.Map

// ResolveValue resolves a config value string. The string can be:
//   - "!command args..." → execute command via sh and capture stdout (trimmed, cached per process)
//   - literal string → use as-is
func ResolveValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}

	if !strings.HasPrefix(raw, "!") {
		return raw, nil
	}

	cmd := raw[1:]

	if cached, ok := commandCache.Load(cmd); ok {
		return cached.(string), nil
	}

	out, err := exec.CommandContext(context.Background(), "sh", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("resolve command %q: %w", cmd, err)
	}

	result := strings.TrimSpace(string(out))
	commandCache.Store(cmd, result)

	return result, nil
}

// ResolveProviderKey resolves the API key for a provider using the full chain:
//  1. Environment variable (e.g. ANTHROPIC_API_KEY)
//  2. Auth file (~/.weave/auth.json)
//  3. Config file provider entry (providers.anthropic.api_key) via ResolveValue
func ResolveProviderKey(providerName, envVar string, cfgEntry *ProviderEntry, auth *AuthFile) (string, error) {
	// 1. Environment variable (highest priority)
	if v := os.Getenv(envVar); v != "" {
		return v, nil
	}

	// 2. Auth file
	if auth != nil {
		if v := auth.GetProviderKey(providerName); v != "" {
			return v, nil
		}
	}

	// 3. Config file entry
	if cfgEntry != nil && cfgEntry.APIKey != "" {
		resolved, err := ResolveValue(cfgEntry.APIKey)
		if err != nil {
			return "", fmt.Errorf("provider %s: %w", providerName, err)
		}

		if resolved != "" {
			return resolved, nil
		}
	}

	return "", nil
}
