package model

import (
	"sync"
)

var providerEnvReg = newProviderEnvRegistry()

type providerEnvRegistry struct {
	mu   sync.RWMutex
	vars map[string]string
}

func newProviderEnvRegistry() *providerEnvRegistry {
	return &providerEnvRegistry{vars: make(map[string]string)}
}

// RegisterProviderEnvVar registers the environment variable name for a provider's API key.
func RegisterProviderEnvVar(providerName, envVar string) {
	providerEnvReg.mu.Lock()
	defer providerEnvReg.mu.Unlock()

	providerEnvReg.vars[providerName] = envVar
}

// ProviderEnvVar returns the environment variable name for a provider's API key.
func ProviderEnvVar(providerName string) string {
	providerEnvReg.mu.RLock()
	defer providerEnvReg.mu.RUnlock()

	return providerEnvReg.vars[providerName]
}

// ResetProviderEnvVarRegistry clears all registered env var mappings. For testing only.
func ResetProviderEnvVarRegistry() {
	providerEnvReg.mu.Lock()
	defer providerEnvReg.mu.Unlock()

	providerEnvReg.vars = make(map[string]string)
}
