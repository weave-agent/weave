package model

import "weave/sdk/registry"

var providerEnvReg = registry.New[string]()

// RegisterProviderEnvVar registers the environment variable name for a provider's API key.
func RegisterProviderEnvVar(providerName, envVar string) {
	providerEnvReg.Register(providerName, envVar)
}

// ProviderEnvVar returns the environment variable name for a provider's API key.
func ProviderEnvVar(providerName string) string {
	v, _ := providerEnvReg.Get(providerName)
	return v
}

// ResetProviderEnvVarRegistry clears all registered env var mappings. For testing only.
func ResetProviderEnvVarRegistry() {
	providerEnvReg.Reset()
}
