package settings

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"weave/internal/auth"
)

var (
	commandCache   = make(map[string]string)
	commandCacheMu sync.Mutex
)

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

	commandCacheMu.Lock()
	if cached, ok := commandCache[cmd]; ok {
		commandCacheMu.Unlock()
		return cached, nil
	}
	commandCacheMu.Unlock()

	out, err := exec.CommandContext(context.Background(), "sh", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("resolve command %q: %w", cmd, err)
	}

	result := strings.TrimSpace(string(out))

	commandCacheMu.Lock()
	commandCache[cmd] = result
	commandCacheMu.Unlock()

	return result, nil
}

// ResolveProviderKey resolves the API key for a provider using the full chain:
//  1. Environment variable (e.g. ANTHROPIC_API_KEY)
//  2. Auth file (~/.weave/auth.json)
//  3. Config file provider entry (providers.anthropic.api_key) via ResolveValue
func ResolveProviderKey(providerName, envVar string, cfgEntry *ProviderEntry) (string, error) {
	// 1. Environment variable (highest priority)
	if v := os.Getenv(envVar); v != "" {
		return v, nil
	}

	// 2. Auth file
	authFile, err := auth.Load()
	if err != nil {
		return "", fmt.Errorf("load auth file: %w", err)
	}

	if v := authFile.GetProviderKey(providerName); v != "" {
		return v, nil
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
